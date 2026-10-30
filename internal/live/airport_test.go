package live

import (
	"math"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

var (
	ytz      = Airports["CYTZ"]
	ytzEnd   = events.Airport{ICAO: "CYTZ", IATA: "YTZ", Lat: 43.6275, Lon: -79.3962}
	bosEnd   = events.Airport{ICAO: "KBOS", IATA: "BOS", Lat: 42.3643, Lon: -71.0052}
	yyzEnd   = events.Airport{ICAO: "CYYZ", IATA: "YYZ", Lat: 43.6772, Lon: -79.6306}
	yulEnd   = events.Airport{ICAO: "CYUL", IATA: "YUL", Lat: 45.4706, Lon: -73.7408}
	arriving = &events.Route{Callsign: "PTR2943", Origin: bosEnd, Destination: ytzEnd}
	leaving  = &events.Route{Callsign: "PTR2635", Origin: ytzEnd, Destination: bosEnd}
)

// at is a position kmEast of Billy Bishop (west when negative), at altFt (on the ground when -1).
func at(kmEast float64, altFt, speedKt, trackDeg float64, fpm int) feeds.Position {
	pt := geo.Destination(ytz.point(), 90, kmEast)
	if kmEast < 0 {
		pt = geo.Destination(ytz.point(), 270, -kmEast)
	}
	p := feeds.Position{Hex: "c0ffee", Callsign: "PTR2943", Lat: pt.Lat, Lon: pt.Lon, SpeedKt: &speedKt, TrackDeg: &trackDeg}
	if altFt < 0 {
		p.OnGround = true
	} else {
		alt := int(altFt)
		p.AltFt = &alt
		p.VertRateFpm = &fpm
	}
	return p
}

func TestClassifyAtBillyBishop(t *testing.T) {
	for _, c := range []struct {
		name          string
		p             feeds.Position
		route         *events.Route
		op, phase, rw string // op "" means nothing to do with the airport
	}{
		{"on final for 26, from the east", at(3, 1150, 125, 251, -700), arriving, "arriving", "final", "26"},
		{"on approach, far out", at(35, 6000, 220, 250, -900), arriving, "arriving", "approach", ""},
		{"landing roll", at(0.3, -1, 95, 252, 0), arriving, "arriving", "landing", "26"},
		{"taxiing in", at(0.1, -1, 12, 120, 0), arriving, "arriving", "taxiing", ""},
		{"at the gate after landing", at(0.1, -1, 0, 0, 0), arriving, "arriving", "arrived", ""},
		{"at the gate before leaving", at(0.1, -1, 0, 0, 0), leaving, "departing", "gate", ""},
		{"take-off roll on 08", at(-0.2, -1, 90, 70, 0), leaving, "departing", "takeoff", "08"},
		{"lifting off", at(1.5, 600, 130, 72, 1800), leaving, "departing", "takeoff", "08"},
		{"climbing out", at(20, 9000, 240, 100, 2000), leaving, "departing", "climbing", ""},
		{"gone: too far out", at(50, 15000, 300, 100, 2000), leaving, "", "", ""},
		{"Pearson to Montreal, passing", at(5, 9000, 280, 70, 1500), &events.Route{Callsign: "ACA400", Origin: yyzEnd, Destination: yulEnd}, "", "", ""},
		{"no route, descending close in", at(-4, 1200, 100, 71, -600), nil, "arriving", "final", "08"},
		{"no route, level close in", at(-4, 1200, 100, 71, 0), nil, "", "", ""},
		{"no route, taxiing", at(0.2, -1, 10, 0, 0), nil, "ground", "taxiing", ""},
		{"no route, parked", at(0.2, -1, 0, 0, 0), nil, "", "", ""},
		{"on the ground at another airport", at(-26, -1, 15, 0, 0), arriving, "", "", ""},
		{"helicopter lifting off: no runway", heli(at(1.5, 600, 60, 72, 900)), nil, "departing", "takeoff", ""},
		// The circling Cessna of 2026-10-30: coming down but heading away, then climbing over the field.
		{"no route, descending but heading away", at(3, 1500, 90, 80, -400), nil, "", "", ""},
		{"no route, climbing high over the field", at(0.1, 1900, 90, 250, 800), nil, "", "", ""},
		{"take-off roll on 06", at(-0.2, -1, 90, 50, 0), leaving, "departing", "takeoff", "06"},
	} {
		got := ytz.classify(c.p, c.route)
		if c.op == "" {
			if got != nil {
				t.Errorf("%s: %+v, want nothing", c.name, got)
			}
			continue
		}
		if got == nil || got.Op != c.op || got.Phase != c.phase || got.Runway != c.rw {
			t.Errorf("%s: %+v, want %s/%s runway %q", c.name, got, c.op, c.phase, c.rw)
		}
	}
}

func heli(p feeds.Position) feeds.Position {
	p.Category, p.Callsign = "A7", "CGKLD"
	return p
}

func TestClassifyTimes(t *testing.T) {
	// 3 km out at 125 kt (231.5 km/h): 0.78 min to touchdown.
	got := ytz.classify(at(3, 1150, 125, 251, -700), arriving)
	if got.InMin == nil || math.Abs(*got.InMin-0.78) > 0.02 || got.HeightFt == nil || *got.HeightFt != 1150-252 {
		t.Fatalf("final: %+v", got)
	}
	// 20 km out at 240 kt (444.5 km/h): took off about 2.7 min ago.
	got = ytz.classify(at(20, 9000, 240, 100, 2000), leaving)
	if got.AgoMin == nil || math.Abs(*got.AgoMin-2.7) > 0.05 || got.InMin != nil {
		t.Fatalf("climbing: %+v", got)
	}
}

func TestNear(t *testing.T) {
	home := geo.Point{Lat: 43.6426, Lon: -79.3871}
	heli := func(km, altFt float64) feeds.Position {
		pt := geo.Destination(home, 180, km)
		alt := int(altFt)
		return feeds.Position{Lat: pt.Lat, Lon: pt.Lon, AltFt: &alt}
	}
	if !near(heli(1, 700), home, 218, 3) {
		t.Error("a helicopter 1 km off at the window's own height is near")
	}
	if near(heli(0.5, 37000), home, 218, 3) {
		t.Error("a jet at 37,000 ft is not near, even straight overhead")
	}
	if near(heli(1, 700), home, 218, 0) {
		t.Error("near-km 0 turns it off")
	}
	ground := heli(1, 0)
	ground.OnGround = true
	if near(ground, home, 218, 3) {
		t.Error("a plane on the ground is not a near pass")
	}
}

func TestViewFollowsTheAirport(t *testing.T) {
	a := ytz
	s := &State{Center: geo.Point{Lat: 43.6426, Lon: -79.3871}, RadiusKm: 60, Airport: &a, NearKm: 3,
		Window: Window{AzimuthDeg: 163, FOVDeg: 100, MinElevationDeg: -10, MaxElevationDeg: 30, HomeElevationM: 218}}
	pos := func(hex string, p feeds.Position) events.Position {
		p.Hex, p.At, p.Source = hex, t0, "adsb.fi"
		return events.Position{Position: p}
	}
	// It counts as an arrival when it touches down: airborne over the threshold, then on the ground.
	over := at(0.8, 300, 110, 252, -600)
	s.Update(pos("aaaaaa", over))
	landing := at(0.3, -1, 95, 252, 0)
	landing.At = t0.Add(5 * time.Second)
	s.Update(events.Position{Position: func() feeds.Position { landing.Hex, landing.Source = "aaaaaa", "adsb.fi"; return landing }()})
	s.SetAircraft(events.Aircraft{Hex: "aaaaaa", Route: arriving})
	parked := at(0.1, -1, 0, 0, 0)
	parked.Callsign = "PTR2635"
	s.Update(pos("bbbbbb", parked))
	s.SetAircraft(events.Aircraft{Hex: "bbbbbb", Route: leaving})
	far := at(30, 35000, 450, 90, 0)
	far.Callsign = "ACA400"
	s.Update(pos("cccccc", far))

	v := s.View(t0.Add(5 * time.Second))
	if !v.Focused || v.Airport == nil || v.Airport.IATA != "YTZ" || v.Airport.RunwayInUse != "26" ||
		v.Airport.ArrivalsToday != 1 || v.Airport.DeparturesToday != 0 {
		t.Fatalf("view: focused %v, airport %+v", v.Focused, v.Airport)
	}
	focus := map[string]bool{}
	for _, p := range v.Planes {
		focus[p.Hex] = p.Focus
	}
	if !focus["aaaaaa"] || focus["bbbbbb"] || focus["cccccc"] {
		t.Fatalf("focus = %v: the landing plane only, not the one at the gate or the one passing far off", focus)
	}
}

func TestRunwayNeedsTheCentreLine(t *testing.T) {
	// Lined up with 26 and heading for the field, but 4 km north of the centre line: on final, on no runway.
	pt := geo.Destination(geo.Destination(ytz.point(), 90, 3), 0, 4)
	track, speed, alt, fpm := 251.0, 125.0, 1150, -700
	p := feeds.Position{Hex: "c0ffee", Lat: pt.Lat, Lon: pt.Lon, TrackDeg: &track, SpeedKt: &speed, AltFt: &alt, VertRateFpm: &fpm}
	got := ytz.classify(p, arriving)
	if got == nil || got.Phase != "final" || got.Runway != "" {
		t.Fatalf("parallel offset: %+v", got)
	}
}
