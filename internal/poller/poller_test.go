package poller

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

var (
	home = geo.Point{Lat: 43.6777, Lon: -79.6248}
	t0   = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
)

type answer struct {
	pos []feeds.Position
	err error
}

// fakeFeed gives its answers in order, repeating the last one.
type fakeFeed struct {
	name    string
	answers []answer
	calls   int
}

func (f *fakeFeed) Name() string { return f.name }

func (f *fakeFeed) Fetch(context.Context, geo.Point, float64) ([]feeds.Position, error) {
	a := f.answers[min(f.calls, len(f.answers)-1)]
	f.calls++
	return a.pos, a.err
}

func pos(hex, callsign string, lat float64, at time.Time) feeds.Position {
	return feeds.Position{Hex: hex, Callsign: callsign, Lat: lat, Lon: home.Lon, At: at}
}

func ok(ps ...feeds.Position) answer { return answer{pos: ps} }

func newPoller(fs ...*fakeFeed) *Poller {
	p := &Poller{Center: home, RadiusKm: 30, Interval: 5 * time.Second}
	for _, f := range fs {
		p.Feeds = append(p.Feeds, f)
	}
	return p
}

func hexes(s Snapshot) string {
	var out []string
	for _, a := range s.Aircraft {
		out = append(out, a.Hex)
	}
	return strings.Join(out, ",")
}

func mustPoll(t *testing.T, p *Poller, now time.Time) Snapshot {
	t.Helper()
	s, ok := p.Poll(context.Background(), now)
	if !ok {
		t.Fatal("Poll reported a cancelled context")
	}
	return s
}

func TestPollMergesAndPlaces(t *testing.T) {
	lol := &fakeFeed{name: "adsb.lol", answers: []answer{ok(
		pos("aaaaaa", "FAR1", 43.85, t0), // ~19 km north
		pos("bbbbbb", "NEAR1", 43.69, t0),
	)}}
	fi := &fakeFeed{name: "adsb.fi", answers: []answer{ok(
		pos("bbbbbb", "NEAR1", 43.70, t0.Add(time.Second)), // fresher, ~2.5 km north
		pos("cccccc", "MID1", 43.75, t0),
	)}}
	p := newPoller(lol, fi)

	s := mustPoll(t, p, t0)
	if got := hexes(s); got != "bbbbbb,cccccc,aaaaaa" {
		t.Fatalf("aircraft = %s, want one each, nearest first", got)
	}
	if near := s.Aircraft[0]; near.Lat != 43.70 || near.DistanceKm < 2.4 || near.DistanceKm > 2.6 || near.BearingDeg > 0.1 {
		t.Errorf("nearest = %+v, want the fresher adsb.fi report 2.5 km due north", near)
	}
	if s.Feeds[0].Name != "adsb.lol" || s.Feeds[0].Count != 2 || s.Feeds[1].Count != 2 {
		t.Errorf("feed results = %+v", s.Feeds)
	}
	st := s.Stats
	if st.Polls != 1 || st.Updates != 1 || st.Distinct != 3 || st.MaxAtOnce != 3 || st.Duplicates != 0 {
		t.Errorf("stats = %+v", st)
	}
	if st.Feeds[0].OK != 1 || st.Feeds[1].OK != 1 {
		t.Errorf("feed stats = %+v", st.Feeds)
	}
}

func TestAircraftLeavesWhenEveryFeedAnswered(t *testing.T) {
	a := &fakeFeed{name: "a", answers: []answer{ok(pos("aaaaaa", "X1", 43.7, t0)), ok()}}
	b := &fakeFeed{name: "b", answers: []answer{ok()}}
	p := newPoller(a, b)

	mustPoll(t, p, t0)
	if s := mustPoll(t, p, t0.Add(5*time.Second)); len(s.Aircraft) != 0 {
		t.Fatalf("both feeds answered without aaaaaa, yet it stayed: %s", hexes(s))
	}
}

func TestSpacedFeedRestsAndKeepsItsAnswer(t *testing.T) {
	every := &fakeFeed{name: "every", answers: []answer{ok(pos("aaaaaa", "A1", 43.70, t0))}}
	spaced := &fakeFeed{name: "spaced", answers: []answer{
		ok(pos("aaaaaa", "A1", 43.70, t0), pos("bbbbbb", "B1", 43.72, t0)),
		ok(pos("aaaaaa", "A1", 43.70, t0)), // bbbbbb has left
	}}
	p := newPoller(every, spaced)
	p.Spacing = map[string]time.Duration{"spaced": 15 * time.Second} // three 5 s polls

	var resting []bool
	var seen []string
	for i := range 7 {
		s := mustPoll(t, p, t0.Add(time.Duration(i)*5*time.Second))
		resting = append(resting, s.Feeds[1].Resting)
		seen = append(seen, hexes(s))
	}
	if want := []bool{false, true, true, false, true, true, false}; !slices.Equal(resting, want) {
		t.Fatalf("spaced feed resting = %v, want %v", resting, want)
	}
	if spaced.calls != 3 || every.calls != 7 {
		t.Fatalf("calls: spaced %d, every %d; want 3 and 7", spaced.calls, every.calls)
	}
	// bbbbbb, seen only by the spaced feed, stays while that feed rests and goes once it answers without it.
	if want := []string{"aaaaaa,bbbbbb", "aaaaaa,bbbbbb", "aaaaaa,bbbbbb", "aaaaaa", "aaaaaa", "aaaaaa", "aaaaaa"}; !slices.Equal(seen, want) {
		t.Fatalf("picture = %v, want %v", seen, want)
	}
	if st := p.Stats(); st.Updates != 7 || st.Feeds[1].Waits != 0 || st.Feeds[1].OK != 3 {
		t.Errorf("resting is neither a wait nor an error: %+v", st)
	}
}

