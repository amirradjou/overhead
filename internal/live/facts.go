package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"slices"
	"time"
)

// SetFacts replaces the facts the screen shows between planes.
func (s *State) SetFacts(facts []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.facts = slices.Clone(facts)
}

// maxFactsAge: facts older than this are dropped rather than shown. "N planes crossed your window today" from a job
// that stopped yesterday is wrong, not merely old.
const maxFactsAge = 3 * time.Hour

// WatchFacts loads the facts file overhead-stats writes (data/facts.json) now and whenever it changes, checking
// every interval until ctx ends. A missing or unreadable file leaves the facts as they were; facts generated more
// than maxFactsAge ago are cleared.
func WatchFacts(ctx context.Context, path string, s *State, interval time.Duration) {
	var seen, generated time.Time
	var facts []string
	check := func() {
		if fi, err := os.Stat(path); err == nil && fi.ModTime().After(seen) {
			if raw, err := os.ReadFile(path); err == nil {
				var doc struct {
					Generated time.Time `json:"generated"`
					Facts     []string  `json:"facts"`
				}
				if err := json.Unmarshal(raw, &doc); err != nil {
					slog.Warn("live: facts file unreadable", "path", path, "err", err)
				} else {
					seen, generated, facts = fi.ModTime(), doc.Generated, doc.Facts
				}
			}
		}
		if time.Since(generated) > maxFactsAge {
			s.SetFacts(nil)
			return
		}
		s.SetFacts(facts)
	}
	check()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			check()
		}
	}
}
