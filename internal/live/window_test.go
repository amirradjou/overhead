package live

import (
	"math"
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

// A south-facing window, 90° wide, that sees down to 5° over the houses opposite.
var south = Window{AzimuthDeg: 180, FOVDeg: 90, MinElevationDeg: 5}

// flying returns a plane bearingDeg/km from Pearson at altFt, on trackDeg at kt and climbing at fpm.
func flying(bearingDeg, km float64, altFt int, trackDeg, kt float64, fpm int) feeds.Position {
	p := geo.Destination(pearson, bearingDeg, km)
	return feeds.Position{
		Hex: "abc123", Lat: p.Lat, Lon: p.Lon, AltFt: &altFt, TrackDeg: &trackDeg, SpeedKt: &kt, VertRateFpm: &fpm, At: t0,
	}
}

func TestSightInView(t *testing.T) {
	p := flying(180, 15, 10000, 90, 250, 0)
	h, _ := heightKm(p)
	s := south.sight(pearson, p.Point(), h)
	if !south.sees(s) || math.Abs(s.offset) > 0.1 || s.elevation < 11 || s.elevation > 12 {
		t.Fatalf("a plane 15 km south at 10,000 ft should be in view near 11.5°, dead centre: %+v", s)
	}
	if south.coming(pearson, p, t0) != nil {
		t.Fatal("a plane already in view is not coming")
	}
	east := flying(90, 15, 10000, 90, 250, 0)
	h, _ = heightKm(east)
	if s := south.sight(pearson, east.Point(), h); south.sees(s) || s.offset > -89 || s.offset < -91 {
		t.Fatalf("a plane due east is 90° left of a south window, out of view: %+v", s)
	}
}

func TestComingAroundTheLeftEdge(t *testing.T) {
	// East-south-east, 20 km out, flying west at 250 kt: it crosses into the window's left edge in about a minute.
	c := south.coming(pearson, flying(120, 20, 10000, 270, 250, 0), t0)
	if c == nil || c.Side != "left" || c.InSec < 50 || c.InSec > 65 || c.OffsetDeg > -40 {
		t.Fatalf("coming = %+v", c)
	}
}

func TestComingOverTheHouse(t *testing.T) {
	// From behind the house, heading straight over it.
	c := south.coming(pearson, flying(0, 5, 5000, 180, 200, 0), t0)
	if c == nil || c.Side != "above" || c.ElevationDeg < overheadDeg {
		t.Fatalf("coming = %+v", c)
	}
}

func TestComingFromTheDistance(t *testing.T) {
	// 60 km south, heading straight for the window at 450 kt: in view once within 40 km, about 90 s away.
	c := south.coming(pearson, flying(180, 60, 30000, 0, 450, 0), t0)
	if c == nil || c.Side != "distance" || c.InSec < 80 || c.InSec > 95 {
		t.Fatalf("coming = %+v", c)
	}
}

func TestComingOverTheRooftops(t *testing.T) {
	// Low and far inside the window's slice, climbing toward home: it rises above the houses opposite.
	c := south.coming(pearson, flying(180, 20, 1000, 0, 160, 2000), t0)
	if c == nil || c.Side != "rooftops" || c.ElevationDeg < south.MinElevationDeg {
		t.Fatalf("coming = %+v", c)
	}
}

func TestNotComing(t *testing.T) {
	// Flying away from the window's slice, on the ground, or without a speed: nothing to predict.
	away := flying(90, 15, 10000, 90, 250, 0)
	ground := flying(180, 2, 0, 180, 15, 0)
	ground.OnGround, ground.AltFt = true, nil
	still := flying(120, 20, 10000, 270, 250, 0)
	still.SpeedKt = nil
	for name, p := range map[string]feeds.Position{"away": away, "ground": ground, "no speed": still} {
		if c := south.coming(pearson, p, t0); c != nil {
			t.Errorf("%s: coming = %+v", name, c)
		}
	}
	// Landing before it gets there: descending 1,500 ft a minute from 2,000 ft, 30 km out.
	if c := south.coming(pearson, flying(180, 30, 2000, 0, 150, -1500), t0); c != nil {
		t.Errorf("a plane that lands before reaching the window: %+v", c)
	}
}

func TestViewPlacesPlanesInTheWindow(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 60, Window: south}
	s.Update(events.Position{Position: flying(180, 15, 10000, 90, 250, 0)})
	left := flying(120, 20, 10000, 270, 250, 0)
	left.Hex = "def456"
	s.Update(events.Position{Position: left})

	v := s.View(t0)
	if v.Window != south {
		t.Fatalf("window = %+v", v.Window)
	}
	byHex := map[string]Plane{}
	for _, p := range v.Planes {
		byHex[p.Hex] = p
	}
	if p := byHex["abc123"]; !p.InView || p.Coming != nil || p.ElevationDeg < 11 {
		t.Errorf("in view: %+v", p)
	}
	if p := byHex["def456"]; p.InView || p.Coming == nil || p.Coming.Side != "left" || p.OffsetDeg > -45 {
		t.Errorf("coming: %+v", p)
	}
}

