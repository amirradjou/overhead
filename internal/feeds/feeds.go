// Package feeds reads live aircraft positions from community flight-tracking networks and normalises them into one
// Position event, whichever network sent it.
package feeds

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/amirradjou/overhead/internal/geo"
)

// Position is one aircraft's position report. It is the event the poller publishes (docs/SPEC.md: Event flow), so its JSON
// form is the wire format.
type Position struct {
	// Hex is the ICAO 24-bit address in lowercase hex; a "~" prefix marks a non-ICAO address (TIS-B).
	Hex          string `json:"hex"`
	Callsign     string `json:"callsign,omitempty"`
	Registration string `json:"registration,omitempty"`
	// TypeCode is the ICAO type designator, e.g. A21N.
	TypeCode string `json:"type_code,omitempty"`
	// Category is the ADS-B emitter category, e.g. A3 for a large aircraft.
	Category string `json:"category,omitempty"`
	Squawk   string `json:"squawk,omitempty"`
	// Emergency is the emergency state the aircraft broadcasts ("general", "lifeguard", ...); empty when none.
	Emergency string  `json:"emergency,omitempty"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	// AltFt is the barometric altitude, else the geometric one; nil on the ground or when unknown.
	AltFt       *int     `json:"alt_ft,omitempty"`
	OnGround    bool     `json:"on_ground,omitempty"`
	SpeedKt     *float64 `json:"speed_kt,omitempty"`
	TrackDeg    *float64 `json:"track_deg,omitempty"`
	VertRateFpm *int     `json:"vert_rate_fpm,omitempty"`
	Military    bool     `json:"military,omitempty"`
	// Private marks aircraft whose owner asked not to be identified (FAA LADD or PIA); see the privacy rule in docs/SPEC.md.
	Private bool `json:"private,omitempty"`
	// Source is the feed this report came from; SourceType is how that network heard it (adsb_icao, mlat, ...).
	Source     string `json:"source"`
	SourceType string `json:"source_type,omitempty"`
	// At is when the position was received, by the network's clock.
	At time.Time `json:"at"`
}

// Point is where the aircraft is.
func (p Position) Point() geo.Point { return geo.Point{Lat: p.Lat, Lon: p.Lon} }

// Feed is one live-position source.
type Feed interface {
	Name() string
	// Fetch returns the aircraft with a current position within radiusKm of center, one report per aircraft.
	Fetch(ctx context.Context, center geo.Point, radiusKm float64) ([]Position, error)
}

// StatusError is a feed answering with something other than 200 OK.
type StatusError struct {
	Feed string
	Code int
	// RetryAfter is the wait the feed asked for, if it sent a Retry-After header.
	RetryAfter time.Duration
	// Body is the start of the response, which some feeds use to say why they refused.
	Body string
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("%s: HTTP %d", e.Feed, e.Code)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

type source struct {
	// pointURL takes latitude, longitude and the radius in whole nautical miles.
	pointURL string
	home     string
	// spacing is the least time overhead leaves between two requests, where the poll interval is too often.
	spacing time.Duration
}

// sources are the networks overhead can read. All three serve readsb's JSON; terms checked 2026-10-29 (docs/SPEC.md: Data
// sources). airplanes.live now answers 403 until the project is approved by email.
var sources = map[string]source{
	// adsb.lol's limits are "dynamic"; one request every 5 s drew 429s within four minutes on 2026-10-29.
	"adsb.lol":       {pointURL: "https://api.adsb.lol/v2/point/%.5f/%.5f/%d", home: "https://adsb.lol", spacing: 15 * time.Second},
	"adsb.fi":        {pointURL: "https://opendata.adsb.fi/api/v3/lat/%.5f/lon/%.5f/dist/%d", home: "https://adsb.fi"},
	"airplanes.live": {pointURL: "https://api.airplanes.live/v2/point/%.5f/%.5f/%d", home: "https://airplanes.live"},
}

// Names lists the feeds ByName knows, sorted.
func Names() []string {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ByName returns the named feed. userAgent identifies overhead to the network, as their terms expect.
func ByName(name string, client *http.Client, userAgent string) (Feed, error) {
	s, ok := sources[name]
	if !ok {
		return nil, fmt.Errorf("unknown feed %q (known: %s)", name, strings.Join(Names(), ", "))
	}
	return NewReadsb(name, s.pointURL, client, userAgent), nil
}

// Spacing is the least time to leave between two requests to the named feed; 0 means the poll interval is fine.
func Spacing(name string) time.Duration { return sources[name].spacing }

// Credit is the on-screen credit a feed's terms ask for: its name and home page.
func Credit(name string) string {
	if s, ok := sources[name]; ok {
		return name + " " + s.home
	}
	return name
}
