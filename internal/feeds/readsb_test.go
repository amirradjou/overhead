package feeds

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/geo"
)

var pearson = geo.Point{Lat: 43.6777, Lon: -79.6248}

// fixtureNow is the "now" in testdata/point.json.
var fixtureNow = time.UnixMilli(1793324881501)

func serve(t *testing.T, status int, header http.Header, body []byte) (*Readsb, *http.Request) {
	t.Helper()
	got := new(http.Request)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.Clone(context.Background())
		maps.Copy(w.Header(), header)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	feed := NewReadsb("test", srv.URL+"/v2/point/%.5f/%.5f/%d", srv.Client(), "overhead/test")
	feed.clock = func() time.Time { return fixtureNow.Add(time.Second) }
	return feed, got
}

func TestFetchNormalises(t *testing.T) {
	body, err := os.ReadFile("testdata/point.json")
	if err != nil {
		t.Fatal(err)
	}
	feed, req := serve(t, http.StatusOK, nil, body)

	got, err := feed.Fetch(context.Background(), pearson, 30)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v2/point/43.67770/-79.62480/17" {
		t.Errorf("path = %q; 30 km should be asked as 17 nm", req.URL.Path)
	}
	if ua := req.Header.Get("User-Agent"); ua != "overhead/test" {
		t.Errorf("User-Agent = %q", ua)
	}

	byHex := make(map[string]Position)
	for _, p := range got {
		byHex[p.Hex] = p
	}
	// c0ffee has no position, a0b1c2 is 58 km north and c1e9ce is an airport tug (category C2): all dropped.
	for _, hex := range []string{"c0ffee", "a0b1c2", "c1e9ce"} {
		if _, ok := byHex[hex]; ok {
			t.Errorf("%s should be dropped", hex)
		}
	}
	if len(got) != 6 {
		t.Fatalf("got %d positions, want 6: %+v", len(got), got)
	}

	poe := byHex["c08861"]
	if poe.Callsign != "POE672" || poe.Registration != "C-GZQU" || poe.TypeCode != "E295" || poe.Emergency != "" {
		t.Errorf("identity = %+v", poe)
	}
	if poe.AltFt == nil || *poe.AltFt != 9700 || poe.OnGround {
		t.Errorf("altitude = %v ground=%v, want 9700 from alt_baro", poe.AltFt, poe.OnGround)
	}
	if poe.VertRateFpm == nil || *poe.VertRateFpm != -1408 {
		t.Errorf("vertical rate = %v, want baro_rate -1408", poe.VertRateFpm)
	}
	if want := fixtureNow.Add(-500 * time.Millisecond); !poe.At.Equal(want) {
		t.Errorf("At = %v, want now - seen_pos = %v", poe.At, want)
	}
	if poe.Source != "test" || poe.SourceType != "adsb_icao" {
		t.Errorf("source = %q/%q", poe.Source, poe.SourceType)
	}

	aca, ok := byHex["c0174f"]
	if !ok {
		t.Fatal("C0174F should be stored lowercase")
	}
	if !aca.OnGround || aca.AltFt != nil {
		t.Errorf("ACA101 on the ground: alt %v ground %v", aca.AltFt, aca.OnGround)
	}

	if _, ok := byHex["~2a1b3c"]; !ok {
		t.Error("TIS-B ~ address should be kept as is")
	}

	rch := byHex["ae1234"]
	if !rch.Military || rch.Emergency != "general" || rch.Squawk != "7700" {
		t.Errorf("military emergency = %+v", rch)
	}
	if rch.AltFt == nil || *rch.AltFt != 3000 || rch.VertRateFpm == nil || *rch.VertRateFpm != 640 {
		t.Errorf("without baro data use geometric: alt %v rate %v", rch.AltFt, rch.VertRateFpm)
	}

	if wja := byHex["c0aaaa"]; wja.AltFt == nil || *wja.AltFt != 5025 || wja.OnGround {
		t.Errorf("alt_baro null should fall back to alt_geom 5025, got %v", wja.AltFt)
	}

	priv := byHex["a12345"]
	if !priv.Private {
		t.Error("dbFlags LADD should mark the aircraft private")
	}
	if !priv.At.Equal(fixtureNow) {
		t.Errorf("no seen_pos or seen: At = %v, want the response time", priv.At)
	}
}