func TestPartialFailureKeepsLastPictureUntilMaxAge(t *testing.T) {
	a := &fakeFeed{name: "a", answers: []answer{ok()}}
	b := &fakeFeed{name: "b", answers: []answer{ok(pos("bbbbbb", "B1", 43.7, t0)), {err: errors.New("timeout")}}}
	p := newPoller(a, b)
	p.MaxAge = 30 * time.Second

	mustPoll(t, p, t0)
	s := mustPoll(t, p, t0.Add(5*time.Second))
	if hexes(s) != "bbbbbb" {
		t.Fatalf("b failed, so what it last showed should stay: %q", hexes(s))
	}
	if s.Stats.Updates != 2 || s.Stats.Feeds[1].Errors != 1 || s.Stats.Feeds[1].LastError != "timeout" {
		t.Errorf("stats = %+v", s.Stats)
	}
	// b is backing off and then failing; after MaxAge the old position goes.
	for at := t0.Add(10 * time.Second); at.Before(t0.Add(30 * time.Second)); at = at.Add(5 * time.Second) {
		if s = mustPoll(t, p, at); hexes(s) != "bbbbbb" {
			t.Fatalf("at %v, younger than MaxAge, bbbbbb should still be there", at.Sub(t0))
		}
	}
	s = mustPoll(t, p, t0.Add(35*time.Second))
	if len(s.Aircraft) != 0 {
		t.Fatalf("position older than MaxAge kept: %s", hexes(s))
	}
}

func TestGapStats(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{ok(), {err: errors.New("down")}, {err: errors.New("down")}, ok()}}
	p := newPoller(f)
	p.Interval = time.Hour // keep backoff out of the way: it allows the next poll after the first failure

	mustPoll(t, p, t0)
	mustPoll(t, p, t0.Add(5*time.Second))  // fails
	mustPoll(t, p, t0.Add(10*time.Second)) // fails; next poll is skipped
	s := mustPoll(t, p, t0.Add(15*time.Second))
	if !s.Feeds[0].Waiting {
		t.Fatalf("after two failures the feed should sit out a poll: %+v", s.Feeds[0])
	}
	s = mustPoll(t, p, t0.Add(40*time.Second))
	st := s.Stats
	if st.Polls != 5 || st.Updates != 2 || st.MaxGap != 40*time.Second {
		t.Fatalf("stats = %+v, want 5 polls, 2 updates, 40s gap", st)
	}
	if got := st.GapUntil(t0.Add(100 * time.Second)); got != 60*time.Second {
		t.Errorf("GapUntil counts the open gap: got %v, want 60s", got)
	}
}

func TestGapBeforeFirstUpdate(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{{err: errors.New("down")}, ok()}}
	p := newPoller(f)
	mustPoll(t, p, t0)
	s := mustPoll(t, p, t0.Add(7*time.Second))
	if s.Stats.MaxGap != 7*time.Second {
		t.Fatalf("MaxGap = %v, want 7s from the start of the run", s.Stats.MaxGap)
	}
}

func TestBackoffSchedule(t *testing.T) {
	down := &fakeFeed{name: "down", answers: []answer{{err: errors.New("x")}}}
	p := newPoller(down)
	var asked []bool
	for i := range 12 {
		s := mustPoll(t, p, t0.Add(time.Duration(i)*5*time.Second))
		asked = append(asked, !s.Feeds[0].Waiting)
	}
	// Asked, asked, skip 1, asked, skip 3, asked, skip 7 ...
	want := []bool{true, true, false, true, false, false, false, true, false, false, false, false}
	for i := range want {
		if asked[i] != want[i] {
			t.Fatalf("asked = %v, want %v", asked, want)
		}
	}
	if st := p.Stats(); st.Feeds[0].Errors != 4 || st.Feeds[0].Waits != 8 {
		t.Errorf("feed stats = %+v", st.Feeds[0])
	}

	b := backoff{failures: 40}
	b.fail(t0, 5*time.Second, 0)
	if b.skip != int(maxBackoff/(5*time.Second)) {
		t.Errorf("skip = %d, want capped at %d polls", b.skip, maxBackoff/(5*time.Second))
	}
}

