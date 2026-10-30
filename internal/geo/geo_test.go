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

func TestDestination(t *testing.T) {
	for _, tt := range []struct {
		bearing, km float64
	}{{0, 10}, {90, 25}, {225, 60}, {359, 3}} {
		p := Destination(pearson, tt.bearing, tt.km)
		if d := DistanceKm(pearson, p); math.Abs(d-tt.km) > 0.01 {
			t.Errorf("Destination(%v°, %v km) is %.3f km away", tt.bearing, tt.km, d)
		}
		if b := BearingDeg(pearson, p); math.Abs(AngleDiff(b, tt.bearing)) > 0.1 {
			t.Errorf("Destination(%v°, %v km) lies at %.2f°", tt.bearing, tt.km, b)
		}
	}
}

func TestElevationDeg(t *testing.T) {
	tests := []struct {
		name           string
		heightKm, dist float64
		want           float64
	}{
		{"straight up", 1, 0, 90},
		{"45 degrees", 1, 1, 45},
		// 10,000 ft (3.05 km) at 30 km: atan(3.05/30) is 5.8°, less about 0.1° for the curve.
		{"10,000 ft at 30 km", 3.048, 30, 5.7},
		// A plane on the ground 40 km away is below the horizon.
		{"ground at 40 km", 0, 40, -0.1},
	}
	for _, tt := range tests {
		if got := ElevationDeg(tt.heightKm, tt.dist); math.Abs(got-tt.want) > 0.06 {
			t.Errorf("%s: ElevationDeg = %.3f, want %.1f", tt.name, got, tt.want)
		}
	}
}

func TestAngleDiff(t *testing.T) {
	for _, tt := range []struct{ a, b, want float64 }{{10, 350, 20}, {350, 10, -20}, {180, 0, -180}, {90, 90, 0}} {
		if got := AngleDiff(tt.a, tt.b); got != tt.want {
			t.Errorf("AngleDiff(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
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