func TestFetchStatusError(t *testing.T) {
	feed, _ := serve(t, http.StatusForbidden, nil, []byte(`{"error":"Please contact us at contact@example.org."}`))
	_, err := feed.Fetch(context.Background(), pearson, 30)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 403 || se.Body != "Please contact us at contact@example.org." {
		t.Fatalf("err = %v", err)
	}

	feed, _ = serve(t, http.StatusTooManyRequests, http.Header{"Retry-After": {"90"}}, nil)
	_, err = feed.Fetch(context.Background(), pearson, 30)
	if !errors.As(err, &se) || se.Code != 429 || se.RetryAfter != 90*time.Second {
		t.Fatalf("err = %v (%+v)", err, se)
	}
}

func TestFetchRejectsUselessAnswers(t *testing.T) {
	tests := map[string]string{
		"no aircraft list": `{"msg":"overloaded","now":1793324881501}`,
		"null list":        `{"ac":null,"now":1793324881501}`,
		"no clock":         `{"ac":[]}`,
		"frozen snapshot":  `{"ac":[],"now":1793324581501}`, // five minutes behind
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			feed, _ := serve(t, http.StatusOK, nil, []byte(body))
			if got, err := feed.Fetch(context.Background(), pearson, 30); err == nil {
				t.Fatalf("want an error, got %d positions", len(got))
			}
		})
	}

	feed, _ := serve(t, http.StatusOK, nil, []byte(`{"ac":[],"now":1793324881501,"msg":"No error"}`))
	if got, err := feed.Fetch(context.Background(), pearson, 30); err != nil || len(got) != 0 {
		t.Fatalf("an empty sky is an answer: %v, %v", got, err)
	}
}

func TestFetchBadJSON(t *testing.T) {
	feed, _ := serve(t, http.StatusOK, nil, []byte(`<html>maintenance</html>`))
	if _, err := feed.Fetch(context.Background(), pearson, 30); err == nil {
		t.Fatal("want a decode error")
	}
}

func TestNowInSeconds(t *testing.T) {
	lat, lon := 43.6, -79.6
	b := readsbResponse{Now: 1793324881.5, AC: []readsbAircraft{{Hex: "abc123", Lat: &lat, Lon: &lon}}}
	got, err := b.positions("x", pearson, 30, fixtureNow)
	if err != nil || len(got) != 1 || !got[0].At.Equal(time.UnixMilli(1793324881500)) {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestFeedTextCannotDriveTheTerminal(t *testing.T) {
	lat, lon := 43.6, -79.6
	b := readsbResponse{Now: 1793324881501, AC: []readsbAircraft{{
		Hex: "abc123", Flight: "ACA1\x1b[2J\x1b]0;pwned\x07  ", R: "C-\u009bGZQU", Lat: &lat, Lon: &lon,
	}}}
	got, err := b.positions("x", pearson, 30, fixtureNow)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got[0].Callsign != "ACA1[2J]0;pwned" || got[0].Registration != "C-GZQU" {
		t.Fatalf("control characters kept: %q %q", got[0].Callsign, got[0].Registration)
	}
	if r := refusal([]byte("{\"error\":\"no\x1b[31m way\"}")); strings.ContainsRune(r, 0x1b) {
		t.Fatalf("refusal kept an escape: %q", r)
	}
}

func TestCallsign(t *testing.T) {
	for in, want := range map[string]string{"ACA101  ": "ACA101", "@@@@@@@@": "", "ACA1@@  ": "", "": "", "  ": "", "c-gabc": "", "N123AB": "N123AB"} {
		if got := callsign(in); got != want {
			t.Errorf("callsign(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSurface(t *testing.T) {
	for cat, want := range map[string]bool{"C1": true, "C2": true, "C7": true, "C0": false, "A3": false, "B6": false, "": false} {
		if got := surface(cat); got != want {
			t.Errorf("surface(%q) = %v, want %v", cat, got, want)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tests := map[string]time.Duration{
		"":                              0,
		"120":                           2 * time.Minute,
		"soon":                          0,
		"Thu, 29 Oct 2026 12:00:30 GMT": 30 * time.Second,
		"Thu, 29 Oct 2026 11:00:00 GMT": 0,
	}
	for h, want := range tests {
		if got := retryAfter(h, now); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", h, got, want)
		}
	}
}

func TestByName(t *testing.T) {
	for _, name := range Names() {
		f, err := ByName(name, http.DefaultClient, "ua")
		if err != nil || f.Name() != name {
			t.Fatalf("ByName(%q) = %v, %v", name, f, err)
		}
	}
	if _, err := ByName("flightradar", http.DefaultClient, "ua"); err == nil {
		t.Fatal("unknown feed should fail")
	}
	if Spacing("adsb.lol") < 10*time.Second || Spacing("adsb.fi") != 0 || Spacing("flightradar") != 0 {
		t.Fatal("adsb.lol needs requests spaced out; the others are fine at the poll interval")
	}
}
