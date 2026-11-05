package relay

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/poller"
)

var t0 = time.Date(2026, 10, 29, 22, 0, 0, 0, time.UTC)

type fakePub struct {
	sent []bus.Message
	err  error
}

func (f *fakePub) Publish(_ context.Context, msgs ...bus.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, msgs...)
	return nil
}

func (f *fakePub) take() (positions []events.Position, jobs []events.EnrichJob) {
	for _, m := range f.sent {
		switch m.Exchange {
		case bus.ExchangePositions:
			var p events.Position
			_ = json.Unmarshal(m.Body, &p)
			positions = append(positions, p)
		case bus.ExchangeEnrich:
			var j events.EnrichJob
			_ = json.Unmarshal(m.Body, &j)
			jobs = append(jobs, j)
		}
	}
	f.sent = nil
	return positions, jobs
}

func snap(at time.Time, planes ...feeds.Position) poller.Snapshot {
	s := poller.Snapshot{At: at}
	for _, p := range planes {
		s.Aircraft = append(s.Aircraft, poller.Aircraft{Position: p})
	}
	return s
}

func plane(hex, callsign string, at time.Time) feeds.Position {
	return feeds.Position{Hex: hex, Callsign: callsign, At: at}
}

func TestHandlePublishesNewPositionsWithSequence(t *testing.T) {
	pub := &fakePub{}
	r := &Relay{Run: "run1", Pub: pub}
	ctx := context.Background()

	if err := r.Handle(ctx, snap(t0, plane("aaaaaa", "ACA1", t0), plane("~bbbbb", "", t0))); err != nil {
		t.Fatal(err)
	}
	pos, jobs := pub.take()
	if len(pos) != 2 || pos[0].Seq != 1 || pos[1].Seq != 2 || pos[0].Run != "run1" || !pos[0].Published.Equal(t0) {
		t.Fatalf("positions = %+v", pos)
	}
	if len(jobs) != 1 || jobs[0] != (events.EnrichJob{Hex: "aaaaaa", Callsign: "ACA1"}) {
		t.Fatalf("jobs = %+v; a ~ address has no airframe to look up", jobs)
	}

	// Same positions again: nothing new to say.
	if err := r.Handle(ctx, snap(t0.Add(5*time.Second), plane("aaaaaa", "ACA1", t0), plane("~bbbbb", "", t0))); err != nil {
		t.Fatal(err)
	}
	if pos, jobs := pub.take(); len(pos) != 0 || len(jobs) != 0 {
		t.Fatalf("republished an old position or job: %+v %+v", pos, jobs)
	}

	// One aircraft moved, the other changed flight number.
	if err := r.Handle(ctx, snap(t0.Add(10*time.Second), plane("aaaaaa", "ACA2", t0.Add(9*time.Second)), plane("~bbbbb", "", t0))); err != nil {
		t.Fatal(err)
	}
	pos, jobs = pub.take()
	if len(pos) != 1 || pos[0].Seq != 3 {
		t.Fatalf("positions = %+v, want seq 3", pos)
	}
	if len(jobs) != 1 || jobs[0].Callsign != "ACA2" {
		t.Fatalf("a new callsign is a new flight to look up: %+v", jobs)
	}
	if st := r.Stats(); st.Positions != 3 || st.Jobs != 2 || st.Lost != 0 {
		t.Errorf("stats = %+v", st)
	}
}

func TestHandleCountsLostPositionsAndAsksAgain(t *testing.T) {
	pub := &fakePub{err: errors.New("connection refused")}
	r := &Relay{Run: "run1", Pub: pub}
	ctx := context.Background()
	if err := r.Handle(ctx, snap(t0, plane("aaaaaa", "ACA1", t0))); err == nil {
		t.Fatal("want the publish error")
	}
	if st := r.Stats(); st.Lost != 1 || st.Positions != 0 || st.LastError != "connection refused" {
		t.Fatalf("stats = %+v", st)
	}

	pub.err = nil
	if err := r.Handle(ctx, snap(t0.Add(5*time.Second), plane("aaaaaa", "ACA1", t0.Add(4*time.Second)))); err != nil {
		t.Fatal(err)
	}
	pos, jobs := pub.take()
	if len(pos) != 1 || pos[0].Seq != 2 {
		t.Fatalf("the lost event keeps seq 1, so the next is 2: %+v", pos)
	}
	if len(jobs) != 1 {
		t.Fatalf("the job was never confirmed, so it is asked again: %+v", jobs)
	}
}

