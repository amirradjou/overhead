package live

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

var (
	pearson = geo.Point{Lat: 43.6777, Lon: -79.6248}
	t0      = time.Date(2026, 10, 29, 18, 0, 0, 0, time.Local)
)

func pos(hex, callsign string, lat float64, at time.Time) events.Position {
	alt, gs := 3000, 180.0
	return events.Position{Position: feeds.Position{
		Hex: hex, Callsign: callsign, Lat: lat, Lon: pearson.Lon, AltFt: &alt, SpeedKt: &gs, Source: "adsb.fi", At: at,
	}}
}

func TestViewNearestFirstAndAgesOut(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30}
	s.Update(pos("aaaaaa", "ACA1", 43.85, t0))
	s.Update(pos("bbbbbb", "ACA2", 43.70, t0))
	s.Update(pos("bbbbbb", "ACA2", 43.99, t0.Add(-time.Second))) // older: ignored

	v := s.View(t0.Add(time.Second))
	if len(v.Planes) != 2 || v.Planes[0].Hex != "bbbbbb" || v.Planes[0].Lat != 43.70 {
		t.Fatalf("planes = %+v", v.Planes)
	}
	if v.Today != 2 || len(v.Sources) != 1 || v.Sources[0] != "adsb.fi" || v.Center.Lat != pearson.Lat {
		t.Fatalf("view = %+v", v)
	}
	if v := s.View(t0.Add(DefaultMaxAge + time.Second)); len(v.Planes) != 0 {
		t.Fatalf("stale positions kept: %+v", v.Planes)
	}
}

func TestPublicViewShowsCommercialFlightsOnly(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30, Public: true}
	s.Update(pos("aaaaaa", "ACA101", 43.70, t0))
	private := pos("bbbbbb", "N123AB", 43.71, t0)
	s.Update(private)
	flagged := pos("cccccc", "EJA455", 43.72, t0)
	flagged.Private = true
	s.Update(flagged)
	mil := pos("dddddd", "RCH401", 43.73, t0)
	mil.Military = true
	s.Update(mil)

	v := s.View(t0)
	if len(v.Planes) != 1 || v.Planes[0].Hex != "aaaaaa" {
		t.Fatalf("public view = %+v", v.Planes)
	}
}

func TestViewJoinsEnrichmentAndETA(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30}
	p := pos("c08861", "POE672", 43.80, t0) // about 13.6 km north of the destination
	s.Update(p)
	s.SetAircraft(events.Aircraft{Hex: "c08861", TypeCode: "E295", Route: &events.Route{
		Callsign: "POE672", Destination: events.Airport{ICAO: "CYYZ", Lat: 43.6772, Lon: -79.6306},
	}})
	pl := s.View(t0).Planes[0]
	if pl.Info == nil || pl.Info.TypeCode != "E295" || pl.ToDestinationKm == nil || pl.ETAMin == nil {
		t.Fatalf("plane = %+v", pl)
	}
	// 13.7 km at 180 kt (333 km/h) is about 2.5 minutes.
	if *pl.ETAMin < 2.2 || *pl.ETAMin > 2.8 {
		t.Errorf("ETA = %.2f min", *pl.ETAMin)
	}

	// The same airframe on its next flight: the old route must not show.
	s.Update(pos("c08861", "POE999", 43.80, t0.Add(time.Second)))
	if pl := s.View(t0.Add(time.Second)).Planes[0]; pl.Info.Route != nil || pl.ETAMin != nil {
		t.Fatalf("stale route shown: %+v", pl.Info.Route)
	}
}

