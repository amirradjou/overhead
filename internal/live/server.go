package live

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultEvery is how often the stream sends a snapshot. A whole snapshot each time keeps TV browsers simple: nothing
// to patch, and a reconnect is caught up by the next message.
const DefaultEvery = 2 * time.Second

// DefaultMaxStreams is how many viewers the public instance serves at once. A viewer costs about 1 KB/s (a snapshot of
// the few planes a focused screen draws, compressed), so this many is under 2 Mbit/s: as much as a home connection
// should be asked to send. The page plays its recording for anyone turned away.
const DefaultMaxStreams = 200

// Server serves the picture: GET /api/now (JSON), /api/events (Server-Sent Events), /api/coverage, /api/predictions,
// /healthz, and the screen's static files at / when Web is set.
type Server struct {
	State *State
	Every time.Duration
	Web   http.Handler
	Now   func() time.Time
	// Public is the instance anyone on the internet can reach: streams are compressed and capped at MaxStreams
	// (DefaultMaxStreams when 0), every viewer shares one encoding of each snapshot, and /api/now answers from it, so
	// the work per tick does not grow with the viewers.
	Public     bool
	MaxStreams int

	mu       sync.Mutex
	clients  map[chan *snapshot]struct{}
	latest   atomic.Pointer[snapshot]
	deflate  *flate.Writer
	deflated bytes.Buffer
}

// snapshot is one picture, encoded once for every viewer.
type snapshot struct {
	at    time.Time
	json  []byte
	frame []byte // as a Server-Sent Event
	// deflated is frame as raw deflate blocks that stand alone and end on a byte boundary, so any snapshot can follow
	// any other in a stream, whatever the viewer was sent before. Nil unless Public.
	deflated []byte
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/now", s.now)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/coverage", s.coverage)
	mux.HandleFunc("GET /api/predictions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, s.State.Predictions())
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	if s.Web != nil {
		mux.Handle("GET /", s.Web)
	}
	if s.Public {
		return cors(hardened(mux))
	}
	return cors(mux)
}

// cors lets a screen served from elsewhere (the Next.js dev server, the public page) read the API; it is read-only.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

// hardened adds what a page on the open internet should say about itself. Inline scripts and styles are the static
// export's own; photos are hotlinked from planespotters, whose image hosts change.
func hardened(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; "+
			"style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; base-uri 'none'; form-action 'none'; "+
			"object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

// Static serves a directory without listing it, and lets browsers keep the export's fingerprinted files for good.
func Static(dir string) http.Handler {
	files := http.FileServer(noListing{http.Dir(dir)})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_next/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// noListing answers for a directory only when it has an index page.
type noListing struct{ fs http.FileSystem }

func (n noListing) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		index, err := n.fs.Open(path.Join(name, "index.html"))
		if err != nil {
			f.Close()
			return nil, &fs.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
		}
		index.Close()
	}
	return f, nil
}

func (s *Server) clock() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) now(w http.ResponseWriter, _ *http.Request) {
	if !s.Public {
		writeJSON(w, s.State.View(s.clock()))
		return
	}
	snap, err := s.current()
	if err != nil {
		slog.Error("encode snapshot", "err", err)
		http.Error(w, "no picture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(snap.json)
}

func (s *Server) coverage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.State.Coverage())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write response", "err", err)
	}
}

// gzipHeader opens a gzip stream: deflate, no name, no time. Snapshots' deflate blocks follow it. The stream is never
// finished, since a stream to a screen ends only when the screen goes.
var gzipHeader = []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 0xff}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	gz := s.Public && acceptsGzip(r)
	ch := make(chan *snapshot, 1)
	if !s.join(ch) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "this screen has as many viewers as it can take", http.StatusServiceUnavailable)
		return
	}
	defer s.leave(ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // let a reverse proxy pass each message through at once
	if s.Public {
		w.Header().Set("Vary", "Accept-Encoding")
	}
	if gz {
		w.Header().Set("Content-Encoding", "gzip")
		if _, err := w.Write(gzipHeader); err != nil {
			return
		}
	}

	// The first snapshot goes out at once, so the screen fills without waiting for the tick.
	first, err := s.current()
	if err != nil {
		return
	}
	if !send(w, rc, first, gz) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case snap := <-ch:
			if !send(w, rc, snap, gz) {
				return
			}
		}
	}
}

