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

// WatchFacts loads the facts file overhead-stats writes (data/facts.json) now and whenever it changes, checking
// every interval until ctx ends. A missing or unreadable file leaves the facts as they were.
func WatchFacts(ctx context.Context, path string, s *State, interval time.Duration) {
	var seen time.Time
	load := func() {
		fi, err := os.Stat(path)
		if err != nil || !fi.ModTime().After(seen) {
			return
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var doc struct {
			Facts []string `json:"facts"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			slog.Warn("live: facts file unreadable", "path", path, "err", err)
			return
		}
		seen = fi.ModTime()
		s.SetFacts(doc.Facts)
	}
	load()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			load()
		}
	}
}