func TestHandleAsksAgainAfterAWhile(t *testing.T) {
	pub := &fakePub{}
	r := &Relay{Run: "run1", Pub: pub, AskAgain: time.Minute}
	ctx := context.Background()
	for i, wantJobs := range []int{1, 0, 1} {
		at := t0.Add(time.Duration(i) * 40 * time.Second)
		if err := r.Handle(ctx, snap(at, plane("aaaaaa", "ACA1", at))); err != nil {
			t.Fatal(err)
		}
		if _, jobs := pub.take(); len(jobs) != wantJobs {
			t.Fatalf("poll %d: %d jobs, want %d", i, len(jobs), wantJobs)
		}
	}
}

func TestNewRun(t *testing.T) {
	a, b := NewRun(t0), NewRun(t0)
	if !regexp.MustCompile(`^20261029T220000Z-[0-9a-f]{6}$`).MatchString(a) || a == b {
		t.Fatalf("runs %q %q", a, b)
	}
	if !events.ValidRun(a) || events.ValidRun("../../etc") || events.ValidRun("test-run") {
		t.Fatal("ValidRun must accept NewRun's ids and nothing path-like")
	}
}

func publicEvents(t *testing.T, sent []bus.Message) []events.Position {
	t.Helper()
	var out []events.Position
	for _, m := range sent {
		if m.Exchange != bus.ExchangePositionsPublic {
			continue
		}
		var p events.Position
		if err := json.Unmarshal(m.Body, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// The public exchange gets the named feed's own reports, once each, whatever the merged picture says, and nothing at
// all from the other feed. The merged picture's flags are the exception: they only ever hide an aircraft.
func TestPublicFeedPublishesOnlyThatFeedsOwnReports(t *testing.T) {
	pub := &fakePub{}
	r := &Relay{Run: "run", Pub: pub, PublicFeed: "adsb.lol"}

	own := feeds.Position{Hex: "aaaaaa", Callsign: "ACA1", Source: "adsb.lol", At: t0}
	freshest := feeds.Position{Hex: "aaaaaa", Callsign: "ACA1", Registration: "C-FILL", Source: "adsb.fi", At: t0.Add(time.Second)}
	otherOnly := feeds.Position{Hex: "bbbbbb", Callsign: "ACA2", Source: "adsb.fi", At: t0}
	hidden := feeds.Position{Hex: "cccccc", Callsign: "ACA3", Source: "adsb.lol", At: t0}
	hiddenMerged := feeds.Position{Hex: "cccccc", Callsign: "ACA3", Source: "adsb.fi", Private: true, At: t0}

	s := snap(t0, freshest, otherOnly, hiddenMerged)
	s.Answers = map[string][]feeds.Position{
		"adsb.lol": {own, hidden},
		"adsb.fi":  {freshest, otherOnly, hiddenMerged},
	}
	if err := r.Handle(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	got := publicEvents(t, pub.sent)
	if len(got) != 2 {
		t.Fatalf("public events = %+v", got)
	}
	if got[0].Hex != "aaaaaa" || got[0].Source != "adsb.lol" || got[0].Registration != "" || !got[0].At.Equal(t0) {
		t.Errorf("the other feed's report reached the public exchange: %+v", got[0])
	}
	if got[1].Hex != "cccccc" || !got[1].Private {
		t.Errorf("a private flag from the other feed was lost: %+v", got[1])
	}

	// The feed sits out the next poll, so its answer comes round again: nothing new to publish.
	pub.sent = nil
	if err := r.Handle(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if got := publicEvents(t, pub.sent); len(got) != 0 {
		t.Fatalf("published again: %+v", got)
	}
}

func TestNoPublicFeedPublishesNothingPublic(t *testing.T) {
	pub := &fakePub{}
	r := &Relay{Run: "run", Pub: pub}
	s := snap(t0, plane("aaaaaa", "ACA1", t0))
	s.Answers = map[string][]feeds.Position{"adsb.lol": {plane("aaaaaa", "ACA1", t0)}}
	if err := r.Handle(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if got := publicEvents(t, pub.sent); len(got) != 0 {
		t.Fatalf("public events = %+v", got)
	}
}
