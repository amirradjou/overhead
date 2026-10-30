package live

import (
	"math"
	"time"

	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

// Window is the view from home: the direction the window faces, how wide a slice of sky it shows, and the lowest
// angle above the horizon you can see over whatever stands opposite.
type Window struct {
	AzimuthDeg      float64 `json:"azimuth_deg"`
	FOVDeg          float64 `json:"fov_deg"`
	MinElevationDeg float64 `json:"min_elevation_deg"`
	// HomeElevationM is the window's height above sea level, which aircraft altitudes are measured from too.
	HomeElevationM float64 `json:"home_elevation_m"`
}

// DefaultWindow sees all round down to 5°, the view from a balcony or a field, until settings describe a window.
var DefaultWindow = Window{FOVDeg: 360, MinElevationDeg: 5}

// visibleKm is how far away a plane may be and still count as in view: beyond it an airliner is a speck by day and
// one light among many at night.
const visibleKm = 40.0

// How far ahead "coming next" looks, and in what steps.
const (
	lookAhead = 10 * time.Minute
	step      = 5 * time.Second
)

// overheadDeg: a plane entering the window's slice of sky this high came over the house, not round its side.
const overheadDeg = 60.0

func (w Window) normalised() Window {
	if w.FOVDeg <= 0 || w.FOVDeg > 360 {
		w.FOVDeg = DefaultWindow.FOVDeg
	}
	w.AzimuthDeg = math.Mod(math.Mod(w.AzimuthDeg, 360)+360, 360)
	return w
}

// sight is where something heightKm above sea level at p is seen from the window at home.
type sight struct {
	elevation, offset, distance float64
}

func (w Window) sight(home, p geo.Point, heightKm float64) sight {
	d := geo.DistanceKm(home, p)
	return sight{
		elevation: geo.ElevationDeg(heightKm-w.HomeElevationM/1000, d),
		offset:    geo.AngleDiff(geo.BearingDeg(home, p), w.AzimuthDeg),
		distance:  d,
	}
}

func (w Window) sees(s sight) bool {
	inSlice := w.FOVDeg >= 360 || math.Abs(s.offset) <= w.FOVDeg/2
	return inSlice && s.elevation >= w.MinElevationDeg && s.distance <= visibleKm
}

// Coming is a plane about to come into view.
type Coming struct {
	InSec float64 `json:"in_s"`
	// Side is where it appears: "left" or "right" (round the window's edge), "rooftops" (rising over what stands
	// opposite), "above" (over the house) or "distance" (closing in from far off, straight ahead).
	Side string `json:"side"`
	// OffsetDeg and ElevationDeg place where it appears.
	OffsetDeg    float64 `json:"offset_deg"`
	ElevationDeg float64 `json:"elevation_deg"`
}

func heightKm(p feeds.Position) (float64, bool) {
	if p.OnGround || p.AltFt == nil {
		return 0, false
	}
	return float64(*p.AltFt) * 0.3048 / 1000, true
}

// coming projects p along its track, at its speed and rate of climb, and returns when and where it first comes into
// view within lookAhead, or nil. Straight lines only: a plane turning onto final approach will surprise it, which is
// what M2's field check measures.
func (w Window) coming(home geo.Point, p feeds.Position, now time.Time) *Coming {
	if p.SpeedKt == nil || p.TrackDeg == nil || *p.SpeedKt < 30 {
		return nil
	}
	at := func(t float64) (sight, bool) {
		pt, h, ok := project(p, t)
		if !ok {
			return sight{}, false
		}
		return w.sight(home, pt, h), true
	}
	age := reportAge(p, now)
	prev, ok := at(age)
	if !ok || w.sees(prev) {
		return nil
	}
	for t := age + step.Seconds(); t <= age+lookAhead.Seconds(); t += step.Seconds() {
		s, ok := at(t)
		if !ok {
			return nil
		}
		if w.sees(s) {
			return &Coming{InSec: t - age, Side: w.side(prev), OffsetDeg: s.offset, ElevationDeg: s.elevation}
		}
		prev = s
	}
	return nil
}

// maxDeadReckon caps how far a report is carried forward to now. Reports arrive up to a poll or two late (adsb.lol
// is asked every 15 s); further than this, a straight line says less than the last fix did.
const maxDeadReckon = 30 * time.Second

// reportAge is how many seconds p's report is carried forward to reach now.
func reportAge(p feeds.Position, now time.Time) float64 {
	return min(max(0, now.Sub(p.At).Seconds()), maxDeadReckon.Seconds())
}

// project places p t seconds after its report: carried along its track at its speed and rate of climb, or left where
// it was without a track and speed. ok is false without an altitude, on the ground, or once it would have landed.
func project(p feeds.Position, t float64) (geo.Point, float64, bool) {
	h, ok := heightKm(p)
	if !ok {
		return geo.Point{}, 0, false
	}
	if p.SpeedKt == nil || p.TrackDeg == nil || t <= 0 {
		return p.Point(), h, true
	}
	if p.VertRateFpm != nil {
		h += float64(*p.VertRateFpm) * 0.3048 / 60 / 1000 * t
	}
	if h < 0 {
		return geo.Point{}, 0, false
	}
	return geo.Destination(p.Point(), *p.TrackDeg, *p.SpeedKt*geo.KmPerNM/3600*t), h, true
}

// Motion is how a plane moves across the window: degrees right (+) and up (+) over the next motionSec seconds.
type Motion struct {
	OffsetDeg    float64 `json:"offset_deg"`
	ElevationDeg float64 `json:"elevation_deg"`
}

// motionSec is the look-ahead for Motion: long enough to show a direction, short enough to stay straight.
const motionSec = 10.0

// motion is where p heads on screen, from its sight at age seconds after its report and motionSec later; nil
// without a track and speed.
func (w Window) motion(home geo.Point, p feeds.Position, age float64, now sight) *Motion {
	if p.SpeedKt == nil || p.TrackDeg == nil {
		return nil
	}
	pt, h, ok := project(p, age+motionSec)
	if !ok {
		return nil
	}
	next := w.sight(home, pt, h)
	return &Motion{OffsetDeg: geo.AngleDiff(next.offset, now.offset), ElevationDeg: next.elevation - now.elevation}
}

// side names where a plane appears from, given where it was just before.
func (w Window) side(before sight) string {
	switch {
	case w.FOVDeg < 360 && math.Abs(before.offset) > w.FOVDeg/2:
		if before.elevation > overheadDeg {
			return "above"
		}
		if before.offset < 0 {
			return "left"
		}
		return "right"
	case before.elevation < w.MinElevationDeg:
		return "rooftops"
	case before.distance > visibleKm:
		return "distance"
	}
	return "above"
}
