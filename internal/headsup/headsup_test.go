package headsup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
	"github.com/amirradjou/overhead/internal/history"
	"github.com/amirradjou/overhead/internal/live"
)

var (
	pearson = geo.Point{Lat: 43.6777, Lon: -79.6248}
	south   = live.Window{AzimuthDeg: 180, FOVDeg: 90, MinElevationDeg: 5}
	t0      = time.Date(2026, 10, 30, 18, 0, 0, 0, time.Local)
)

func plane(hex, callsign, typeCode string, altFt int, over func(*live.Plane)) live.Plane {
	p := live.Plane{Position: feeds.Position{Hex: hex, Callsign: callsign, TypeCode: typeCode, AltFt: &altFt}, DistanceKm: 12}
	if over != nil {
		over(&p)
	}
	return p
}

func kinds(ns []Notice) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Hex+":"+string(n.Kind))
	}
	return out
}

type fakeTypes map[string]bool

func (f fakeTypes) Known(code string) bool { return f[code] }

func TestRules(t *testing.T) {
	v := live.View{At: t0, Planes: []live.Plane{
		// An emergency counts wherever it is, in view or not.
		plane("e00001", "ACA101", "A320", 6000, func(p *live.Plane) { p.Squawk, p.Notable = "7700", "Squawking 7700: emergency" }),
		// A rare airframe coming into view in two minutes: worth a heads-up; in eight minutes: not yet.
		plane("a12401", "ADB3021", "A124", 9000, func(p *live.Plane) { p.Coming = &live.Coming{InSec: 120, Side: "left"} }),
		plane("a12402", "ADB3022", "A124", 9000, func(p *live.Plane) { p.Coming = &live.Coming{InSec: 480, Side: "left"} }),
		// Low and nearly straight over the window.
		plane("10e001", "N123AB", "C172", 2000, func(p *live.Plane) { p.InView, p.ElevationDeg = true, 50 }),
		// Low but far down the sky: an ordinary approach.
		plane("10e002", "WJA9", "B38M", 2000, func(p *live.Plane) { p.InView, p.ElevationDeg = true, 12 }),
		// A type never seen here.
		plane("b74801", "DLH400", "B748", 30000, func(p *live.Plane) { p.InView, p.ElevationDeg = true, 20 }),
		// On the ground: nothing.
		plane("0a0a0a", "ACA1", "A124", 0, func(p *live.Plane) { p.OnGround, p.InView = true, true }),
	}}
	got := kinds(Rules{Types: fakeTypes{"A320": true, "A124": true, "C172": true, "B38M": true}}.Check(v))
	want := []string{"e00001:emergency", "a12401:rare", "10e001:low", "b74801:new-type"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("notices = %v, want %v", got, want)
	}
	n := Rules{}.Check(live.View{At: t0, Planes: v.Planes[1:2]})[0]
	if n.Text != "Rare visitor: Antonov An-124 Ruslan (ADB3021) in view in about 2 min." {
		t.Errorf("text = %q", n.Text)
	}
}

type flakySender struct {
	fail  bool
	texts []string
}

func (f *flakySender) Send(_ context.Context, text string) error {
	if f.fail {
		return errors.New("telegram: HTTP 502")
	}
	f.texts = append(f.texts, text)
	return nil
}

func TestFailedSendIsRetriedNotLost(t *testing.T) {
	state := &live.State{Center: pearson, RadiusKm: 60, Window: south}
	alt := 6000
	state.Update(events.Position{Position: feeds.Position{
		Hex: "e00001", Callsign: "ACA101", Squawk: "7700", Lat: 43.70, Lon: -79.62, AltFt: &alt, At: t0,
	}})
	send := &flakySender{fail: true}
	w := &Watcher{State: state, Ledger: &MemLedger{}, Send: send}

	if got := w.Tick(context.Background(), t0); len(got) != 0 {
		t.Fatalf("a failed send was reported as sent: %v", kinds(got))
	}
	send.fail = false
	if got := w.Tick(context.Background(), t0.Add(30*time.Second)); len(got) != 0 {
		t.Fatal("retried before its minute was up")
	}
	// The plane is still there (its report refreshed) a minute later: the emergency goes out, once.
	state.Update(events.Position{Position: feeds.Position{
		Hex: "e00001", Callsign: "ACA101", Squawk: "7700", Lat: 43.70, Lon: -79.62, AltFt: &alt, At: t0.Add(61 * time.Second),
	}})
	if got := w.Tick(context.Background(), t0.Add(61*time.Second)); len(got) != 1 || len(send.texts) != 1 {
		t.Fatalf("after the failure: sent %v, texts %v", kinds(got), send.texts)
	}
	if got := w.Tick(context.Background(), t0.Add(63*time.Second)); len(got) != 0 {
		t.Fatal("sent twice")
	}
}

