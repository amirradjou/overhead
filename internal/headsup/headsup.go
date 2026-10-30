// Package headsup sends a message to the phone when something worth getting up for is about to pass the window
// (README: Heads-up): an emergency squawk, a rare airframe, a low pass right over the window, a type never seen
// here before. Each aircraft gets each notice once a day.
package headsup

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/amirradjou/overhead/internal/live"
)

// Kind names a rule.
type Kind string

// The rules, most urgent first.
const (
	Emergency Kind = "emergency"
	Rare      Kind = "rare"
	Low       Kind = "low"
	NewType   Kind = "new-type"
)

// Notice is one heads-up.
type Notice struct {
	Hex  string
	Kind Kind
	Text string
	At   time.Time
}

// Key identifies a notice for deduplication: one per aircraft, rule and local day.
func (n Notice) Key() string {
	return n.At.Local().Format(time.DateOnly) + "|" + n.Hex + "|" + string(n.Kind)
}

// TypeHistory says whether a type has been seen here before; nil switches the new-type rule off.
type TypeHistory interface {
	// Known reports whether code was seen before, and records it as seen.
	Known(code string) bool
}

// Rules decides which planes in a view deserve a heads-up.
type Rules struct {
	// Rare maps ICAO type designators to names; nil means live.SpecialTypes.
	Rare map[string]string
	// LowFt and LowElevationDeg define a low pass: below this altitude and this high in the window's sky, so
	// nearly straight over it.
	LowFt           int
	LowElevationDeg float64
	// Lead is how early before a plane comes into view the heads-up goes out: time to get to the window.
	Lead  time.Duration
	Types TypeHistory
}

// Defaults for Rules' zero values.
const (
	DefaultLowFt           = 3000
	DefaultLowElevationDeg = 45
	DefaultLead            = 3 * time.Minute
)

// Check returns the notices the view calls for. It doesn't deduplicate: a Ledger does.
func (r Rules) Check(v live.View) []Notice {
	rare := r.Rare
	if rare == nil {
		rare = live.SpecialTypes
	}
	lowFt := orInt(r.LowFt, DefaultLowFt)
	lowEl := r.LowElevationDeg
	if lowEl == 0 {
		lowEl = DefaultLowElevationDeg
	}
	lead := r.Lead
	if lead == 0 {
		lead = DefaultLead
	}

	var out []Notice
	for _, p := range v.Planes {
		if p.OnGround {
			continue
		}
		name := callsign(p)
		// Soon in view, or in view already (a plane first seen inside the window gets no earlier warning).
		soon := p.InView || (p.Coming != nil && p.Coming.InSec <= lead.Seconds())
		when := whenText(p)

		if p.Squawk == "7500" || p.Squawk == "7600" || p.Squawk == "7700" || p.Emergency != "" {
			out = append(out, Notice{Hex: p.Hex, Kind: Emergency, At: v.At,
				Text: fmt.Sprintf("%s: %s, %s. %s", p.Notable, name, where(p), when)})
		}
		if !soon {
			continue
		}
		if typeName, ok := rare[p.TypeCode]; ok {
			out = append(out, Notice{Hex: p.Hex, Kind: Rare, At: v.At,
				Text: fmt.Sprintf("Rare visitor: %s (%s) %s.", typeName, name, when)})
		}
		if p.AltFt != nil && *p.AltFt < lowFt && maxElevation(p) >= lowEl {
			out = append(out, Notice{Hex: p.Hex, Kind: Low, At: v.At,
				Text: fmt.Sprintf("Low pass: %s at %d ft, nearly over the window, %s.", name, roundTo(*p.AltFt, 50), when)})
		}
		if r.Types != nil && p.TypeCode != "" && !r.Types.Known(p.TypeCode) {
			out = append(out, Notice{Hex: p.Hex, Kind: NewType, At: v.At,
				Text: fmt.Sprintf("First %s ever seen here: %s, %s.", p.TypeCode, name, when)})
		}
	}
	return out
}

func callsign(p live.Plane) string {
	switch {
	case p.Callsign != "":
		return p.Callsign
	case p.Registration != "":
		return p.Registration
	}
	return strings.ToUpper(p.Hex)
}

func where(p live.Plane) string {
	return fmt.Sprintf("%.0f km from home", p.DistanceKm)
}

func whenText(p live.Plane) string {
	if p.InView {
		return "in view now"
	}
	if p.Coming == nil {
		return "not heading for the window"
	}
	m := int(math.Round(p.Coming.InSec / 60))
	if m < 1 {
		return "in view any moment"
	}
	return fmt.Sprintf("in view in about %d min", m)
}

// maxElevation is how high in the window's sky the plane is, or will be when it comes into view.
func maxElevation(p live.Plane) float64 {
	if p.InView || p.Coming == nil {
		return p.ElevationDeg
	}
	return math.Max(p.ElevationDeg, p.Coming.ElevationDeg)
}

func orInt(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

func roundTo(v, step int) int { return (v + step/2) / step * step }
