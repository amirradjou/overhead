// Package enrich finds out what an aircraft is and where it is going: airframe and route from adsbdb, a photo of the
// exact airframe from planespotters.net. Answers are cached (airframes for a month, routes for a day) so a new plane
// costs a few API calls, not one per poll.
package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
)

// HTTPError is an API answering with an unexpected status.
type HTTPError struct {
	API  string
	Code int
}

func (e *HTTPError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.API, e.Code) }

// Retryable reports whether a failed lookup may succeed later: network trouble, rate limits and server errors may;
// a malformed request or answer won't.
func Retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			if Retryable(e) {
				return true
			}
		}
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Code == http.StatusTooManyRequests || he.Code >= 500
	}
	var de *decodeError
	return !errors.As(err, &de)
}

type decodeError struct{ err error }

func (e *decodeError) Error() string { return "decode: " + e.err.Error() }

// pacer spaces calls to one API: free services, so overhead asks at most once a second.
type pacer struct {
	mu    sync.Mutex
	every time.Duration
	next  time.Time
}

func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if d := time.Until(p.next); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	p.next = time.Now().Add(p.every)
	return nil
}

type client struct {
	api       string
	base      string
	http      *http.Client
	userAgent string
	pace      pacer
}

// get fetches path into dst. It reports false, with no error, when the API answers 404.
func (c *client) get(ctx context.Context, path string, dst any) (bool, error) {
	if err := c.pace.wait(ctx); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("%s: %w", c.api, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return false, nil
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return false, &HTTPError{API: c.api, Code: resp.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst); err != nil {
		return false, fmt.Errorf("%s: %w", c.api, &decodeError{err})
	}
	return true, nil
}

// ADSBDB reads api.adsbdb.com: airframes by ICAO address and routes by callsign.
type ADSBDB struct{ c client }

// NewADSBDB returns a client for base (https://api.adsbdb.com in production).
func NewADSBDB(base string, hc *http.Client, userAgent string) *ADSBDB {
	return &ADSBDB{c: client{api: "adsbdb", base: base, http: hc, userAgent: userAgent, pace: pacer{every: time.Second}}}
}

type adsbdbAnswer struct {
	Response struct {
		Aircraft    *adsbdbAircraft `json:"aircraft"`
		Flightroute *adsbdbRoute    `json:"flightroute"`
	} `json:"response"`
}

type adsbdbAircraft struct {
	Type         string `json:"type"`
	ICAOType     string `json:"icao_type"`
	Manufacturer string `json:"manufacturer"`
	Registration string `json:"registration"`
	Owner        string `json:"registered_owner"`
	OwnerCountry string `json:"registered_owner_country_name"`
}

type adsbdbRoute struct {
	Callsign     string `json:"callsign"`
	CallsignIATA string `json:"callsign_iata"`
	Airline      *struct {
		Name string `json:"name"`
		ICAO string `json:"icao"`
	} `json:"airline"`
	Origin      adsbdbAirport `json:"origin"`
	Destination adsbdbAirport `json:"destination"`
}

type adsbdbAirport struct {
	ICAO    string  `json:"icao_code"`
	IATA    string  `json:"iata_code"`
	Name    string  `json:"name"`
	City    string  `json:"municipality"`
	Country string  `json:"country_name"`
	Lat     float64 `json:"latitude"`
	Lon     float64 `json:"longitude"`
}

// Aircraft asks for an airframe and, if callsign is set, its route, in one call. It returns nil for whichever adsbdb
// doesn't know. routeSettled is false when the route was not looked at: adsbdb answers 404 for an unknown airframe
// without checking the callsign.
func (a *ADSBDB) Aircraft(ctx context.Context, hex, callsign string) (ac *events.Aircraft, route *events.Route, routeSettled bool, err error) {
	path := "/v0/aircraft/" + url.PathEscape(strings.ToUpper(hex))
	if callsign != "" {
		path += "?callsign=" + url.QueryEscape(callsign)
	}
	var ans adsbdbAnswer
	found, err := a.c.get(ctx, path, &ans)
	if err != nil || !found {
		return nil, nil, false, err
	}
	if r := ans.Response.Aircraft; r != nil {
		ac = &events.Aircraft{
			Hex:          hex,
			Registration: feeds.Printable(r.Registration),
			TypeCode:     feeds.Printable(r.ICAOType),
			TypeName:     feeds.Printable(r.Type),
			Manufacturer: feeds.Printable(r.Manufacturer),
			Owner:        feeds.Printable(r.Owner),
			OwnerCountry: feeds.Printable(r.OwnerCountry),
		}
	}
	return ac, ans.Response.Flightroute.toRoute(), callsign != "", nil
}

// Route asks for the route flown under callsign; nil when adsbdb doesn't know it.
func (a *ADSBDB) Route(ctx context.Context, callsign string) (*events.Route, error) {
	var ans adsbdbAnswer
	if _, err := a.c.get(ctx, "/v0/callsign/"+url.PathEscape(callsign), &ans); err != nil {
		return nil, err
	}
	return ans.Response.Flightroute.toRoute(), nil
}

func (r *adsbdbRoute) toRoute() *events.Route {
	if r == nil {
		return nil
	}
	out := &events.Route{
		Callsign:    feeds.Printable(r.Callsign),
		FlightIATA:  feeds.Printable(r.CallsignIATA),
		Origin:      r.Origin.toAirport(),
		Destination: r.Destination.toAirport(),
	}
	if r.Airline != nil {
		out.Airline, out.AirlineICAO = feeds.Printable(r.Airline.Name), feeds.Printable(r.Airline.ICAO)
	}
	return out
}

func (a adsbdbAirport) toAirport() events.Airport {
	return events.Airport{
		ICAO: feeds.Printable(a.ICAO), IATA: feeds.Printable(a.IATA), Name: feeds.Printable(a.Name),
		City: feeds.Printable(a.City), Country: feeds.Printable(a.Country), Lat: a.Lat, Lon: a.Lon,
	}
}

// Planespotters reads planespotters.net's public photo API.
type Planespotters struct{ c client }

// NewPlanespotters returns a client for base (https://api.planespotters.net in production).
func NewPlanespotters(base string, hc *http.Client, userAgent string) *Planespotters {
	return &Planespotters{c: client{api: "planespotters", base: base, http: hc, userAgent: userAgent, pace: pacer{every: time.Second}}}
}

// Photo returns the newest photo of the airframe, or nil if there is none.
func (p *Planespotters) Photo(ctx context.Context, hex string) (*events.Photo, error) {
	var ans struct {
		Photos []struct {
			Thumbnail struct {
				Src string `json:"src"`
			} `json:"thumbnail"`
			Large struct {
				Src string `json:"src"`
			} `json:"thumbnail_large"`
			Link         string `json:"link"`
			Photographer string `json:"photographer"`
		} `json:"photos"`
	}
	if _, err := p.c.get(ctx, "/pub/photos/hex/"+url.PathEscape(strings.ToUpper(hex)), &ans); err != nil {
		return nil, err
	}
	for _, ph := range ans.Photos {
		// Without the link and the photographer the photo can't be credited, so it isn't used.
		if ph.Thumbnail.Src == "" || ph.Link == "" || ph.Photographer == "" {
			continue
		}
		return &events.Photo{
			Thumbnail: ph.Thumbnail.Src, Large: ph.Large.Src,
			Link: ph.Link, Photographer: feeds.Printable(ph.Photographer),
		}, nil
	}
	return nil, nil
}
