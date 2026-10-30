package geo

import (
	"math"
	"testing"
)

var pearson = Point{Lat: 43.6777, Lon: -79.6248}

func TestDistanceKm(t *testing.T) {
	tests := []struct {
		name string
		to   Point
		want float64
	}{
		{"same point", pearson, 0},
		// One degree of latitude is about 111.2 km everywhere.
		{"one degree north", Point{Lat: 44.6777, Lon: -79.6248}, 111.2},
		// Billy Bishop (CYTZ), the island airport downtown: 5.6 km south and 18.4 km east of Pearson.
		{"Pearson to Billy Bishop", Point{Lat: 43.6275, Lon: -79.3962}, 19.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DistanceKm(pearson, tt.to); math.Abs(got-tt.want) > 0.2 {
				t.Fatalf("DistanceKm = %.2f, want %.1f", got, tt.want)
			}
		})
	}
}

func TestBearingDeg(t *testing.T) {
	tests := []struct {
		name string
		to   Point
		want float64
	}{
		{"north", Point{Lat: 44.6777, Lon: -79.6248}, 0},
		{"south", Point{Lat: 42.6777, Lon: -79.6248}, 180},
		{"east", Point{Lat: 43.6777, Lon: -79.5}, 90},
		{"west", Point{Lat: 43.6777, Lon: -79.7}, 270},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BearingDeg(pearson, tt.to)
			// East and west bend slightly off 90/270 on a sphere; a tenth of a degree is plenty here.
			if diff := math.Abs(got - tt.want); diff > 0.1 && diff < 359.9 {
				t.Fatalf("BearingDeg = %.3f, want %.0f", got, tt.want)
			}
		})
	}
}

func TestValid(t *testing.T) {
	if !pearson.Valid() {
		t.Fatal("Pearson should be valid")
	}
	for _, p := range []Point{{Lat: 91}, {Lon: -181}} {
		if p.Valid() {
			t.Fatalf("%+v should be invalid", p)
		}
	}
}
