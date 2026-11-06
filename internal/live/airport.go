package live

import (
	"math"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

// Airport is an airport the screen can follow: planes landing at it, taking off from it or moving on its ground get
// a fuller caption and a place on its board, and with a followed airport the screen shows those planes and the ones
// very close to home only.
type Airport struct {
	ICAO, IATA, Name string
	Lat, Lon         float64
	ElevationFt      float64
	// Runways are the runway ends by the true heading flown along them: runway 26 is flown towards 251°.
	Runways map[string]float64
}

// Airports are the airports overhead can follow, by ICAO code. Billy Bishop's runway headings come from the tracks
// of its take-offs and landings in the history (runway 26 on 2026-10-30: 250–255°) and agree with OurAirports'
// thresholds (71.2°/251.2°); the short 06/24 is open too.
var Airports = map[string]Airport{
	"CYTZ": {
		ICAO: "CYTZ", IATA: "YTZ", Name: "Billy Bishop", Lat: 43.6275, Lon: -79.3962, ElevationFt: 252,
		Runways: map[string]float64{"08": 71, "26": 251, "06": 50, "24": 230},
	},
}

// AtAirport is what a plane is doing at the followed airport.
type AtAirport struct {
	// Op is "arriving" or "departing", by the plane's route or, without one, by how it moves; "ground" is a plane
	// moving on the airport's ground with no route to tell which.
	Op string `json:"op"`
	// Phase is, arriving: "approach", "final", "landing" (rolling out), "taxiing", "arrived"; departing: "gate",
	// "taxiing", "takeoff", "climbing"; on the ground with no route: "taxiing" or "runway".
	Phase string `json:"phase"`
	// Runway is the runway in use, once the plane is lined up with one.
	Runway     string  `json:"runway,omitempty"`
	DistanceKm float64 `json:"distance_km"`
	// HeightFt is an airborne plane's height above the airport.
	HeightFt *float64 `json:"height_ft,omitempty"`
	// InMin is the time to touchdown of an arriving plane, at its speed now; AgoMin, how long ago a departing one
	// took off, from how far it has come at its speed now.
	InMin  *float64 `json:"in_min,omitempty"`
	AgoMin *float64 `json:"ago_min,omitempty"`
}

// AirportView is the followed airport on the screen.
type AirportView struct {
	ICAO        string  `json:"icao"`
	IATA        string  `json:"iata"`
	Name        string  `json:"name"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	ElevationFt float64 `json:"elevation_ft"`
	// RunwayInUse is the runway the latest take-off or landing used, within the last runwayFresh.
	RunwayInUse string `json:"runway_in_use,omitempty"`
	// ArrivalsToday and DeparturesToday count the flights seen touching down and lifting off since local midnight,
	// rebuilt from the stored history when the live API starts.
	ArrivalsToday   int `json:"arrivals_today"`
	DeparturesToday int `json:"departures_today"`
	// Uncounted says the counts mean nothing: the public instance has no history to rebuild them from.
	Uncounted bool `json:"uncounted,omitempty"`
}

const (
	finalKm       = 10.0   // an arriving plane closer than this, and lower than finalFt, is on final
	finalFt       = 3000.0 //
	takeoffKm     = 5.0    // a departing plane closer than this, and lower than takeoffFt, is still taking off
	takeoffFt     = 1500.0 //
	rollKt        = 40.0   // faster than this on the ground is a take-off or landing roll
	stillKt       = 3.0    // slower than this on the ground is standing still
	groundKm      = 3.0    // on the ground this close to the airport is on its ground
	unroutedKm    = 8.0    // without a route: a plane this close, below unroutedFt, climbing or descending at least
	unroutedFt    = 2500.0 // unroutedFPM, is taking off or landing
	unroutedFPM   = 300
	departedKm    = 40.0 // a departure is followed this far out
	runwayAlign   = 15.0 // a track within this of a runway's heading is lined up with it (08 and 06 are 21° apart)
	runwaySide    = 25.0 // on final, the plane lies within this of the runway's extended centre line
	closingDeg    = 35.0 // a plane whose track is within this of the airport's bearing is heading for it
	runwayFresh   = 45 * time.Minute
	flyingSpeedKt = 60.0 // slower than this, a time from distance and speed means nothing
)

func (a *Airport) point() geo.Point { return geo.Point{Lat: a.Lat, Lon: a.Lon} }

// is reports whether ap, one end of a route, is this airport.
func (a *Airport) is(ap events.Airport) bool {
	return (ap.ICAO != "" && ap.ICAO == a.ICAO) || (ap.IATA != "" && ap.IATA == a.IATA)
}

// classify says what p is doing at the airport, or nil when it has nothing to do with it. r is the plane's route,
// already checked (nil when unknown).
func (a *Airport) classify(p feeds.Position, r *events.Route) *AtAirport {
	d := geo.DistanceKm(a.point(), p.Point())
	var height *float64
	if p.AltFt != nil && !p.OnGround {
		h := math.Max(0, float64(*p.AltFt)-a.ElevationFt)
		height = &h
	}
	speed := 0.0
	if p.SpeedKt != nil {
		speed = *p.SpeedKt
	}
	at := &AtAirport{DistanceKm: d, HeightFt: height}
	switch {
	case r != nil && a.is(r.Destination):
		at.Op = "arriving"
	case r != nil && a.is(r.Origin):
		at.Op = "departing"
	case r != nil && r.Origin.ICAO != "" && r.Destination.ICAO != "":
		return nil // flying between two other airports
	case p.OnGround:
		at.Op = "ground"
	case d <= unroutedKm && height != nil && *height <= unroutedFt && p.VertRateFpm != nil:
		switch {
		// Without a route, a circuit round the field looks like both: it is a landing only when it comes down
		// heading for the field, a take-off only low and close, heading away (a Cessna circling at 1,700 ft was
		// called both, on 2026-10-30).
		case *p.VertRateFpm <= -unroutedFPM && a.closing(p):
			at.Op = "arriving"
		case *p.VertRateFpm >= unroutedFPM && !a.closing(p) && d <= takeoffKm && *height <= takeoffFt:
			at.Op = "departing"
		default:
			return nil
		}
	default:
		return nil
	}
	if p.OnGround {
		if d > groundKm {
			return nil // on the ground somewhere else
		}
		switch {
		case speed >= rollKt:
			at.Phase = map[string]string{"arriving": "landing", "departing": "takeoff", "ground": "runway"}[at.Op]
		case speed >= stillKt:
			at.Phase = "taxiing"
		case at.Op == "arriving":
			at.Phase = "arrived"
		case at.Op == "departing":
			at.Phase = "gate"
		default:
			return nil // parked, with no flight to tell of
		}
	} else {
		switch at.Op {
		case "arriving":
			at.Phase = "approach"
			closing := a.closing(p)
			if d <= finalKm && height != nil && *height <= finalFt && closing {
				at.Phase = "final"
			}
			// A time to touchdown only while it heads for the field: vectored or on a downwind leg, it lands far
			// later than distance over speed says.
			if speed >= flyingSpeedKt && (closing || d < 2) {
				m := d / (speed * geo.KmPerNM) * 60
				at.InMin = &m
			}
		case "departing":
			if d > departedKm {
				return nil
			}
			at.Phase = "climbing"
			if d <= takeoffKm && height != nil && *height <= takeoffFt {
				at.Phase = "takeoff"
			}
			if speed >= flyingSpeedKt {
				m := d / (speed * geo.KmPerNM) * 60
				at.AgoMin = &m
			}
		}
	}
	if p.TrackDeg != nil && p.Category != "A7" { // a helicopter (A7) lifts off anywhere, along no runway
		from := geo.BearingDeg(a.point(), p.Point())
		switch {
		case at.Phase == "landing" || at.Phase == "runway" || (at.Phase == "takeoff" && p.OnGround):
			at.Runway = a.runway(*p.TrackDeg, nil, 0) // on it: its track says which
		case at.Phase == "final":
			at.Runway = a.runway(*p.TrackDeg, &from, 180) // off the runway's start, coming in along it
		case at.Phase == "takeoff":
			at.Runway = a.runway(*p.TrackDeg, &from, 0) // just off, beyond its far end
		}
	}
	return at
}

// closing reports whether p is heading for the airport.
func (a *Airport) closing(p feeds.Position) bool {
	if p.TrackDeg == nil {
		return false
	}
	return math.Abs(geo.AngleDiff(geo.BearingDeg(p.Point(), a.point()), *p.TrackDeg)) <= closingDeg
}

// runway is the runway a plane on track is lined up with, or "". With from (the bearing from the airport to the
// plane), the plane must also lie along the runway's centre line, turn degrees from its heading (180: before its
// start, on final; 0: beyond its end, just off it), so a plane flying parallel a few kilometres off is not on it.
func (a *Airport) runway(track float64, from *float64, turn float64) string {
	best, bestOff := "", runwayAlign
	for name, heading := range a.Runways {
		off := math.Abs(geo.AngleDiff(track, heading))
		if off > bestOff || (from != nil && math.Abs(geo.AngleDiff(*from, heading+turn)) > runwaySide) {
			continue
		}
		best, bestOff = name, off
	}
	return best
}

// near reports whether p is very close to home: airborne, within km in a straight line from the window.
func near(p feeds.Position, home geo.Point, homeElevationM, km float64) bool {
	if km <= 0 || p.OnGround || p.AltFt == nil {
		return false
	}
	h := float64(*p.AltFt)*0.3048/1000 - homeElevationM/1000
	d := geo.DistanceKm(home, p.Point())
	return math.Hypot(d, h) <= km
}
