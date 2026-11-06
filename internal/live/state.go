// Package live keeps the current picture of the sky from the live queue and serves it to the screen: a JSON snapshot
// and a Server-Sent Events stream.
package live

import (
	"math"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

// DefaultMaxAge drops an aircraft from the picture when its newest position is older than this.
const DefaultMaxAge = 60 * time.Second

// airline matches an airline flight's callsign: the operator's three-letter ICAO code and a flight number.
var airline = regexp.MustCompile(`^[A-Z]{3}[0-9][0-9A-Z]{0,3}$`)

// Commercial reports whether p looks like an airline flight, the only kind the public screen shows (privacy
// rule, docs/SPEC.md): an airline callsign, and neither military nor flagged private.
func Commercial(p feeds.Position) bool {
	return airline.MatchString(p.Callsign) && !p.Military && !p.Private
}

// State is the live picture. It is safe for concurrent use.
type State struct {
	Center   geo.Point
	RadiusKm float64
	MaxAge   time.Duration
	// Public keeps only commercial flights in views (privacy rule, docs/SPEC.md), without routes or photos unless PublicRoutes
	// or PublicPhotos says their sources allow it (docs/setup.md, section 7), and only the planes a focused screen shows.
	Public, PublicRoutes, PublicPhotos bool
	// Window is the view the screen draws; the zero value is DefaultWindow.
	Window Window
	// Airport, when set, is the airport the screen follows; NearKm, when set, how close to the window (in a straight
	// line) a plane counts as very close. With either, the screen shows those planes only (View.Focused).
	Airport *Airport
	NearKm  float64

	mu       sync.Mutex
	pos      map[string]feeds.Position
	info     map[string]events.Aircraft
	day      string
	today    map[string]struct{}
	flights  map[string]*flight
	sources  map[string]struct{}
	received uint64
	pruned   time.Time

	weather              *Weather
	metar                *Metar
	runway               string // the runway the latest take-off or landing at Airport used
	runwayAt             time.Time
	arrivals, departures map[string]struct{} // flights that landed at or took off from Airport today

	facts                               []string
	predicted                           map[string]prediction
	predErrors                          []float64
	predChecked, predWithin, predMissed int
}

// flight is one commercial flight's pass through the circle, for the coverage tally.
type flight struct {
	first, last       time.Time
	hasType, hasRoute bool
	hex, callsign     string
}

func (s *State) init() {
	if s.pos == nil {
		s.pos, s.info = make(map[string]feeds.Position), make(map[string]events.Aircraft)
		s.today, s.flights = make(map[string]struct{}), make(map[string]*flight)
		s.arrivals, s.departures = make(map[string]struct{}), make(map[string]struct{})
		s.sources = make(map[string]struct{})
	}
}

// Update takes a position event. Older positions than the one held are ignored (the live queue may redeliver).
func (s *State) Update(e events.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	s.received++
	if e.Source != "" {
		s.sources[e.Source] = struct{}{}
	}
	cur, had := s.pos[e.Hex]
	if had && !e.At.After(cur.At) {
		return
	}
	s.pos[e.Hex] = e.Position
	// A new day only moves forward, and a late report from before midnight counts toward its own day, not today.
	day := e.At.Local().Format(time.DateOnly)
	if day > s.day {
		s.day, s.today = day, make(map[string]struct{})
		s.arrivals, s.departures = make(map[string]struct{}), make(map[string]struct{})
	}
	if day == s.day {
		s.today[e.Hex] = struct{}{}
		if had {
			s.countMovement(cur, e.Position)
		}
	}
	s.track(e.Position)
}

// SetAircraft takes an enrichment result.
func (s *State) SetAircraft(a events.Aircraft) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	s.info[a.Hex] = a
	if p, ok := s.pos[a.Hex]; ok {
		s.track(p)
	}
}

// HasInfo reports whether enrichment has answered for hex.
func (s *State) HasInfo(hex string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.info[hex]
	return ok
}

