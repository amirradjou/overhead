package live

import (
	"slices"
	"time"
)

// Prediction accuracy, for M2's "coming next is right to within a minute": the first prediction made for a plane
// between minLead and maxLead ahead is remembered, and checked when the plane comes into view or doesn't.
const (
	minLead = time.Minute
	maxLead = 5 * time.Minute
	// A plane that hasn't appeared this long after its predicted time was a wrong prediction.
	missedAfter = 3 * time.Minute
	keepErrors  = 500
)

type prediction struct {
	due  time.Time
	side string
}

// Predictions is the tally since the process started.
type Predictions struct {
	Checked int `json:"checked"`
	// WithinMinute counts planes that came into view less than a minute from the predicted time.
	WithinMinute int `json:"within_minute"`
	// Missed counts predictions of planes that never came into view (they turned, or landed short).
	Missed int `json:"missed"`
	// MedianErrorSec is the median of |actual - predicted| over the planes that did appear.
	MedianErrorSec float64 `json:"median_error_s"`
	Share          float64 `json:"share_within_minute"`
}

// trackPredictions updates the tally with one view; the caller holds s.mu.
func (s *State) trackPredictions(v View) {
	if s.predicted == nil {
		s.predicted = make(map[string]prediction)
	}
	for _, p := range v.Planes {
		pr, pending := s.predicted[p.Hex]
		switch {
		case p.InView && pending:
			err := v.At.Sub(pr.due)
			s.predErrors = append(s.predErrors, err.Abs().Seconds())
			if len(s.predErrors) > keepErrors {
				s.predErrors = s.predErrors[1:]
			}
			s.predChecked++
			if err.Abs() < time.Minute {
				s.predWithin++
			}
			delete(s.predicted, p.Hex)
		case !pending && p.Coming != nil:
			lead := time.Duration(p.Coming.InSec * float64(time.Second))
			if lead >= minLead && lead <= maxLead {
				s.predicted[p.Hex] = prediction{due: v.At.Add(lead), side: p.Coming.Side}
			}
		}
	}
	for hex, pr := range s.predicted {
		if v.At.Sub(pr.due) > missedAfter {
			s.predChecked++
			s.predMissed++
			delete(s.predicted, hex)
		}
	}
}

// Predictions returns the tally of "coming next" against what happened.
func (s *State) Predictions() Predictions {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Predictions{Checked: s.predChecked, WithinMinute: s.predWithin, Missed: s.predMissed}
	if n := len(s.predErrors); n > 0 {
		sorted := slices.Sorted(slices.Values(s.predErrors))
		p.MedianErrorSec = sorted[n/2]
	}
	if p.Checked > 0 {
		p.Share = float64(p.WithinMinute) / float64(p.Checked)
	}
	return p
}