// acceptsGzip reads Accept-Encoding the simple way: no browser that can open this page sends gzip;q=0.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		if name, _, _ := strings.Cut(strings.TrimSpace(part), ";"); strings.EqualFold(name, "gzip") {
			return true
		}
	}
	return false
}

func (s *Server) join(ch chan *snapshot) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Public {
		limit := s.MaxStreams
		if limit <= 0 {
			limit = DefaultMaxStreams
		}
		if len(s.clients) >= limit {
			return false
		}
	}
	if s.clients == nil {
		s.clients = make(map[chan *snapshot]struct{})
	}
	s.clients[ch] = struct{}{}
	return true
}

func (s *Server) leave(ch chan *snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, ch)
}

func send(w http.ResponseWriter, rc *http.ResponseController, snap *snapshot, gz bool) bool {
	data := snap.frame
	if gz {
		data = snap.deflated
	}
	// A write that can't finish in time means the client is gone or stuck; dropping it frees the slot.
	_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := w.Write(data); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// current returns the newest snapshot, encoding one if the ticker has not (no one was watching, or Run isn't running).
func (s *Server) current() (*snapshot, error) {
	now := s.clock()
	if snap := s.latest.Load(); snap != nil && now.Sub(snap.at) < 2*s.every() {
		return snap, nil
	}
	return s.encode(s.State.View(now), now)
}

func (s *Server) every() time.Duration {
	if s.Every > 0 {
		return s.Every
	}
	return DefaultEvery
}

// encode turns a view into a snapshot and keeps it as the latest.
func (s *Server) encode(view View, now time.Time) (*snapshot, error) {
	data, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	snap := &snapshot{at: now, json: data, frame: fmt.Appendf(nil, "retry: 3000\nevent: snapshot\ndata: %s\n\n", data)}
	if s.Public {
		if snap.deflated, err = s.deflateFrame(snap.frame); err != nil {
			return nil, err
		}
	}
	s.latest.Store(snap)
	return snap, nil
}

// deflateFrame compresses one frame on its own: a fresh window each time, ended by a flush, not a final block.
func (s *Server) deflateFrame(frame []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deflated.Reset()
	if s.deflate == nil {
		w, err := flate.NewWriter(&s.deflated, flate.DefaultCompression)
		if err != nil {
			return nil, err
		}
		s.deflate = w
	} else {
		s.deflate.Reset(&s.deflated)
	}
	if _, err := s.deflate.Write(frame); err != nil {
		return nil, err
	}
	if err := s.deflate.Flush(); err != nil {
		return nil, err
	}
	return bytes.Clone(s.deflated.Bytes()), nil
}

// Clients counts the connected streams.
func (s *Server) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// Run broadcasts a snapshot to every stream each Every, until ctx ends. A client still busy with the previous one
// skips a beat rather than queueing: only the latest picture matters.
func (s *Server) Run(ctx context.Context) {
	tick := time.NewTicker(s.every())
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		// The view is worked out every tick even with no screen watching, so the prediction tally keeps counting.
		now := s.clock()
		view := s.State.View(now)
		// The public instance keeps a snapshot ready: /api/now answers from it and works out nothing per request.
		if s.Clients() == 0 && !s.Public {
			continue
		}
		snap, err := s.encode(view, now)
		if err != nil {
			slog.Error("encode snapshot", "err", err)
			continue
		}
		s.mu.Lock()
		for ch := range s.clients {
			select {
			case ch <- snap:
			default:
			}
		}
		s.mu.Unlock()
	}
}
