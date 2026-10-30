package feeds

import (
	"regexp"
	"slices"
	"strings"
)

// Merge combines reports from several feeds into one per aircraft, keyed by ICAO address. The freshest position wins;
// identity fields it lacks (callsign, registration, type, category) are filled from the other reports, because one
// network's aircraft database sometimes knows an airframe the other doesn't. For the same reason the military and
// private flags are kept if any report sets them. On a tie the earlier list wins. The result is sorted by address.
func Merge(reports ...[]Position) []Position {
	byHex := make(map[string]Position)
	for _, list := range reports {
		for _, p := range list {
			cur, ok := byHex[p.Hex]
			if !ok {
				byHex[p.Hex] = p
				continue
			}
			newer, older := cur, p
			if p.At.After(cur.At) {
				newer, older = p, cur
			}
			byHex[p.Hex] = fillIdentity(newer, older)
		}
	}
	out := make([]Position, 0, len(byHex))
	for _, p := range byHex {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Position) int { return strings.Compare(a.Hex, b.Hex) })
	return out
}

// helicopter matches ICAO designators of helicopters: a type that cannot be right for an aircraft whose emitter
// category is not A7 (rotorcraft).
var helicopter = regexp.MustCompile(`^(R22|R44|R66|B06|B407|B412|B429|B505|EC\d\d|AS\d\d|A1[0-6]9|AW\d+|S76|H60|BK17|MD[56]\d\d?)$`)

func fillIdentity(p, from Position) Position {
	// Two feeds' aircraft databases can disagree on the type (c012ed: an A220 in one, an AS350 helicopter in the
	// other); keep the one the emitter category allows, so the type does not flip from one report to the next.
	cat := p.Category
	if cat == "" {
		cat = from.Category
	}
	rotor := cat == "A7"
	if cat != "" && p.TypeCode != "" && from.TypeCode != "" && p.TypeCode != from.TypeCode &&
		helicopter.MatchString(p.TypeCode) != rotor && helicopter.MatchString(from.TypeCode) == rotor {
		p.TypeCode = from.TypeCode
	}
	if p.Callsign == "" {
		p.Callsign = from.Callsign
	}
	if p.Registration == "" {
		p.Registration = from.Registration
	}
	if p.TypeCode == "" {
		p.TypeCode = from.TypeCode
	}
	if p.Category == "" {
		p.Category = from.Category
	}
	p.Military = p.Military || from.Military
	p.Private = p.Private || from.Private
	return p
}
