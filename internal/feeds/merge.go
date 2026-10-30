package feeds

import (
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

func fillIdentity(p, from Position) Position {
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
