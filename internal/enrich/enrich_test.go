package enrich

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/events"
)

// Answers as the real APIs gave them on 2026-10-29.
const (
	adsbdbBoth = `{"response":{"aircraft":{"type":"ERJ 190-400","icao_type":"E295","manufacturer":"Embraer","mode_s":"C08861","registration":"CA-GZQU","registered_owner_country_iso_name":"CA","registered_owner_country_name":"Canada","registered_owner_operator_flag_code":null,"registered_owner":"Porter Airlines (Canada) Limited","url_photo":null,"url_photo_thumbnail":null},"flightroute":{"callsign":"POE672","callsign_icao":"POE672","callsign_iata":"PD672","airline":{"name":"Porter Airlines","icao":"POE","iata":"PD","country":"Canada","country_iso":"CA","callsign":"PORTER AIR"},"origin":{"country_iso_name":"US","country_name":"United States","elevation":13,"iata_code":"SFO","icao_code":"KSFO","latitude":37.619,"longitude":-122.375,"municipality":"San Francisco","name":"San Francisco International Airport"},"destination":{"country_iso_name":"CA","country_name":"Canada","elevation":569,"iata_code":"YYZ","icao_code":"CYYZ","latitude":43.6772,"longitude":-79.6306,"municipality":"Toronto","name":"Lester B. Pearson International Airport"}}}}`
	photos     = `{"photos":[{"id":"1962638","thumbnail":{"src":"https://t.plnspttrs.net/01058/1962638_9d18523526_t.jpg","size":{"width":200,"height":133}},"thumbnail_large":{"src":"https://t.plnspttrs.net/01058/1962638_9d18523526_280.jpg","size":{"width":420,"height":280}},"link":"https://www.planespotters.net/photo/1962638/c-gzqu?utm_source=api","photographer":"Frederick Tremblay"}]}`
)