func TestViewDropsImpossibleRoutes(t *testing.T) {
	airport := func(lat, lon float64) events.Airport { return events.Airport{Lat: lat, Lon: lon} }
	bos, lga, ewr, mco := airport(42.3643, -71.0052), airport(40.7769, -73.874), airport(40.6925, -74.1687), airport(28.4294, -81.309)
	ytz, lhr, yyz := airport(43.6275, -79.3962), airport(51.47, -0.4543), airport(43.6772, -79.6306)
	jfk, hnd, ams, sfo := airport(40.6398, -73.7789), airport(35.5523, 139.78), airport(52.3086, 4.7639), airport(37.619, -122.375)
	dfw, dca, yqy, yqb := airport(32.8968, -97.038), airport(38.8521, -77.0377), airport(46.1614, -60.0478), airport(46.7911, -71.3933)
	cun, yow := airport(21.0365, -86.8771), airport(45.3225, -75.6692)
	// Over Lake Ontario, and round Pearson, as on 2026-10-30.
	lake := geo.Point{Lat: 43.45, Lon: -79.30}
	nearYYZ := geo.Point{Lat: 43.62, Lon: -79.50} // 12 km south-east of Pearson
	for _, c := range []struct {
		name     string
		at       geo.Point
		alt, fpm int
		ground   bool
		from, to events.Airport
		keep     bool
	}{
		{"UAL397 Newark to Orlando, over the lake", lake, 35000, 0, false, ewr, mco, false},
		{"RPA5603 Boston to LaGuardia, over the lake", lake, 33000, 0, false, bos, lga, false},
		{"PTR2943 Boston to Billy Bishop, coming down", lake, 3000, -700, false, bos, ytz, true},
		{"ACA855 Heathrow to Pearson, coming down", lake, 5000, -900, false, lhr, yyz, true},
		{"JAL5 JFK to Haneda, cruising over Canada", lake, 37000, 0, false, jfk, hnd, true},
		{"a Pearson departure climbing out", nearYYZ, 6000, 2000, false, yyz, lhr, true},
		{"no origin: nothing to check", lake, 3000, 0, false, events.Airport{}, mco, true},
		// The six the independent check found wrong.
		{"KLM147 'Pearson to Amsterdam' landing at Pearson", nearYYZ, 3000, -800, false, yyz, ams, false},
		{"UAL1172 'Boston to San Francisco' landing at Pearson", nearYYZ, 3700, -600, false, bos, sfo, false},
		{"AAL1876 'Dallas to Washington' on the ground at Pearson", geo.Point{Lat: 43.68, Lon: -79.62}, 0, 0, true, dfw, dca, false},
		{"JZA482 'Pearson to Sydney NS' cruising over Pearson", nearYYZ, 35000, 0, false, yyz, yqy, false},
		{"ROU1798 'Quebec to Cancun' climbing out of Toronto", nearYYZ, 7875, 1500, false, yqb, cun, false},
		{"POE505 'Ottawa to Orlando' parked at Pearson", geo.Point{Lat: 43.685, Lon: -79.625}, 0, 0, true, yow, mco, false},
	} {
		s := &State{Center: pearson, RadiusKm: 60}
		p := pos("aaaaaa", "XXX1", c.at.Lat, t0)
		p.Lon, p.OnGround = c.at.Lon, c.ground
		alt, fpm := c.alt, c.fpm
		if c.ground {
			p.AltFt = nil
		} else {
			p.AltFt, p.VertRateFpm = &alt, &fpm
		}
		s.Update(p)
		s.SetAircraft(events.Aircraft{Hex: "aaaaaa", Route: &events.Route{Callsign: "XXX1", Origin: c.from, Destination: c.to}})
		if got := s.View(t0).Planes[0].Info.Route != nil; got != c.keep {
			t.Errorf("%s: route shown = %v, want %v", c.name, got, c.keep)
		}
	}
}

func TestETAOnlyWhileHeadingIn(t *testing.T) {
	yyz := events.Airport{Lat: 43.6772, Lon: -79.6306}
	r := &events.Route{Callsign: "WJA712", Destination: yyz}
	p := pos("aaaaaa", "WJA712", 43.80, t0).Position // 13.7 km north of Pearson
	south, east := 190.0, 90.0
	p.TrackDeg = &south
	if _, m := eta(p, r); m == nil {
		t.Fatal("heading for Pearson: no time to landing")
	}
	p.TrackDeg = &east // on a downwind leg, vectored round
	if km, m := eta(p, r); m != nil || km == nil {
		t.Fatalf("on a downwind leg: time %v, distance %v", m, km)
	}
}