func TestLowPassJudgedWhereItEntersTheWindow(t *testing.T) {
	// High in the sky now but outside the window's slice, entering it low at its edge: not "nearly over the window".
	p := plane("10e003", "N55AB", "C172", 2000, func(p *live.Plane) {
		p.ElevationDeg, p.Coming = 70, &live.Coming{InSec: 60, Side: "left", ElevationDeg: 12}
	})
	if got := kinds(Rules{}.Check(live.View{At: t0, Planes: []live.Plane{p}})); len(got) != 0 {
		t.Fatalf("notices = %v", got)
	}
}

func TestLedger(t *testing.T) {
	l, err := OpenLedger(filepath.Join(t.TempDir(), "headsup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	n := Notice{Hex: "a12401", Kind: Rare, At: t0}
	for i, want := range []bool{true, false} {
		if first, err := l.First(n); err != nil || first != want {
			t.Fatalf("First #%d = %v, %v", i, first, err)
		}
	}
	n.At = t0.Add(24 * time.Hour)
	if first, _ := l.First(n); !first {
		t.Fatal("a new day is a new heads-up")
	}

	// The first week only learns types.
	if !l.Known("B748") {
		t.Fatal("during the first week every type counts as known")
	}
	l.now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if l.Known("A388") {
		t.Fatal("after the first week a new type is news")
	}
	if !l.Known("A388") || !l.Known("B748") {
		t.Fatal("a type seen once is known")
	}
}

func TestTelegram(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botSECRET-TOKEN/sendMessage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		if got.Get("text") == "fail" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	tg := &Telegram{Token: "SECRET-TOKEN", ChatID: "42", Client: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	if err := tg.Send(ctx, "Rare visitor"); err != nil || got.Get("chat_id") != "42" || got.Get("text") != "Rare visitor" {
		t.Fatalf("send: %v, %v", err, got)
	}
	for _, sender := range []*Telegram{tg, {Token: "SECRET-TOKEN", ChatID: "42", Client: srv.Client(), Base: "http://127.0.0.1:1"}} {
		err := sender.Send(ctx, "fail")
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("error = %v: it must exist and must not carry the token", err)
		}
	}
}

// TestReplay is M5's "Done when" in small: a stored stretch replayed through the watcher gives exactly the
// expected heads-ups, however long the rare plane stays in sight.
func TestReplay(t *testing.T) {
	root := t.TempDir()
	run := "20261030T220000Z-abc123"
	start := time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC)
	// An An-124 east of a south-facing window, flying west across it at 250 kt for six minutes, and an A320
	// going the same way a little later. One position every 5 s each.
	var evs []events.Position
	seq := uint64(0)
	for i := range 72 {
		at := start.Add(time.Duration(i) * 5 * time.Second)
		for _, p := range []struct {
			hex, callsign, typeCode string
			from                    geo.Point
		}{
			{"a12401", "ADB3021", "A124", geo.Destination(pearson, 110, 25)},
			{"c0aaaa", "ACA101", "A320", geo.Destination(pearson, 120, 30)},
		} {
			seq++
			pos := geo.Destination(p.from, 270, 250*geo.KmPerNM/3600*float64(i*5))
			alt, track, kt := 10000, 270.0, 250.0
			evs = append(evs, events.Position{Run: run, Seq: seq, Published: at, Position: feeds.Position{
				Hex: p.hex, Callsign: p.callsign, TypeCode: p.typeCode, Lat: pos.Lat, Lon: pos.Lon,
				AltFt: &alt, TrackDeg: &track, SpeedKt: &kt, At: at,
			}})
		}
	}
	store := history.DirStore{Root: root}
	for i := 0; i < len(evs); i += 24 { // batches of a minute
		bt := &history.Batch{Minute: evs[i].Published.Truncate(time.Minute), Run: run, FirstSeq: evs[i].Seq}
		for _, ev := range evs[i:min(i+24, len(evs))] {
			line, _ := json.Marshal(ev)
			bt.Lines = append(bt.Lines, line)
		}
		body, _ := bt.Encode()
		if err := store.Put(context.Background(), bt.Key(), body); err != nil {
			t.Fatal(err)
		}
	}

	stored, err := history.Read(root, start, start.Add(time.Hour))
	if err != nil || len(stored) != len(evs) {
		t.Fatalf("read %d events (%v), want %d", len(stored), err, len(evs))
	}
	replay := func() []Notice {
		return Replay(stored, &live.State{Center: pearson, RadiusKm: 60, Window: south}, Rules{}, 0)
	}
	got := replay()
	if k := kinds(got); len(k) != 1 || k[0] != "a12401:rare" {
		t.Fatalf("notices = %v, want one rare-visitor heads-up", k)
	}
	if !strings.Contains(got[0].Text, "in view in about") && !strings.Contains(got[0].Text, "in view now") {
		t.Errorf("text = %q", got[0].Text)
	}
	if again := kinds(replay()); len(again) != 1 {
		t.Fatalf("a second replay gave %v", again)
	}
}