func (s *State) track(p feeds.Position) {
	if !Commercial(p) || p.OnGround {
		return
	}
	key := p.Hex + "/" + p.Callsign
	f := s.flights[key]
	if f == nil {
		f = &flight{first: p.At, hex: p.Hex, callsign: p.Callsign}
		s.flights[key] = f
	}
	if p.At.After(f.last) {
		f.last = p.At
	}
	info, ok := s.info[p.Hex]
	if p.TypeCode != "" || (ok && info.TypeCode != "") {
		f.hasType = true
	}
	if ok && info.Route != nil && info.Route.Callsign == p.Callsign && plausible(p, info.Route) {
		f.hasRoute = true
	}
}

// Plane is one aircraft as the screen gets it.
type Plane struct {
	feeds.Position
	DistanceKm float64          `json:"distance_km"`
	BearingDeg float64          `json:"bearing_deg"`
	Info       *events.Aircraft `json:"info,omitempty"`
	// ToDestinationKm and ETAMin are set when the route's destination is known and within reach.
	ToDestinationKm *float64 `json:"to_destination_km,omitempty"`
	ETAMin          *float64 `json:"eta_min,omitempty"`
	// Notable says why the screen's banner should show this aircraft; empty for most.
	Notable string `json:"notable,omitempty"`
	// ElevationDeg and OffsetDeg place the plane as seen from the window: degrees above the horizon, and degrees
	// right (+) or left (-) of the window's centre. InView says the window shows it now; Coming, when it will.
	ElevationDeg float64 `json:"elevation_deg"`
	OffsetDeg    float64 `json:"offset_deg"`
	InView       bool    `json:"in_view"`
	Coming       *Coming `json:"coming,omitempty"`
	Motion       *Motion `json:"motion,omitempty"`
	// Airport says what the plane is doing at the followed airport; Near, that it is very close to the window.
	// Focus marks the planes a focused screen shows: those two, less the ones standing still on the ground.
	Airport *AtAirport `json:"airport,omitempty"`
	Near    bool       `json:"near,omitempty"`
	Focus   bool       `json:"focus,omitempty"`
	// AltMSLFt is the true altitude, the pressure altitude corrected for today's pressure, below 18,000 ft when the
	// pressure is known; alt_ft stays as the aircraft reported it.
	AltMSLFt *int `json:"alt_msl_ft,omitempty"`
}

// SpecialTypes are airframes rare almost anywhere, by ICAO type designator. Rarity at home (what is unusual over this
// street) comes from the history in M4; this list covers the obvious cases until then.
var SpecialTypes = map[string]string{
	"A124": "Antonov An-124 Ruslan",
	"A225": "Antonov An-225 Mriya",
	"AN22": "Antonov An-22",
	"A3ST": "Airbus Beluga",
	"A337": "Airbus BelugaXL",
	"BLCF": "Boeing 747 Dreamlifter",
	"SGUP": "Super Guppy",
	"B52":  "Boeing B-52 Stratofortress",
	"C5M":  "Lockheed C-5M Super Galaxy",
	"A400": "Airbus A400M Atlas",
	"CONC": "Concorde",
}

// notable says why an aircraft deserves the banner, most urgent first, or "" if it doesn't.
func notable(p feeds.Position) string {
	switch p.Squawk {
	case "7500":
		return "Squawking 7500: unlawful interference"
	case "7600":
		return "Squawking 7600: radio failure"
	case "7700":
		return "Squawking 7700: emergency"
	}
	if p.Emergency != "" {
		return "Declared emergency: " + p.Emergency
	}
	if name, ok := SpecialTypes[p.TypeCode]; ok {
		return "Rare visitor: " + name
	}
	if p.Military {
		return "Military aircraft"
	}
	return ""
}

// View is the picture at one moment.
type View struct {
	At       time.Time `json:"at"`
	Center   LatLon    `json:"center"`
	RadiusKm float64   `json:"radius_km"`
	Window   Window    `json:"window"`
	Planes   []Plane   `json:"planes"` // nearest first
	// Today counts distinct aircraft seen since local midnight.
	Today int `json:"today"`
	// Sources are the feeds that have reported since the start, to credit on screen: every one of them, whichever
	// happened to have the freshest report of the planes shown at this moment.
	Sources []string `json:"sources"`
	// Facts are sentences about the history (overhead-stats) for the screen to show between planes.
	Facts []string `json:"facts"`
	// Focused says the screen shows only the planes marked Focus: those at the followed airport (Airport) and those
	// very close to the window (within NearKm).
	Focused bool         `json:"focused,omitempty"`
	NearKm  float64      `json:"near_km,omitempty"`
	Airport *AirportView `json:"airport,omitempty"`
	// Weather is the weather at home, when a reading younger than weatherStale is held.
	Weather *Weather `json:"weather,omitempty"`
	// Public says this is the public instance: commercial flights only, seen by people who don't know the window.
	Public bool `json:"public,omitempty"`
}

