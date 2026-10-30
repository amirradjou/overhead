package feeds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/amirradjou/overhead/internal/geo"
)

// maxBody bounds a response; a 30 km circle round a big airport is about 50 kB.
const maxBody = 8 << 20

// maxLag is how far behind our clock a network's "now" may be before its answer counts as a stale snapshot, not data.
const maxLag = 30 * time.Second

// Readsb reads a network that serves readsb's aircraft JSON (the ADSBExchange v2 format).
type Readsb struct {
	name      string
	pointURL  string
	client    *http.Client
	userAgent string
	clock     func() time.Time
}

// NewReadsb returns a feed for pointURL, a format string taking latitude, longitude and the radius in whole nautical
// miles.
func NewReadsb(name, pointURL string, client *http.Client, userAgent string) *Readsb {
	return &Readsb{name: name, pointURL: pointURL, client: client, userAgent: userAgent, clock: time.Now}
}

// Name is the network's name, e.g. adsb.lol.
func (r *Readsb) Name() string { return r.name }

// Fetch asks the network for the circle round center. The APIs take whole nautical miles, so it asks for a little
// more and trims to radiusKm itself.
func (r *Readsb) Fetch(ctx context.Context, center geo.Point, radiusKm float64) ([]Position, error) {
	nm := int(math.Ceil(radiusKm / geo.KmPerNM))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(r.pointURL, center.Lat, center.Lon, nm), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &StatusError{
			Feed:       r.name,
			Code:       resp.StatusCode,
			RetryAfter: retryAfter(resp.Header.Get("Retry-After"), r.clock()),
			Body:       refusal(snippet),
		}
	}
	var body readsbResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return nil, fmt.Errorf("%s: decode: %w", r.name, err)
	}
	positions, err := body.positions(r.name, center, radiusKm, r.clock())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.name, err)
	}
	return positions, nil
}

type readsbResponse struct {
	// Now is the network's clock in milliseconds since the epoch (adsb.fi's retired v2 sent seconds).
	Now float64 `json:"now"`
	// AC is nil when the answer has no aircraft list at all, which is a failure, not an empty sky.
	AC  []readsbAircraft `json:"ac"`
	Msg string           `json:"msg"`
}

// readsbAircraft holds the fields overhead uses; see readsb's README-json.md for the rest.
type readsbAircraft struct {
	Hex       string `json:"hex"`
	Type      string `json:"type"`
	Flight    string `json:"flight"`
	R         string `json:"r"`
	T         string `json:"t"`
	Category  string `json:"category"`
	Squawk    string `json:"squawk"`
	Emergency string `json:"emergency"`
	// Lat and Lon are absent once the position is too old; readsb moves it to lastPosition, which overhead ignores.
	Lat *float64 `json:"lat"`
	Lon *float64 `json:"lon"`
	// AltBaro is feet, or the string "ground".
	AltBaro  json.RawMessage `json:"alt_baro"`
	AltGeom  *float64        `json:"alt_geom"`
	GS       *float64        `json:"gs"`
	Track    *float64        `json:"track"`
	BaroRate *float64        `json:"baro_rate"`
	GeomRate *float64        `json:"geom_rate"`
	// SeenPos is how many seconds before Now the position was received; Seen is the same for any message.
	SeenPos *float64 `json:"seen_pos"`
	Seen    *float64 `json:"seen"`
	DBFlags int      `json:"dbFlags"`
}

// readsb's dbFlags bits.
const (
	flagMilitary = 1
	flagPIA      = 4
	flagLADD     = 8
)