func apiServer(t *testing.T, routes map[string]func(http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := routes[r.URL.RequestURI()]; ok {
			h(w)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"response":"unknown aircraft"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func body(s string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { _, _ = w.Write([]byte(s)) }
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func TestADSBDB(t *testing.T) {
	srv := apiServer(t, map[string]func(http.ResponseWriter){
		"/v0/aircraft/C08861?callsign=POE672": body(adsbdbBoth),
		"/v0/aircraft/C0BUSY":                 status(http.StatusTooManyRequests),
		"/v0/callsign/ZZZ9999": func(w http.ResponseWriter) {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"response":"unknown callsign"}`))
		},
	})
	a := NewADSBDB(srv.URL, srv.Client(), "test")
	a.c.pace.every = 0
	ctx := context.Background()

	ac, route, settled, err := a.Aircraft(ctx, "c08861", "POE672")
	if err != nil || ac == nil || route == nil || !settled {
		t.Fatalf("got %+v %+v %v %v", ac, route, settled, err)
	}
	if ac.TypeCode != "E295" || ac.TypeName != "ERJ 190-400" || ac.Owner != "Porter Airlines (Canada) Limited" {
		t.Errorf("aircraft = %+v", ac)
	}
	if route.Airline != "Porter Airlines" || route.Origin.IATA != "SFO" || route.Destination.ICAO != "CYYZ" || route.Destination.Lat != 43.6772 {
		t.Errorf("route = %+v", route)
	}

	ac, route, settled, err = a.Aircraft(ctx, "aaaaaa", "POE672")
	if err != nil || ac != nil || route != nil || settled {
		t.Fatalf("unknown airframe: %+v %+v settled=%v %v; adsbdb never looked at the callsign", ac, route, settled, err)
	}

	if _, _, _, err = a.Aircraft(ctx, "c0busy", ""); !Retryable(err) {
		t.Fatalf("429 should be retryable: %v", err)
	}
	if r, err := a.Route(ctx, "ZZZ9999"); err != nil || r != nil {
		t.Fatalf("unknown callsign: %+v %v", r, err)
	}
}

func TestPlanespotters(t *testing.T) {
	srv := apiServer(t, map[string]func(http.ResponseWriter){
		"/pub/photos/hex/C08861": body(photos),
		"/pub/photos/hex/C0NONE": body(`{"photos":[]}`),
		"/pub/photos/hex/C0ANON": body(`{"photos":[{"thumbnail":{"src":"x.jpg"},"link":"https://example.org"}]}`),
	})
	p := NewPlanespotters(srv.URL, srv.Client(), "test")
	p.c.pace.every = 0
	ctx := context.Background()
	ph, err := p.Photo(ctx, "c08861")
	if err != nil || ph == nil || ph.Photographer != "Frederick Tremblay" || !strings.Contains(ph.Link, "planespotters.net/photo/1962638") {
		t.Fatalf("photo = %+v, %v", ph, err)
	}
	for _, hex := range []string{"c0none", "c0anon"} {
		if ph, err := p.Photo(ctx, hex); err != nil || ph != nil {
			t.Errorf("%s: no creditable photo, got %+v %v", hex, ph, err)
		}
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{&HTTPError{Code: 429}, true},
		{&HTTPError{Code: 503}, true},
		{&HTTPError{Code: 400}, false},
		{&decodeError{errors.New("bad")}, false},
		{errors.New("connection reset"), true},
		{context.Canceled, false},
		{errors.Join(&decodeError{errors.New("bad")}, &HTTPError{Code: 429}), true},
		{nil, false},
	}
	for _, tt := range tests {
		if got := Retryable(tt.err); got != tt.want {
			t.Errorf("Retryable(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

// fakeAPIs answer from maps and count calls.
type fakeAPIs struct {
	aircraft map[string]*events.Aircraft
	routes   map[string]*events.Route
	photos   map[string]*events.Photo
	photoErr error
	calls    map[string]int
}

func (f *fakeAPIs) Aircraft(_ context.Context, hex, callsign string) (*events.Aircraft, *events.Route, bool, error) {
	f.calls["aircraft"]++
	ac, ok := f.aircraft[hex]
	if !ok {
		return nil, nil, false, nil
	}
	if callsign == "" {
		return ac, nil, false, nil
	}
	return ac, f.routes[callsign], true, nil
}

func (f *fakeAPIs) Route(_ context.Context, callsign string) (*events.Route, error) {
	f.calls["route"]++
	return f.routes[callsign], nil
}

func (f *fakeAPIs) Photo(_ context.Context, hex string) (*events.Photo, error) {
	f.calls["photo"]++
	if f.photoErr != nil {
		return nil, f.photoErr
	}
	return f.photos[hex], nil
}

func newWorker(t *testing.T) (*Worker, *fakeAPIs, *time.Time) {
	t.Helper()
	cache, err := OpenCache(filepath.Join(t.TempDir(), "enrich.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })
	now := time.Date(2026, 10, 29, 22, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	apis := &fakeAPIs{
		aircraft: map[string]*events.Aircraft{"c08861": {TypeCode: "E295", Owner: "Porter"}},
		routes: map[string]*events.Route{
			"POE672": {Callsign: "POE672", Airline: "Porter Airlines"},
			"POE111": {Callsign: "POE111"},
			"ACA101": {Callsign: "ACA101"},
		},
		photos: map[string]*events.Photo{"c08861": {Thumbnail: "t.jpg", Link: "l", Photographer: "p"}},
		calls:  map[string]int{},
	}
	return &Worker{Aircraft: apis, Photos: apis, Cache: cache}, apis, &now
}

func TestWorkerCallsEachAPIOncePerNewPlane(t *testing.T) {
	w, apis, _ := newWorker(t)
	ctx := context.Background()
	job := events.EnrichJob{Hex: "c08861", Callsign: "POE672"}

	got, err := w.Enrich(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hex != "c08861" || got.TypeCode != "E295" || got.Route == nil || got.Route.Airline != "Porter Airlines" || got.Photo == nil {
		t.Fatalf("result = %+v", got)
	}
	if apis.calls["aircraft"] != 1 || apis.calls["route"] != 0 || apis.calls["photo"] != 1 {
		t.Fatalf("calls = %v: airframe and route come in one adsbdb call", apis.calls)
	}

	// Every later poll's job is answered from the cache.
	for range 5 {
		if _, err := w.Enrich(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	// Next flight of the same airframe: only the route is new.
	if _, err := w.Enrich(ctx, events.EnrichJob{Hex: "c08861", Callsign: "POE111"}); err != nil {
		t.Fatal(err)
	}
	c := w.Counts()
	if c.Calls != 3 || c.NewAircraft != 1 || apis.calls["route"] != 1 {
		t.Fatalf("counts = %+v, calls = %v", c, apis.calls)
	}
}

func TestWorkerUnknownAirframeStillGetsItsRoute(t *testing.T) {
	w, apis, _ := newWorker(t)
	ctx := context.Background()
	job := events.EnrichJob{Hex: "aaaaaa", Callsign: "ACA101"}
	got, err := w.Enrich(ctx, job)
	if err != nil || got.TypeCode != "" || got.Route == nil || got.Route.Callsign != "ACA101" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if apis.calls["aircraft"] != 1 || apis.calls["route"] != 1 || apis.calls["photo"] != 1 {
		t.Fatalf("calls = %v", apis.calls)
	}
	// "Unknown" is cached too.
	if _, err := w.Enrich(ctx, job); err != nil {
		t.Fatal(err)
	}
	if w.Counts().Calls != 3 {
		t.Fatalf("an unknown airframe was looked up again: %v", apis.calls)
	}
}

func TestWorkerCachedUnknownsStayUnknown(t *testing.T) {
	w, apis, _ := newWorker(t)
	ctx := context.Background()
	// An airframe adsbdb doesn't know, a callsign without a route, no photo: asked twice, the second from the cache.
	job := events.EnrichJob{Hex: "aaaaaa", Callsign: "ZZZ999"}
	for i := range 2 {
		got, err := w.Enrich(ctx, job)
		if err != nil {
			t.Fatal(err)
		}
		if got.TypeCode != "" || got.Route != nil || got.Photo != nil {
			t.Fatalf("ask %d: unknowns came back as empty values: %+v route=%+v photo=%+v", i+1, got, got.Route, got.Photo)
		}
	}
	if apis.calls["aircraft"] != 1 || apis.calls["route"] != 1 || apis.calls["photo"] != 1 {
		t.Fatalf("calls = %v: the second ask should be answered from the cache", apis.calls)
	}
}

func TestWorkerPartialResultOnPhotoFailure(t *testing.T) {
	w, apis, _ := newWorker(t)
	ctx := context.Background()
	apis.photoErr = &HTTPError{API: "planespotters", Code: 429}
	job := events.EnrichJob{Hex: "c08861", Callsign: "POE672"}

	got, err := w.Enrich(ctx, job)
	if !Retryable(err) || got.TypeCode != "E295" || got.Route == nil || got.Photo != nil {
		t.Fatalf("want type and route despite the photo failing: %+v, %v", got, err)
	}
	apis.photoErr = nil
	got, err = w.Enrich(ctx, job)
	if err != nil || got.Photo == nil {
		t.Fatalf("retry: %+v %v", got, err)
	}
	if apis.calls["aircraft"] != 1 || apis.calls["photo"] != 2 {
		t.Fatalf("the retry should cost only the photo call: %v", apis.calls)
	}
}

func TestWorkerRoutesExpireAfterADay(t *testing.T) {
	w, apis, now := newWorker(t)
	ctx := context.Background()
	job := events.EnrichJob{Hex: "c08861", Callsign: "POE672"}
	if _, err := w.Enrich(ctx, job); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(25 * time.Hour)
	if _, err := w.Enrich(ctx, job); err != nil {
		t.Fatal(err)
	}
	if apis.calls["aircraft"] != 1 || apis.calls["route"] != 1 || apis.calls["photo"] != 1 {
		t.Fatalf("after a day only the route is stale: %v", apis.calls)
	}
	if n, err := w.Cache.Purge(); err != nil || n != 0 {
		t.Fatalf("Purge = %d, %v; nothing should be expired after the refresh", n, err)
	}
	*now = now.Add(31 * 24 * time.Hour)
	if n, err := w.Cache.Purge(); err != nil || n != 3 {
		t.Fatalf("Purge = %d, %v; want airframe, route and photo gone", n, err)
	}
}

func TestConsumerMessages(t *testing.T) {
	c := &Consumer{}
	job := events.EnrichJob{Hex: "c08861"}
	d := amqp.Delivery{Body: []byte(`{"hex":"c08861"}`)}

	msgs, _ := c.messages(d, job, events.Aircraft{Hex: "c08861"}, nil)
	if len(msgs) != 1 || msgs[0].Exchange != bus.ExchangeAircraft {
		t.Fatalf("success: %+v", msgs)
	}

	msgs, _ = c.messages(d, job, events.Aircraft{Hex: "c08861"}, &HTTPError{Code: 429})
	if len(msgs) != 2 || msgs[1].Exchange != bus.ExchangeEnrichRetry || bus.Attempts(msgs[1].Headers) != 1 {
		t.Fatalf("first failure: %+v", msgs)
	}

	d.Headers = amqp.Table{bus.HeaderAttempts: int32(bus.MaxAttempts - 1)}
	msgs, _ = c.messages(d, job, events.Aircraft{}, &HTTPError{Code: 429})
	if msgs[1].Exchange != bus.ExchangeEnrichDead || bus.Attempts(msgs[1].Headers) != bus.MaxAttempts {
		t.Fatalf("fifth failure should be parked: %+v", msgs[1])
	}

	d.Headers = nil
	msgs, _ = c.messages(d, job, events.Aircraft{}, &decodeError{errors.New("bad json")})
	if msgs[1].Exchange != bus.ExchangeEnrichDead {
		t.Fatalf("a failure that won't pass goes straight to the dead-letter queue: %+v", msgs[1])
	}
}
