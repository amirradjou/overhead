// Package relay turns the poller's snapshots into events: each new position report onto the positions exchange, and
// an enrichment job for each aircraft or callsign not asked about lately. With PublicFeed set, that feed's own reports
// go to the public exchange as well.
package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"maps"
	"strings"
	"time"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/poller"
)

// DefaultAskAgain is how long before an aircraft still flying the same callsign is sent to enrichment again. The
// worker's cache answers repeats without calling the APIs, so this only bounds queue traffic.
const DefaultAskAgain = time.Hour

// forget drops what the relay remembers about an aircraft that has been out of the picture this long.
const forget = 10 * time.Minute

// Publisher is the part of bus.Publisher the relay needs.
type Publisher interface {
	Publish(ctx context.Context, msgs ...bus.Message) error
}

// Relay publishes events for snapshots. It is not safe for concurrent use.
type Relay struct {
	Run      string
	Pub      Publisher
	AskAgain time.Duration
	// PublicFeed names the feed whose reports may be shown publicly: its data's licence allows it, and merged reports
	// would mix in feeds' whose terms do not (docs/setup.md, section 7). Empty publishes nothing to the public exchange.
	PublicFeed string

	seq        uint64
	last       map[string]time.Time // hex → At of the newest position published
	lastPublic map[string]time.Time // hex → At of the newest position published to the public exchange
	seen       map[string]time.Time // hex → when last in a snapshot
	asked      map[string]ask
	stats      Stats
}

type ask struct {
	callsign string
	at       time.Time
}

// Stats counts what the relay published. Lost positions were given a sequence number but not confirmed by the broker,
// so the history shows them as a gap.
type Stats struct {
	Positions, Lost, Jobs uint64
	LastError             string
}

// NewRun names a poller run: its start time plus a random suffix, e.g. 20261029T220612Z-3fa2c1.
func NewRun(now time.Time) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b)
}

// Stats returns the counts so far.
func (r *Relay) Stats() Stats { return r.stats }

// Handle publishes the snapshot's new positions and the enrichment jobs it calls for, in one confirmed batch.
func (r *Relay) Handle(ctx context.Context, snap poller.Snapshot) error {
	if r.last == nil {
		r.last, r.seen, r.asked = make(map[string]time.Time), make(map[string]time.Time), make(map[string]ask)
		r.lastPublic = make(map[string]time.Time)
	}
	msgs, err := r.publicMessages(snap)
	if err != nil {
		return err
	}
	newAsks := make(map[string]ask)
	positions := 0
	for _, a := range snap.Aircraft {
		r.seen[a.Hex] = snap.At
		if a.At.After(r.last[a.Hex]) {
			r.seq++
			body, err := json.Marshal(events.Position{Run: r.Run, Seq: r.seq, Published: snap.At, Position: a.Position})
			if err != nil {
				return err
			}
			msgs = append(msgs, bus.Message{Exchange: bus.ExchangePositions, Key: bus.KeyPosition, Body: body})
			r.last[a.Hex] = a.At
			positions++
		}
		if r.wantsJob(a.Hex, a.Callsign, snap.At) {
			body, err := json.Marshal(events.EnrichJob{Hex: a.Hex, Callsign: a.Callsign})
			if err != nil {
				return err
			}
			msgs = append(msgs, bus.Message{Exchange: bus.ExchangeEnrich, Key: bus.KeyJob, Body: body})
			newAsks[a.Hex] = ask{callsign: a.Callsign, at: snap.At}
		}
	}
	r.prune(snap.At)
	if len(msgs) == 0 {
		return nil
	}
	if err := r.Pub.Publish(ctx, msgs...); err != nil {
		// The positions keep their sequence numbers, so the loss is visible downstream; jobs are asked again next poll.
		r.stats.Lost += uint64(positions)
		r.stats.LastError = err.Error()
		return err
	}
	r.stats.Positions += uint64(positions)
	r.stats.Jobs += uint64(len(newAsks))
	maps.Copy(r.asked, newAsks)
	return nil
}

// publicMessages returns the events for the feed's reports not yet published. Nothing another feed said is added to
// them. The other way round is fine: any feed's military or private flag still hides an aircraft from the public view.
func (r *Relay) publicMessages(snap poller.Snapshot) ([]bus.Message, error) {
	if r.PublicFeed == "" {
		return nil, nil
	}
	merged := make(map[string]feeds.Position, len(snap.Aircraft))
	for _, a := range snap.Aircraft {
		merged[a.Hex] = a.Position
	}
	var msgs []bus.Message
	for _, p := range snap.Answers[r.PublicFeed] {
		if !p.At.After(r.lastPublic[p.Hex]) {
			continue
		}
		r.lastPublic[p.Hex] = p.At
		m := merged[p.Hex]
		p.Military, p.Private = p.Military || m.Military, p.Private || m.Private
		body, err := json.Marshal(events.Position{Run: r.Run, Published: snap.At, Position: p})
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, bus.Message{Exchange: bus.ExchangePositionsPublic, Body: body})
	}
	return msgs, nil
}

func (r *Relay) wantsJob(hex, callsign string, now time.Time) bool {
	if strings.HasPrefix(hex, "~") {
		return false // not an ICAO address: there is no airframe to look up
	}
	prev, ok := r.asked[hex]
	return !ok || prev.callsign != callsign || now.Sub(prev.at) >= r.askAgain()
}

func (r *Relay) prune(now time.Time) {
	for h, at := range r.seen {
		if now.Sub(at) > forget {
			delete(r.seen, h)
			delete(r.last, h)
			delete(r.lastPublic, h)
			delete(r.asked, h)
		}
	}
}

func (r *Relay) askAgain() time.Duration {
	if r.AskAgain > 0 {
		return r.AskAgain
	}
	return DefaultAskAgain
}