func TestCoverage(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30}
	// Long pass with type (from the feed) and route (from enrichment).
	a := pos("aaaaaa", "ACA101", 43.70, t0)
	a.TypeCode = "A320"
	s.Update(a)
	s.SetAircraft(events.Aircraft{Hex: "aaaaaa", Route: &events.Route{Callsign: "ACA101"}})
	a.At = t0.Add(2 * time.Minute)
	s.Update(a)
	// Long pass, type known, route unknown.
	b := pos("bbbbbb", "WJA9", 43.75, t0)
	b.TypeCode = "B38M"
	s.Update(b)
	b.At = t0.Add(90 * time.Second)
	s.Update(b)
	// Short pass: not counted. Private callsign: not an airline flight.
	s.Update(pos("cccccc", "POE1", 43.71, t0))
	s.Update(pos("dddddd", "N123AB", 43.72, t0))
	d := pos("dddddd", "N123AB", 43.72, t0.Add(5*time.Minute))
	s.Update(d)

	c := s.Coverage()
	if c.Flights != 2 || c.WithType != 2 || c.WithRoute != 1 || c.WithBoth != 1 || c.Share != 0.5 {
		t.Fatalf("coverage = %+v", c)
	}
	if len(c.Missing) != 1 || c.Missing[0] != "WJA9 (bbbbbb)" {
		t.Fatalf("missing = %v", c.Missing)
	}
}

func TestServerNowAndStream(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30}
	s.Update(pos("aaaaaa", "ACA1", 43.70, t0))
	srv := &Server{State: s, Every: 20 * time.Millisecond, Now: func() time.Time { return t0 }}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)

	resp, err := http.Get(ts.URL + "/api/now")
	if err != nil {
		t.Fatal(err)
	}
	var v View
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil || len(v.Planes) != 1 {
		t.Fatalf("/api/now: %+v %v", v, err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("no CORS header")
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	snapshots := 0
	for sc.Scan() && snapshots < 3 {
		line := sc.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var v View
			if err := json.Unmarshal([]byte(data), &v); err != nil || len(v.Planes) != 1 {
				t.Fatalf("bad snapshot %q: %v", data, err)
			}
			snapshots++
		}
	}
	if err := sc.Err(); err != nil && snapshots < 3 {
		t.Fatalf("reading the stream: %v", err)
	}
	if snapshots < 3 {
		t.Fatalf("got %d snapshots: the first at once, then one per tick", snapshots)
	}
	if srv.Clients() != 1 {
		t.Fatalf("Clients = %d", srv.Clients())
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for srv.Clients() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Clients() != 0 {
		t.Fatal("a closed stream was not removed")
	}
}

func TestPruneForgetsWhatIsLongGone(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30}
	s.Update(pos("aaaaaa", "ACA1", 43.70, t0))
	s.SetAircraft(events.Aircraft{Hex: "aaaaaa", TypeCode: "A320", Looked: t0})
	s.SetAircraft(events.Aircraft{Hex: "gone00", TypeCode: "B38M", Looked: t0})
	s.View(t0.Add(7 * time.Hour)) // aaaaaa's position is stale by now too
	if s.HasInfo("gone00") || s.HasInfo("aaaaaa") {
		t.Fatal("enrichment for aircraft gone six hours was kept")
	}
	s.Update(pos("bbbbbb", "ACA2", 43.70, t0.Add(7*time.Hour)))
	s.SetAircraft(events.Aircraft{Hex: "bbbbbb", Looked: t0})
	s.View(t0.Add(7*time.Hour + 2*time.Minute))
	if !s.HasInfo("bbbbbb") {
		t.Fatal("enrichment for an aircraft still in the picture was dropped")
	}
}