func TestBackoffResetsOnSuccess(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{{err: errors.New("x")}, ok(), {err: errors.New("x")}, ok()}}
	p := newPoller(f)
	for i := range 4 {
		if s := mustPoll(t, p, t0.Add(time.Duration(i)*5*time.Second)); s.Feeds[0].Waiting {
			t.Fatalf("poll %d: a success in between should reset the backoff", i)
		}
	}
}

func TestRateLimitedFeedWaitsRetryAfter(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{{err: &feeds.StatusError{Feed: "a", Code: 429, RetryAfter: 90 * time.Second}}, ok()}}
	p := newPoller(f)
	mustPoll(t, p, t0)
	if s := mustPoll(t, p, t0.Add(85*time.Second)); !s.Feeds[0].Waiting {
		t.Fatal("feed asked again before its Retry-After")
	}
	if s := mustPoll(t, p, t0.Add(90*time.Second)); s.Feeds[0].Waiting || s.Feeds[0].Err != nil {
		t.Fatalf("feed should be asked once Retry-After passed: %+v", s.Feeds[0])
	}
}

func TestRetryAfterOnAnyStatus(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{{err: &feeds.StatusError{Feed: "a", Code: 503, RetryAfter: 40 * time.Second}}, ok()}}
	p := newPoller(f)
	mustPoll(t, p, t0)
	if s := mustPoll(t, p, t0.Add(35*time.Second)); !s.Feeds[0].Waiting {
		t.Fatal("a 503's Retry-After was ignored")
	}
	if s := mustPoll(t, p, t0.Add(40*time.Second)); s.Feeds[0].Waiting {
		t.Fatal("feed should be asked once Retry-After passed")
	}
}

func TestDuplicateNearbyAirborne(t *testing.T) {
	at := func(p feeds.Position, ft int) feeds.Position { p.AltFt = &ft; return p }
	tests := []struct {
		name string
		ps   []feeds.Position
		want string
	}{
		// 0.001° of latitude is 111 m: one aircraft under two addresses.
		{"same aircraft twice", []feeds.Position{at(pos("aaaaaa", "", 43.700, t0), 3000), at(pos("~bbbbb", "", 43.701, t0), 3100)}, "aaaaaa and ~bbbbb 111 m apart at 3000 ft"},
		{"stacked 1000 ft apart", []feeds.Position{at(pos("aaaaaa", "", 43.700, t0), 3000), at(pos("bbbbbb", "", 43.700, t0), 4000)}, ""},
		{"same height 2 km apart", []feeds.Position{at(pos("aaaaaa", "", 43.700, t0), 3000), at(pos("bbbbbb", "", 43.718, t0), 3000)}, ""},
		{"on the ground at the gates", []feeds.Position{pos("aaaaaa", "", 43.700, t0), pos("bbbbbb", "", 43.7001, t0)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustPoll(t, newPoller(&fakeFeed{name: "a", answers: []answer{ok(tt.ps...)}}), t0)
			if tt.want == "" && s.Stats.Duplicates != 0 {
				t.Fatalf("false duplicate: %s", s.Stats.LastDuplicate)
			}
			if tt.want != "" && (s.Stats.Duplicates != 1 || !strings.HasPrefix(s.Stats.LastDuplicate, tt.want)) {
				t.Fatalf("duplicates = %d %q, want %q", s.Stats.Duplicates, s.Stats.LastDuplicate, tt.want)
			}
		})
	}
}

func TestDuplicateCallsignCounted(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{ok(
		pos("aaaaaa", "ACA101", 43.70, t0),
		pos("~bbbbb", "ACA101", 43.71, t0),
		pos("cccccc", "", 43.72, t0),
		pos("dddddd", "", 43.73, t0),
	)}}
	s := mustPoll(t, newPoller(f), t0)
	st := s.Stats
	if st.Duplicates != 1 || !strings.Contains(st.LastDuplicate, "ACA101 on aaaaaa and ~bbbbb") {
		t.Fatalf("duplicates = %d %q", st.Duplicates, st.LastDuplicate)
	}
	if st.Distinct != 4 {
		t.Errorf("Distinct = %d, want 4 (empty callsigns are not duplicates)", st.Distinct)
	}
}

func TestPollCancelledChangesNothing(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{ok(pos("aaaaaa", "X", 43.7, t0))}}
	p := newPoller(f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := p.Poll(ctx, t0); ok {
		t.Fatal("Poll should report the cancelled context")
	}
	if st := p.Stats(); st.Polls != 0 || st.Feeds[0].Errors != 0 {
		t.Fatalf("a cancelled poll was counted: %+v", st)
	}
}

func TestRun(t *testing.T) {
	f := &fakeFeed{name: "a", answers: []answer{ok(pos("aaaaaa", "X", 43.7, time.Now()))}}
	p := newPoller(f)
	p.Interval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emitted := 0
	st := p.Run(ctx, func(s Snapshot) {
		emitted++
		if emitted == 3 {
			cancel()
		}
	})
	if emitted != 3 || st.Polls != 3 || st.Updates != 3 {
		t.Fatalf("emitted %d, stats %+v", emitted, st)
	}
}