func TestMotion(t *testing.T) {
	// Due south and flying east at 250 kt: from a south window it moves left across the glass, level.
	p := flying(180, 15, 10000, 90, 250, 0)
	h, _ := heightKm(p)
	m := south.motion(pearson, p, 0, south.sight(pearson, p.Point(), h))
	if m == nil || m.OffsetDeg > -4 || m.OffsetDeg < -6 || math.Abs(m.ElevationDeg) > 0.2 {
		t.Fatalf("motion = %+v, want about 5° left and level", m)
	}
}

func TestDefaultWindowSeesAllRound(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 60}
	s.Update(events.Position{Position: flying(0, 15, 10000, 90, 250, 0)})
	v := s.View(t0)
	if v.Window.FOVDeg != 360 || !v.Planes[0].InView {
		t.Fatalf("view = %+v", v)
	}
}

func TestPlacedWhereItIsNow(t *testing.T) {
	// Reported 20 s ago just outside the window's left edge and flying into it at 250 kt (2.6 km in 20 s): by now
	// it is inside, and the view should say so rather than call it neither in view nor coming.
	p := flying(136.5, 14, 10000, 270, 250, 0)
	s := &State{Center: pearson, RadiusKm: 60, Window: south}
	p.At = t0.Add(-20 * time.Second)
	s.Update(events.Position{Position: p})
	got := s.View(t0).Planes[0]
	if !got.InView || got.Coming != nil {
		t.Fatalf("plane = in view %v, coming %+v, offset %.1f", got.InView, got.Coming, got.OffsetDeg)
	}
	// A report hours old is not carried across the map: dead reckoning stops at maxDeadReckon.
	if a := reportAge(feeds.Position{At: t0.Add(-time.Hour)}, t0); a != maxDeadReckon.Seconds() {
		t.Fatalf("reportAge = %v", a)
	}
}

func TestWindowAzimuthNormalised(t *testing.T) {
	for in, want := range map[float64]float64{540: 180, -90: 270, 360: 0, 200: 200} {
		if got := (Window{AzimuthDeg: in, FOVDeg: 90}).normalised().AzimuthDeg; got != want {
			t.Errorf("normalised(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestTodayOnlyMovesForward(t *testing.T) {
	s := &State{Center: pearson, RadiusKm: 60}
	midnight := time.Date(2026, 10, 31, 0, 0, 0, 0, time.Local)
	s.Update(events.Position{Position: feeds.Position{Hex: "aaaaaa", Lat: 43.7, Lon: -79.6, At: midnight.Add(time.Minute)}})
	s.Update(events.Position{Position: feeds.Position{Hex: "bbbbbb", Lat: 43.7, Lon: -79.6, At: midnight.Add(time.Minute)}})
	// A late report from before midnight, for another aircraft.
	s.Update(events.Position{Position: feeds.Position{Hex: "cccccc", Lat: 43.7, Lon: -79.6, At: midnight.Add(-time.Minute)}})
	if v := s.View(midnight.Add(2 * time.Minute)); v.Today != 2 {
		t.Fatalf("today = %d, want 2: a late report must not reset the day", v.Today)
	}
}
