package live

import (
	"testing"
	"time"

	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/feeds"
)

func viewAt(at time.Time, planes ...Plane) View { return View{At: at, Planes: planes} }

func comingIn(hex string, sec float64) Plane {
	return Plane{Position: feeds.Position{Hex: hex}, Coming: &Coming{InSec: sec, Side: "left"}}
}

func inView(hex string) Plane { return Plane{Position: feeds.Position{Hex: hex}, InView: true} }

func TestPredictionsTally(t *testing.T) {
	s := &State{}
	// Predicted 2 min ahead; appears 30 s late: right to within a minute.
	s.trackPredictions(viewAt(t0, comingIn("good", 120), comingIn("late", 90), comingIn("gone", 180), comingIn("soon", 20)))
	// Later predictions for the same plane don't replace the first.
	s.trackPredictions(viewAt(t0.Add(10*time.Second), comingIn("good", 300)))
	s.trackPredictions(viewAt(t0.Add(150*time.Second), inView("good")))
	// Predicted at 90 s, appears at 180 s: 90 s off.
	s.trackPredictions(viewAt(t0.Add(180*time.Second), inView("late"), inView("soon")))
	// "gone" was due at 180 s and never came; three minutes after that it counts as missed.
	s.trackPredictions(viewAt(t0.Add(361 * time.Second)))

	p := s.Predictions()
	if p.Checked != 3 || p.WithinMinute != 1 || p.Missed != 1 {
		t.Fatalf("tally = %+v; want good within, late off by 90 s, gone missed, soon (20 s lead) ignored", p)
	}
	if p.MedianErrorSec != 90 || p.Share < 0.33 || p.Share > 0.34 {
		t.Fatalf("tally = %+v", p)
	}
}

func TestPredictionsCountedWithoutAScreen(t *testing.T) {
	// Server.Run works the view out on every tick whether or not a stream is open, and View feeds the tally.
	s := &State{Center: pearson, RadiusKm: 60, Window: south}
	far := flying(110, 25, 10000, 270, 250, 0) // east of the window, reaching its left edge in about two minutes
	s.Update(eventFor(far, t0))
	s.View(t0)
	if pr, ok := s.predicted["abc123"]; !ok || pr.side != "left" || pr.due.Sub(t0) < minLead {
		t.Fatalf("prediction = %+v, %v", pr, ok)
	}
}

func eventFor(p feeds.Position, at time.Time) events.Position {
	p.At = at
	return events.Position{Position: p}
}