// LatLon is a point in the JSON the screen reads.
type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// etaReach is how far away a destination may be for a time to landing to make sense on the screen.
const etaReach = 150.0

// View returns the picture at now, dropping aircraft whose position is older than MaxAge.
func (s *State) View(now time.Time) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	maxAge := s.MaxAge
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	s.prune(now)
	w := s.Window.normalised()
	v := View{
		At: now, Center: LatLon{s.Center.Lat, s.Center.Lon}, RadiusKm: s.RadiusKm, Window: w,
		Planes: []Plane{}, Sources: []string{}, Facts: append([]string{}, s.facts...),
		Focused: s.Airport != nil || s.NearKm > 0, NearKm: s.NearKm, Public: s.Public,
	}
	today := now.Local().Format(time.DateOnly) == s.day
	v.Weather = s.weatherNow(now)
	lift := pressureFeet(v.Weather)
	for hex, p := range s.pos {
		if now.Sub(p.At) > maxAge {
			delete(s.pos, hex)
			continue
		}
		if s.Public && !Commercial(p) {
			continue
		}
		pl := Plane{
			Position:   p,
			DistanceKm: geo.DistanceKm(s.Center, p.Point()),
			BearingDeg: geo.BearingDeg(s.Center, p.Point()),
			Notable:    notable(p),
		}
		// Heights are true heights: the pressure altitude corrected for today's pressure, below the transition.
		g := p
		if lift != 0 && p.AltFt != nil && *p.AltFt < transitionFt {
			a := *p.AltFt + int(math.Round(lift))
			g.AltFt, pl.AltMSLFt = &a, &a
		}
		var route *events.Route
		if info, ok := s.info[hex]; ok {
			if info.Route != nil && info.Route.Callsign != p.Callsign {
				info.Route = nil // a route looked up for an earlier flight of this airframe
			}
			if info.Route != nil && !plausible(g, info.Route) {
				info.Route = nil // an old leg of a reused flight number
			}
			// A route still decides, unseen, whether a plane belongs to the followed airport.
			route = info.Route
			if s.Public && !s.PublicRoutes {
				info.Route = nil
			}
			if s.Public && !s.PublicPhotos {
				info.Photo = nil
			}
			pl.Info = &info
			pl.ToDestinationKm, pl.ETAMin = eta(p, info.Route)
		}
		if s.Airport != nil {
			if at := s.Airport.classify(g, route); at != nil {
				pl.Airport = at
				s.noteAirport(p, at)
			}
		}
		pl.Near = near(g, s.Center, w.HomeElevationM, s.NearKm)
		pl.Focus = pl.Near || (pl.Airport != nil && pl.Airport.Phase != "gate" && pl.Airport.Phase != "arrived")
		// Placed where it should be now, not where it was last reported: the report may be several seconds old.
		age := reportAge(p, now)
		if pt, h, ok := project(g, age); ok {
			sg := w.sight(s.Center, pt, h)
			pl.ElevationDeg, pl.OffsetDeg, pl.InView = sg.elevation, sg.offset, w.sees(sg)
			pl.Motion = w.motion(s.Center, g, age, sg)
			if !pl.InView {
				pl.Coming = w.coming(s.Center, g, now)
			}
		} else {
			pl.OffsetDeg = geo.AngleDiff(pl.BearingDeg, w.AzimuthDeg)
		}
		v.Planes = append(v.Planes, pl)
	}
	slices.SortFunc(v.Planes, func(a, b Plane) int {
		switch {
		case a.DistanceKm < b.DistanceKm:
			return -1
		case a.DistanceKm > b.DistanceKm:
			return 1
		}
		return 0
	})
	for src := range s.sources {
		v.Sources = append(v.Sources, src)
	}
	slices.Sort(v.Sources)
	if today {
		v.Today = len(s.today)
	}

	if a := s.Airport; a != nil {
		v.Airport = &AirportView{ICAO: a.ICAO, IATA: a.IATA, Name: a.Name, Lat: a.Lat, Lon: a.Lon, ElevationFt: a.ElevationFt}
		if s.runway != "" && now.Sub(s.runwayAt) <= runwayFresh {
			v.Airport.RunwayInUse = s.runway
		}
		switch {
		case s.Public:
			v.Airport.Uncounted = true
		case today:
			v.Airport.ArrivalsToday, v.Airport.DeparturesToday = len(s.arrivals), len(s.departures)
		}
	}
	s.trackPredictions(v)
	if s.Public && v.Focused {
		// Most of a snapshot is planes a focused screen never draws: the board's and the sky's are all it needs.
		v.Planes = slices.DeleteFunc(v.Planes, func(p Plane) bool { return !p.Focus && p.Airport == nil })
	}
	return v
}

