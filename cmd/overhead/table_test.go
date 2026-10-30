package main

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
	"github.com/amirradjou/overhead/internal/poller"
)

var t0 = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func snapshot() poller.Snapshot {
	return poller.Snapshot{
		At: t0,
		Aircraft: []poller.Aircraft{
			{
				Position: feeds.Position{Hex: "c0174f", Callsign: "ACA101", TypeCode: "B38M", Registration: "C-FGKN",
					OnGround: true, SpeedKt: new(12.0), Squawk: "1234", Source: "adsb.fi", At: t0.Add(-2 * time.Second)},
				DistanceKm: 1.2, BearingDeg: 66,
			},
			{
				Position: feeds.Position{Hex: "ae1234", Callsign: "RCH401", TypeCode: "C17", AltFt: new(3000),
					VertRateFpm: new(640), Squawk: "7700", Emergency: "general", Military: true, Source: "adsb.lol", At: t0},
				DistanceKm: 9.6, BearingDeg: 310,
			},
		},
		Feeds: []poller.FeedResult{
			{Name: "adsb.lol", Count: 34, Took: 120 * time.Millisecond},
			{Name: "adsb.fi", Err: &url.Error{Op: "Get", URL: "https://opendata.adsb.fi/api/v3/x", Err: errors.New("context deadline exceeded")}},
		},
		Stats: poller.Stats{Started: t0.Add(-time.Minute), Polls: 12, LastUpdate: t0, MaxGap: 5 * time.Second},
	}
}

func TestRenderTable(t *testing.T) {
	var b strings.Builder
	renderTable(&b, snapshot(), geo.Point{Lat: 43.6777, Lon: -79.6248}, 30)
	out := b.String()
	for _, want := range []string{
		"2 aircraft within 30 km of 43.6777, -79.6248",
		"adsb.lol 34 (120ms) · adsb.fi error: context deadline exceeded",
		"longest gap 5s · 12 polls",
		"ACA101", "ground", "+640", "EMERGENCY general, military",
		"Data: adsb.lol https://adsb.lol · adsb.fi https://adsb.fi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "ACA101") > strings.Index(out, "RCH401") {
		t.Error("nearest aircraft should come first")
	}
}

func TestRenderLine(t *testing.T) {
	var b strings.Builder
	s := snapshot()
	s.Feeds[0] = poller.FeedResult{Name: "adsb.lol", Resting: true, Count: 30}
	s.Feeds[1] = poller.FeedResult{Name: "adsb.fi", Waiting: true}
	renderLine(&b, s)
	out := b.String()
	if !strings.Contains(out, "2 aircraft  adsb.lol 30 (earlier) · adsb.fi backing off  nearest ACA101 1.2 km") {
		t.Fatalf("line = %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("want one line, got %q", out)
	}
}

func TestRenderSummary(t *testing.T) {
	good := poller.Stats{
		Started: t0, Polls: 720, Updates: 720, LastUpdate: t0.Add(time.Hour), MaxGap: 5200 * time.Millisecond,
		Feeds:    []poller.FeedStats{{Name: "adsb.lol", OK: 718, Errors: 2, LastError: "timeout", LastErrorAt: t0}, {Name: "adsb.fi", OK: 720}},
		Distinct: 312, MaxAtOnce: 41,
	}
	var b strings.Builder
	if err := renderSummary(&b, good, t0.Add(time.Hour+2*time.Second), 30*time.Second); err != nil {
		t.Fatalf("good run failed: %v\n%s", err, b.String())
	}
	for _, want := range []string{"ran 1h0m2s: 720 polls", "longest gap between updates: 5.2s (limit 30s)",
		"adsb.lol       718 ok, 2 errors", "312 distinct, at most 41", "duplicates: none", "check: pass"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, b.String())
		}
	}

	gappy := good
	gappy.MaxGap = 45 * time.Second
	if err := renderSummary(&strings.Builder{}, gappy, t0.Add(time.Hour), 30*time.Second); err == nil || !strings.Contains(err.Error(), "updates stopped for 45s") {
		t.Errorf("a 45s gap should fail: %v", err)
	}

	dup := good
	dup.Duplicates, dup.LastDuplicate = 1, "ACA101 on aaaaaa and ~bbbbb at 12:00:00"
	b.Reset()
	if err := renderSummary(&b, dup, t0.Add(time.Hour), 30*time.Second); err == nil || !strings.Contains(b.String(), "ACA101 on aaaaaa and ~bbbbb") {
		t.Errorf("duplicates should fail and be named: %v\n%s", err, b.String())
	}

	// The run ends long after the last update: the open gap counts.
	stalled := good
	if err := renderSummary(&strings.Builder{}, stalled, t0.Add(time.Hour+time.Minute), 30*time.Second); err == nil {
		t.Error("a minute without updates at the end should fail")
	}

	oneFeed := good
	oneFeed.Feeds = []poller.FeedStats{good.Feeds[0], {Name: "adsb.fi", Errors: 720}}
	if err := renderSummary(&strings.Builder{}, oneFeed, t0.Add(time.Hour), 30*time.Second); err == nil ||
		!strings.Contains(err.Error(), "adsb.fi never answered") {
		t.Errorf("a feed that never answered means nothing was merged: %v", err)
	}

	empty := good
	empty.Distinct, empty.MaxAtOnce = 0, 0
	if err := renderSummary(&strings.Builder{}, empty, t0.Add(time.Hour), 30*time.Second); err == nil ||
		!strings.Contains(err.Error(), "no aircraft seen") {
		t.Errorf("a run that saw nothing proves nothing: %v", err)
	}
}

func TestEnvCenter(t *testing.T) {
	t.Setenv("OVERHEAD_LAT", "")
	t.Setenv("OVERHEAD_LON", "")
	if c, err := envCenter(); err != nil || c != defaultCenter {
		t.Fatalf("unset env: %v %v", c, err)
	}
	t.Setenv("OVERHEAD_LAT", " 43.7 ")
	t.Setenv("OVERHEAD_LON", "-79.4")
	if c, err := envCenter(); err != nil || c != (geo.Point{Lat: 43.7, Lon: -79.4}) {
		t.Fatalf("env: %v %v", c, err)
	}
	t.Setenv("OVERHEAD_LON", "west")
	if _, err := envCenter(); err == nil {
		t.Fatal("a non-number should fail")
	}
}
