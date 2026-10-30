package live

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/amirradjou/overhead/internal/geo"
)

// windowVectors are shared with analysis/tests/test_window_vectors.py: "in view" is worked out here for the screen and
// heads-ups, and in SQL for the facts, and the two must not drift.
type windowVectors struct {
	Home  geo.Point `json:"home"`
	Cases []struct {
		Name   string  `json:"name"`
		Window Window  `json:"window"`
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		AltFt  float64 `json:"alt_ft"`
		InView bool    `json:"in_view"`
	} `json:"cases"`
}

func TestWindowVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/window_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v windowVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) < 10 || !v.Home.Valid() || v.Home.Lat == 0 {
		t.Fatalf("vectors did not load: %d cases, home %+v", len(v.Cases), v.Home)
	}
	for _, c := range v.Cases {
		w := c.Window.normalised()
		s := w.sight(v.Home, geo.Point{Lat: c.Lat, Lon: c.Lon}, c.AltFt*0.3048/1000)
		if got := w.sees(s); got != c.InView {
			t.Errorf("%s: in view = %v, want %v (elevation %.2f, offset %.1f, %.1f km)", c.Name, got, c.InView, s.elevation, s.offset, s.distance)
		}
	}
}