// noteAirport keeps the runway in use and the day's arrivals and departures. Caller holds s.mu.
func (s *State) noteAirport(p feeds.Position, at *AtAirport) {
	if at.Runway != "" && !p.At.Before(s.runwayAt) {
		s.runway, s.runwayAt = at.Runway, p.At
	}
}

// countMovement counts a touchdown or a lift-off at the followed airport: a flight whose reports change between
// airborne and on the ground close to it. Each flight is counted once a day each way. Caller holds s.mu.
func (s *State) countMovement(prev, p feeds.Position) {
	if s.Airport == nil || prev.OnGround == p.OnGround {
		return
	}
	ap := s.Airport.point()
	if geo.DistanceKm(ap, p.Point()) > groundKm || geo.DistanceKm(ap, prev.Point()) > 2*groundKm {
		return
	}
	key := p.Hex + "/" + p.Callsign
	if p.OnGround {
		s.arrivals[key] = struct{}{}
	} else {
		s.departures[key] = struct{}{}
	}
}

// How long the state remembers an aircraft's enrichment after it leaves, and a flight for the coverage tally.
const (
	keepInfo    = 6 * time.Hour
	keepFlights = 7 * 24 * time.Hour
	pruneEvery  = time.Minute
)

// prune forgets enrichment for aircraft long gone and flights older than the coverage window, at most once a
// minute, so a process left running for months holds what it needs, not everything it ever saw. Caller holds s.mu.
func (s *State) prune(now time.Time) {
	if now.Sub(s.pruned) < pruneEvery {
		return
	}
	s.pruned = now
	for hex, a := range s.info {
		p, here := s.pos[hex]
		gone := !here || now.Sub(p.At) > keepInfo // a position this old is about to be dropped anyway
		if gone && now.Sub(a.Looked) > keepInfo {
			delete(s.info, hex)
		}
	}
	for key, f := range s.flights {
		if now.Sub(f.last) > keepFlights {
			delete(s.flights, key)
		}
	}
}

// A route is implausible when the plane is far off any reasonable way between its ends. adsbdb's routes are
// crowd-sourced, and a flight number flown on a new leg can keep its old one: on 2026-10-30, UAL397 "Newark to
// Orlando" and RPA5603 "Boston to LaGuardia" were both over Lake Ontario. Airways, weather and holding stay well
// inside the slack.
const (
	routeSlack   = 1.25  // origin → plane → destination may be this much longer than origin → destination
	routeSlackKm = 150.0 // and this much more, for short hops and planes still near either end
)

// The corridor is not enough: a route must also fit what the plane is doing. Checked against independent sources on
// 2026-10-30, 6 of 43 routes passed the corridor and were still wrong (KLM147 "Pearson to Amsterdam" landing at
// Pearson, JZA482 "departing Pearson" cruising over it, AAL1876 parked at Pearson as "Dallas to Washington"). So: on
// the ground it must be at one end; low it must be near an end, descending towards its destination or climbing
// away from its origin; and it cannot be cruising close to either end.
const (
	routeGroundKm  = 5.0
	routeLowFt     = 10000
	routeLowKm     = 100.0
	routeCruiseFt  = 25000
	routeCruiseKm  = 60.0
	routeVerticalF = 500 // ft/min: climbing or descending, not wobbling level
)