func TestNotable(t *testing.T) {
	tests := []struct {
		p    feeds.Position
		want string
	}{
		{feeds.Position{Squawk: "7700", Military: true}, "Squawking 7700: emergency"},
		{feeds.Position{Squawk: "7600"}, "Squawking 7600: radio failure"},
		{feeds.Position{Emergency: "lifeguard"}, "Declared emergency: lifeguard"},
		{feeds.Position{TypeCode: "A124", Military: true}, "Rare visitor: Antonov An-124 Ruslan"},
		{feeds.Position{Military: true, TypeCode: "C17"}, "Military aircraft"},
		{feeds.Position{TypeCode: "A320", Squawk: "1234"}, ""},
	}
	for _, tt := range tests {
		if got := notable(tt.p); got != tt.want {
			t.Errorf("notable(%+v) = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestCommercial(t *testing.T) {
	for cs, want := range map[string]bool{"ACA101": true, "POE672": true, "WJA9": true, "JZA8Q": true, "N123AB": false, "CGXYZ": false, "": false} {
		if got := Commercial(feeds.Position{Callsign: cs}); got != want {
			t.Errorf("Commercial(%q) = %v, want %v", cs, got, want)
		}
	}
}

// A public stream decodes as gzip from any point, one standalone block per snapshot, and the first snapshot arrives
// before the stream is ever finished: a browser reads it as it comes.
func TestPublicStreamIsGzipFromTheFirstSnapshot(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30, Public: true}
	s.Update(pos("aaaaaa", "ACA1", 43.70, t0))
	s.Update(pos("bbbbbb", "N123AB", 43.70, t0))
	srv := &Server{State: s, Public: true, Every: 20 * time.Millisecond, Now: func() time.Time { return t0 }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	req.Header.Set("Accept-Encoding", "gzip") // set by hand, so the transport leaves the body compressed
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q", got)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(nil, 1<<20)
	snapshots := 0
	for snapshots < 3 && sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var v View
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
			t.Fatalf("snapshot %d: %v", snapshots, err)
		}
		if !v.Public || len(v.Planes) != 1 || v.Planes[0].Hex != "aaaaaa" {
			t.Fatalf("snapshot %d: public %v, planes %+v", snapshots, v.Public, v.Planes)
		}
		snapshots++
	}
	if snapshots < 3 {
		t.Fatalf("got %d snapshots, err %v", snapshots, sc.Err())
	}
}

func TestPublicStreamWithoutGzipIsPlain(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30, Public: true}
	srv := &Server{State: s, Public: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	if !sc.Scan() || sc.Text() != "retry: 3000" || resp.Header.Get("Content-Encoding") != "" {
		t.Fatalf("first line %q, encoding %q", sc.Text(), resp.Header.Get("Content-Encoding"))
	}
}

// The home instance streams as it always has, whatever the browser accepts: TV browsers are old.
func TestHomeStreamIsNeverCompressed(t *testing.T) {
	srv := &Server{State: &State{Center: pearson, RadiusKm: 30}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != "" {
		t.Fatalf("the home instance got the public headers: %q", got)
	}
}

func TestPublicStreamsAreCapped(t *testing.T) {
	srv := &Server{State: &State{Center: pearson, RadiusKm: 30}, Public: true, MaxStreams: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	open := func() *http.Response {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	first := open()
	defer first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first stream: %s", first.Status)
	}
	second := open()
	defer second.Body.Close()
	if second.StatusCode != http.StatusServiceUnavailable || second.Header.Get("Retry-After") == "" {
		t.Fatalf("second stream: %s, Retry-After %q", second.Status, second.Header.Get("Retry-After"))
	}
	if got := second.Header.Get("Content-Security-Policy"); got == "" {
		t.Fatal("no Content-Security-Policy on the public instance")
	}
}

// /api/now on the public instance answers from the shared snapshot: asking does no work, however often it is asked.
func TestPublicNowAnswersFromTheSnapshot(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30, Public: true}
	s.Update(pos("aaaaaa", "ACA1", 43.70, t0))
	srv := &Server{State: s, Public: true, Every: time.Hour, Now: func() time.Time { return t0 }}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	get := func() View {
		resp, err := http.Get(ts.URL + "/api/now")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v View
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := get(); len(v.Planes) != 1 {
		t.Fatalf("planes = %d", len(v.Planes))
	}
	s.Update(pos("cccccc", "ACA2", 43.70, t0))
	if v := get(); len(v.Planes) != 1 {
		t.Fatalf("planes = %d: the answer was worked out again within the tick", len(v.Planes))
	}
}

func TestSourcesCreditEveryFeedThatReported(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 30, Public: true}
	a, b := pos("aaaaaa", "ACA1", 43.70, t0), pos("bbbbbb", "N123AB", 43.70, t0)
	b.Source = "adsb.lol" // the only report from this feed, of a plane the public view leaves out
	s.Update(a)
	s.Update(b)
	v := s.View(t0)
	if len(v.Planes) != 1 || !slices.Equal(v.Sources, []string{"adsb.fi", "adsb.lol"}) {
		t.Fatalf("planes %d, sources %v", len(v.Planes), v.Sources)
	}
}

func TestStaticNeverListsADirectory(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "home", "_next/static/app.js": "js", "empty/.keep": ""} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(Static(dir))
	defer ts.Close()
	for path, want := range map[string]int{
		"/": http.StatusOK, "/_next/static/app.js": http.StatusOK,
		"/_next/static/": http.StatusNotFound, "/empty/": http.StatusNotFound, "/nothing": http.StatusNotFound,
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %s, want %d", path, resp.Status, want)
		}
	}
	resp, err := http.Get(ts.URL + "/_next/static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
}

// Routes and photos stay off a public page until their sources allow them; a route still places the plane.
func TestPublicViewHoldsBackRoutesAndPhotos(t *testing.T) {
	route := &events.Route{Callsign: "ACA1", Airline: "Air Canada",
		Origin:      events.Airport{ICAO: "CYUL", Lat: 45.47, Lon: -73.74},
		Destination: events.Airport{ICAO: "CYYZ", Lat: 43.6777, Lon: -79.6248}}
	photo := &events.Photo{Thumbnail: "https://t.plnspttrs.net/x.jpg", Link: "https://www.planespotters.net/photo/1", Photographer: "A"}
	air := events.Aircraft{Hex: "aaaaaa", TypeName: "A320", Owner: "Air Canada", Route: route, Photo: photo, Looked: t0}
	view := func(s *State) Plane {
		t.Helper()
		s.Update(pos("aaaaaa", "ACA1", 43.70, t0))
		s.SetAircraft(air)
		v := s.View(t0)
		if len(v.Planes) != 1 || v.Planes[0].Info == nil {
			t.Fatalf("planes = %+v", v.Planes)
		}
		return v.Planes[0]
	}

	p := view(&State{Center: pearson, RadiusKm: 30, Public: true})
	if p.Info.Route != nil || p.Info.Photo != nil || p.ToDestinationKm != nil || p.ETAMin != nil {
		t.Fatalf("public plane = %+v, info %+v", p, p.Info)
	}
	if p.Info.Owner != "Air Canada" || p.Info.TypeName != "A320" {
		t.Fatalf("the aircraft's own details went too: %+v", p.Info)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"CYUL", "CYYZ", "plnspttrs", "planespotters"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("%s in the public JSON: %s", leak, raw)
		}
	}

	if p := view(&State{Center: pearson, RadiusKm: 30, Public: true, PublicRoutes: true, PublicPhotos: true}); p.Info.Route == nil || p.Info.Photo == nil {
		t.Fatalf("with permission: %+v", p.Info)
	}
	if p := view(&State{Center: pearson, RadiusKm: 30}); p.Info.Route == nil || p.Info.Photo == nil {
		t.Fatalf("at home: %+v", p.Info)
	}
}

// A focused public view sends only what the screen draws: the planes in focus and those on the airport's board.
func TestPublicFocusedViewDropsPlanesNeverDrawn(t *testing.T) {
	ap := Airports["CYTZ"]
	s := &State{Center: geo.Point{Lat: 43.6426, Lon: -79.3871}, RadiusKm: 60, Public: true, Airport: &ap, NearKm: 3}
	alt, gs, far := 3000, 180.0, 36000
	s.Update(events.Position{Position: feeds.Position{Hex: "aaaaaa", Callsign: "ACA1", Lat: 43.65, Lon: -79.39, AltFt: &alt, SpeedKt: &gs, Source: "adsb.lol", At: t0}})
	s.Update(events.Position{Position: feeds.Position{Hex: "bbbbbb", Callsign: "ACA2", Lat: 44.0, Lon: -79.8, AltFt: &far, SpeedKt: &gs, Source: "adsb.lol", At: t0}})
	v := s.View(t0)
	if len(v.Planes) != 1 || v.Planes[0].Hex != "aaaaaa" {
		t.Fatalf("planes = %+v", v.Planes)
	}
}
