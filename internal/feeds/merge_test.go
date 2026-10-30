package feeds

import (
	"testing"
	"time"
)

func TestMerge(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	lol := []Position{
		{Hex: "c08861", Callsign: "POE672", Lat: 43.59, Source: "adsb.lol", At: t0},
		{Hex: "c06a88", Callsign: "TSC188", TypeCode: "A21N", Registration: "C-GOIX", Lat: 43.54, Source: "adsb.lol", At: t0.Add(2 * time.Second)},
		{Hex: "~2a1b3c", Lat: 43.70, Source: "adsb.lol", At: t0},
	}
	fi := []Position{
		{Hex: "c08861", Callsign: "POE672", Lat: 43.61, Source: "adsb.fi", At: t0.Add(time.Second)},
		// Fresher position from a report without identity: the position wins, identity comes from adsb.lol.
		{Hex: "c06a88", Lat: 43.55, Source: "adsb.fi", At: t0.Add(3 * time.Second)},
		{Hex: "c0174f", Callsign: "ACA101", Source: "adsb.fi", At: t0},
	}

	got := Merge(lol, fi)
	if len(got) != 4 {
		t.Fatalf("got %d aircraft, want 4: %+v", len(got), got)
	}
	byHex := make(map[string]Position)
	for i, p := range got {
		if i > 0 && got[i-1].Hex >= p.Hex {
			t.Fatalf("not sorted by address: %q before %q", got[i-1].Hex, p.Hex)
		}
		byHex[p.Hex] = p
	}
	if p := byHex["c08861"]; p.Source != "adsb.fi" || p.Lat != 43.61 {
		t.Errorf("c08861: freshest (adsb.fi) should win, got %+v", p)
	}
	tsc := byHex["c06a88"]
	if tsc.Source != "adsb.fi" || tsc.Lat != 43.55 {
		t.Errorf("c06a88: freshest position should win, got %+v", tsc)
	}
	if tsc.Callsign != "TSC188" || tsc.TypeCode != "A21N" || tsc.Registration != "C-GOIX" {
		t.Errorf("c06a88: identity should be filled from the older report, got %+v", tsc)
	}
}

func TestMergeKeepsFlags(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	got := Merge(
		[]Position{{Hex: "a12345", Private: true, Military: true, At: t0}},
		[]Position{{Hex: "a12345", At: t0.Add(time.Second)}}, // fresher, from a network without the flags
	)
	if len(got) != 1 || !got[0].Private || !got[0].Military {
		t.Fatalf("flags lost in the merge: %+v", got)
	}
}

func TestMergeTieKeepsEarlierFeed(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	got := Merge(
		[]Position{{Hex: "abc123", Source: "first", At: t0}},
		[]Position{{Hex: "abc123", Source: "second", At: t0}},
	)
	if len(got) != 1 || got[0].Source != "first" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeEmpty(t *testing.T) {
	if got := Merge(); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}