func plausible(p feeds.Position, r *events.Route) bool {
	o := geo.Point{Lat: r.Origin.Lat, Lon: r.Origin.Lon}
	d := geo.Point{Lat: r.Destination.Lat, Lon: r.Destination.Lon}
	if o == (geo.Point{}) || d == (geo.Point{}) || !o.Valid() || !d.Valid() {
		return true // nothing to check it against
	}
	here := p.Point()
	toO, toD := geo.DistanceKm(o, here), geo.DistanceKm(here, d)
	if toO+toD > routeSlack*geo.DistanceKm(o, d)+routeSlackKm {
		return false
	}
	near := math.Min(toO, toD)
	if p.OnGround {
		return near <= routeGroundKm
	}
	if p.AltFt == nil {
		return true
	}
	switch alt := *p.AltFt; {
	case alt > routeCruiseFt:
		return near >= routeCruiseKm
	case alt < routeLowFt:
		if near > routeLowKm {
			return false
		}
		if vr := p.VertRateFpm; vr != nil {
			if *vr <= -routeVerticalF && toO < toD {
				return false // coming down at its origin
			}
			if *vr >= routeVerticalF && toD < toO {
				return false // climbing out of its destination
			}
		}
	}
	return true
}

// etaClosingDeg: a time to landing is given only to a plane heading for its destination within this many degrees.
// One being vectored or flying a downwind leg lands far later than distance over speed says (ten minutes later, in
// the check of 2026-10-30).
const etaClosingDeg = 30.0

func eta(p feeds.Position, r *events.Route) (*float64, *float64) {
	if r == nil || (r.Destination.Lat == 0 && r.Destination.Lon == 0) {
		return nil, nil
	}
	km := geo.DistanceKm(p.Point(), geo.Point{Lat: r.Destination.Lat, Lon: r.Destination.Lon})
	if km > etaReach {
		return nil, nil
	}
	if p.OnGround || p.SpeedKt == nil || *p.SpeedKt < 60 {
		return &km, nil
	}
	dest := geo.Point{Lat: r.Destination.Lat, Lon: r.Destination.Lon}
	if p.TrackDeg != nil && km > 3 && math.Abs(geo.AngleDiff(geo.BearingDeg(p.Point(), dest), *p.TrackDeg)) > etaClosingDeg {
		return &km, nil
	}
	minutes := km / (*p.SpeedKt * geo.KmPerNM) * 60
	return &km, &minutes
}

// Coverage is M1's measure: of the airline flights that spent at least MinInView airborne in the circle, how many
// were shown with their type and their route.
type Coverage struct {
	Flights   int     `json:"flights"`
	WithType  int     `json:"with_type"`
	WithRoute int     `json:"with_route"`
	WithBoth  int     `json:"with_both"`
	Share     float64 `json:"share_with_both"`
	// Missing lists flights without a route (callsign, type), newest first, so gaps can be checked by hand.
	Missing []string `json:"missing,omitempty"`
}

// MinInView is how long a flight must be in view to count toward coverage: shorter passes can end before enrichment
// answers and say more about timing than about coverage.
const MinInView = time.Minute

// Coverage returns the tally since the process started.
func (s *State) Coverage() Coverage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c Coverage
	var missing []*flight
	for _, f := range s.flights {
		if f.last.Sub(f.first) < MinInView {
			continue
		}
		c.Flights++
		c.WithType += btoi(f.hasType)
		c.WithRoute += btoi(f.hasRoute)
		c.WithBoth += btoi(f.hasType && f.hasRoute)
		if !f.hasRoute {
			missing = append(missing, f)
		}
	}
	if c.Flights > 0 {
		c.Share = float64(c.WithBoth) / float64(c.Flights)
	}
	slices.SortFunc(missing, func(a, b *flight) int { return b.last.Compare(a.last) })
	for _, f := range missing[:min(len(missing), 20)] {
		c.Missing = append(c.Missing, f.callsign+" ("+f.hex+")")
	}
	return c
}

// Received counts the position events taken so far.
func (s *State) Received() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
