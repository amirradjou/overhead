package live

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWatchFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facts.json")
	s := &State{Center: pearson, RadiusKm: 60}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No file yet: no facts, and no complaint.
	go WatchFacts(ctx, path, s, 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if f := s.View(t0).Facts; f == nil || len(f) != 0 {
		t.Fatalf("facts = %#v, want an empty list", f)
	}

	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"generated":"2026-10-30T04:14:25Z","facts":["7 planes crossed your window today."]}`)
	waitFor(t, func() bool { return slices.Equal(s.View(t0).Facts, []string{"7 planes crossed your window today."}) })

	// A broken rewrite keeps the facts the screen already has.
	time.Sleep(15 * time.Millisecond) // a later modification time
	write(`{"facts": [`)
	time.Sleep(40 * time.Millisecond)
	if f := s.View(t0).Facts; len(f) != 1 {
		t.Fatalf("a broken file replaced the facts: %#v", f)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
