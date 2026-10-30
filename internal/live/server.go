package live

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// DefaultEvery is how often the stream sends a snapshot. A whole snapshot each time keeps TV browsers simple: nothing
// to patch, and a reconnect is caught up by the next message.
const DefaultEvery = 2 * time.Second

// Server serves the picture: GET /api/now (JSON), /api/events (Server-Sent Events), /api/coverage, /api/predictions,
// /healthz, and the screen's static files at / when Web is set.
type Server struct {
	State *State
	Every time.Duration
	Web   http.Handler
	Now   func() time.Time

	mu      sync.Mutex
	clients map[chan []byte]struct{}
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
	return cors(mux)
}

// cors lets a screen served from elsewhere (the Next.js dev server) read the API; it is read-only.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clock() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) now(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.State.View(s.clock()))
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

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // let a reverse proxy pass each message through at once
	ch := make(chan []byte, 1)
	s.mu.Lock()
	if s.clients == nil {
		s.clients = make(map[chan []byte]struct{})
	}
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()

	// The first snapshot goes out at once, so the screen fills without waiting for the tick.
	first, err := json.Marshal(s.State.View(s.clock()))
	if err != nil {
		return
	}
	if !s.send(w, rc, first) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if !s.send(w, rc, msg) {
				return
			}
		}
	}
}

func (s *Server) send(w http.ResponseWriter, rc *http.ResponseController, data []byte) bool {
	// A write that can't finish in time means the client is gone or stuck; dropping it frees the slot.
	_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := fmt.Fprintf(w, "retry: 3000\nevent: snapshot\ndata: %s\n\n", data); err != nil {
		return false
	}
	return rc.Flush() == nil
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
	every := s.Every
	if every <= 0 {
		every = DefaultEvery
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		// The view is worked out every tick even with no screen watching, so the prediction tally keeps counting.
		view := s.State.View(s.clock())
		s.mu.Lock()
		n := len(s.clients)
		s.mu.Unlock()
		if n == 0 {
			continue
		}
		data, err := json.Marshal(view)
		if err != nil {
			slog.Error("encode snapshot", "err", err)
			continue
		}
		s.mu.Lock()
		for ch := range s.clients {
			select {
			case ch <- data:
			default:
			}
		}
		s.mu.Unlock()
	}
}