// positions normalises an answer. An answer without an aircraft list or a clock, or one whose clock is well behind
// ours (a frozen cache), is an error: counting it as data would hide a dead feed behind a live-looking empty sky.
func (b readsbResponse) positions(feed string, center geo.Point, radiusKm float64, local time.Time) ([]Position, error) {
	var now time.Time
	switch {
	case b.Now > 1e11:
		now = time.UnixMilli(int64(b.Now))
	case b.Now > 0:
		now = time.UnixMilli(int64(b.Now * 1000))
	}
	switch {
	case b.AC == nil || now.IsZero():
		return nil, fmt.Errorf("answer has no aircraft list or time (msg %q)", b.Msg)
	case local.Sub(now) > maxLag:
		return nil, fmt.Errorf("answer is a snapshot from %v ago", local.Sub(now).Round(time.Second))
	}
	out := make([]Position, 0, len(b.AC))
	for _, a := range b.AC {
		hex := strings.ToLower(Printable(a.Hex))
		if hex == "" || a.Lat == nil || a.Lon == nil || surface(a.Category) {
			continue
		}
		p := Position{
			Hex:          hex,
			Callsign:     callsign(a.Flight),
			Registration: Printable(a.R),
			TypeCode:     Printable(a.T),
			Category:     Printable(a.Category),
			Squawk:       Printable(a.Squawk),
			Lat:          *a.Lat,
			Lon:          *a.Lon,
			SpeedKt:      a.GS,
			TrackDeg:     a.Track,
			Military:     a.DBFlags&flagMilitary != 0,
			Private:      a.DBFlags&(flagPIA|flagLADD) != 0,
			Source:       feed,
			SourceType:   Printable(a.Type),
		}
		if !p.Point().Valid() || geo.DistanceKm(center, p.Point()) > radiusKm {
			continue
		}
		if a.Emergency != "none" {
			p.Emergency = Printable(a.Emergency)
		}
		p.AltFt, p.OnGround = altitude(a.AltBaro, a.AltGeom)
		p.VertRateFpm = roundPtr(a.BaroRate)
		if p.VertRateFpm == nil {
			p.VertRateFpm = roundPtr(a.GeomRate)
		}
		age := a.SeenPos
		if age == nil {
			age = a.Seen
		}
		p.At = now
		if age != nil {
			p.At = now.Add(-time.Duration(*age * float64(time.Second)))
		}
		out = append(out, p)
	}
	return out, nil
}

// validCallsign is the ADS-B identification alphabet: up to eight letters and digits (spaces pad it).
var validCallsign = regexp.MustCompile(`^[A-Z0-9]{1,8}$`)

// callsign cleans a broadcast identification. readsb prints "@" for characters it could not decode, and a callsign
// with any of them is no callsign: looked up, it only earns an error.
func callsign(flight string) string {
	c := strings.TrimSpace(flight)
	if !validCallsign.MatchString(c) {
		return ""
	}
	return c
}

// surface reports whether an ADS-B emitter category is a ground vehicle or obstacle (C1 to C7) rather than an
// aircraft. Airports fit their tugs and follow-me cars with transmitters: a dozen of them sit within a kilometre of
// Pearson's centre at any time.
func surface(category string) bool {
	return len(category) == 2 && category[0] == 'C' && category[1] >= '1' && category[1] <= '7'
}

func altitude(baro json.RawMessage, geom *float64) (*int, bool) {
	if bytes.Equal(bytes.TrimSpace(baro), []byte(`"ground"`)) {
		return nil, true
	}
	var ft *float64 // stays nil for an absent or null alt_baro
	if json.Unmarshal(baro, &ft) == nil && ft != nil {
		return roundPtr(ft), false
	}
	return roundPtr(geom), false
}

func roundPtr(f *float64) *int {
	if f == nil {
		return nil
	}
	n := int(math.Round(*f))
	return &n
}

// retryAfter reads a Retry-After header, either seconds or an HTTP date.
func retryAfter(h string, now time.Time) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// refusal pulls the reason out of an error response: the "error" or "msg" field of a JSON body, else the text.
func refusal(body []byte) string {
	var j struct {
		Error string `json:"error"`
		Msg   string `json:"msg"`
	}
	text := Printable(string(body))
	if json.Unmarshal(body, &j) == nil {
		text = strings.TrimSpace(j.Error + " " + j.Msg)
	}
	if strings.HasPrefix(text, "<") {
		return "" // an HTML error page says nothing useful in a terminal
	}
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return Printable(text)
}

// Printable drops control and other non-printing characters and trims spaces. Feed text ends up on a terminal, where
// an escape sequence in a callsign or an error page could move the cursor, recolour or retitle the screen.
func Printable(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s))
}
