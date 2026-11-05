// Package poller asks every feed for the circle round home on a fixed interval, merges the answers into the current
// picture of the sky and keeps the numbers M0 is judged on: gaps between updates and duplicate aircraft.
package poller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
)

// Defaults for the zero values of Poller's settings.
const (
	DefaultInterval = 5 * time.Second
	DefaultMaxAge   = 60 * time.Second
	// maxBackoff caps how long a failing feed is left alone.
	maxBackoff = 2 * time.Minute
	// rateLimitWait is how long a feed that answered 429 without a Retry-After is left alone.
	rateLimitWait = time.Minute
)

// Poller polls feeds and keeps the merged picture. It is not safe for concurrent use; Run drives it from one goroutine.
//
// The picture is the merge of every feed's latest answer that is younger than MaxAge. So an aircraft leaves once each
// feed has answered again without it, a feed that fails or sits out a poll keeps showing what it last saw for a while,
// and the freshest report of each aircraft wins whichever feed sent it.
type Poller struct {
	Feeds []feeds.Feed
	// Spacing is the least time between two requests to one feed, by feed name. A feed with a spacing longer than
	// Interval sits out the polls in between; the others are asked every poll.
	Spacing  map[string]time.Duration
	Center   geo.Point
	RadiusKm float64
	Interval time.Duration
	// MaxAge drops a feed's answer, and any position, older than this.
	MaxAge time.Duration

	started bool
	state   []feedState
	picture []feeds.Position
	stats   Stats
	seen    map[string]struct{}
}

type feedState struct {
	backoff
	// every is how many polls apart the feed is asked; idle counts polls since it last was.
	every, idle int
	last        []feeds.Position
	lastAt      time.Time
}

// Aircraft is one aircraft in the picture, placed relative to home.
type Aircraft struct {
	feeds.Position
	DistanceKm float64
	BearingDeg float64
}

// FeedResult is what one feed did in one poll.
type FeedResult struct {
	Name  string
	Count int
	Took  time.Duration
	Err   error
	// Waiting is set when the feed was not asked this time because it is backing off after errors.
	Waiting bool
	// Resting is set when the feed was not asked this time because of its Spacing; Count is then its last answer's.
	Resting bool
}

// Snapshot is the picture after one poll.
type Snapshot struct {
	At       time.Time
	Aircraft []Aircraft // nearest first
	// Answers is each feed's own latest answer, unmerged, by feed name, while it is younger than MaxAge.
	Answers map[string][]feeds.Position
	Feeds   []FeedResult
	Stats   Stats
}

// Stats summarises a run so far.
type Stats struct {
	Started time.Time
	Polls   int
	// Updates counts polls in which at least one feed was asked and answered.
	Updates    int
	LastUpdate time.Time
	// MaxGap is the longest time without an update, counting from Started.
	MaxGap time.Duration
	Feeds  []FeedStats
	// Distinct is how many different aircraft have been in the picture; MaxAtOnce the most at one time.
	Distinct  int
	MaxAtOnce int
	// Duplicates counts polls in which one aircraft looked listed twice (see duplicate).
	Duplicates    int
	LastDuplicate string
}

// FeedStats counts one feed's answers.
type FeedStats struct {
	Name              string
	OK, Errors, Waits int
	LastError         string
	LastErrorAt       time.Time
}

// GapUntil is the longest gap including the one still open at end.
func (s Stats) GapUntil(end time.Time) time.Duration {
	last := s.LastUpdate
	if last.IsZero() {
		last = s.Started
	}
	return max(s.MaxGap, end.Sub(last))
}

// Run polls now and then every Interval until ctx ends, calling emit with each snapshot, and returns the run's stats.
func (p *Poller) Run(ctx context.Context, emit func(Snapshot)) Stats {
	interval := p.interval()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if snap, ok := p.Poll(ctx, time.Now()); ok {
			emit(snap)
		}
		select {
		case <-ctx.Done():
			s := p.Stats()
			if s.Started.IsZero() {
				s.Started = time.Now()
			}
			return s
		case <-tick.C:
		}
	}
}

// Stats returns a copy of the stats so far.
func (p *Poller) Stats() Stats {
	s := p.stats
	s.Feeds = slices.Clone(s.Feeds)
	return s
}

// Poll asks the feeds that are due and updates the picture. It reports false, changing nothing, when ctx ended
// mid-poll.
func (p *Poller) Poll(ctx context.Context, now time.Time) (Snapshot, bool) {
	if !p.started {
		p.start(now)
	}
	results, answered := p.fetchAll(ctx, now)
	if ctx.Err() != nil {
		return Snapshot{}, false
	}

	cutoff := now.Add(-p.maxAge())
	var lists [][]feeds.Position
	answers := make(map[string][]feeds.Position)
	for i, st := range p.state {
		if !st.lastAt.IsZero() && !st.lastAt.Before(cutoff) {
			lists = append(lists, st.last)
			answers[p.Feeds[i].Name()] = st.last
		}
	}
	p.picture = slices.DeleteFunc(feeds.Merge(lists...), func(pos feeds.Position) bool { return pos.At.Before(cutoff) })

	snap := Snapshot{At: now, Feeds: results, Aircraft: p.place(), Answers: answers}
	p.record(now, answered, results, snap.Aircraft)
	snap.Stats = p.Stats()
	return snap, true
}

func (p *Poller) start(now time.Time) {
	p.started = true
	p.state = make([]feedState, len(p.Feeds))
	p.seen = make(map[string]struct{})
	p.stats = Stats{Started: now, Feeds: make([]FeedStats, len(p.Feeds))}
	for i, f := range p.Feeds {
		p.stats.Feeds[i].Name = f.Name()
		every := int((p.Spacing[f.Name()] + p.interval() - 1) / p.interval()) // whole polls, rounded up
		p.state[i].every = max(every, 1)
		p.state[i].idle = p.state[i].every - 1 // so the first poll asks every feed
	}
}

// fetchAll asks the feeds that are due and not backing off, all at once, keeps each answer as that feed's latest, and
// returns each feed's result in feed order and whether any feed answered.
func (p *Poller) fetchAll(ctx context.Context, now time.Time) ([]FeedResult, bool) {
	// A poll never runs into the next one: a feed slower than the interval counts as failed.
	fetchCtx, cancel := context.WithTimeout(ctx, max(p.interval(), 2*time.Second))
	defer cancel()

	results := make([]FeedResult, len(p.Feeds))
	positions := make([][]feeds.Position, len(p.Feeds))
	var wg sync.WaitGroup
	for i, f := range p.Feeds {
		st := &p.state[i]
		results[i].Name = f.Name()
		if st.idle++; st.idle < st.every {
			results[i].Resting, results[i].Count = true, len(st.last)
			continue
		}
		if !st.ready(now) {
			results[i].Waiting = true
			continue
		}
		st.idle = 0
		wg.Go(func() {
			start := time.Now()
			positions[i], results[i].Err = f.Fetch(fetchCtx, p.Center, p.RadiusKm)
			results[i].Took = time.Since(start)
			results[i].Count = len(positions[i])
		})
	}
	wg.Wait()

	answered := false
	for i, r := range results {
		st := &p.state[i]
		switch {
		case r.Waiting, r.Resting:
		case r.Err != nil:
			var wait time.Duration
			if se, ok := errors.AsType[*feeds.StatusError](r.Err); ok {
				wait = se.RetryAfter
				if se.Code == http.StatusTooManyRequests {
					wait = max(wait, rateLimitWait)
				}
			}
			st.fail(now, p.interval(), wait)
		default:
			st.backoff = backoff{}
			st.last, st.lastAt = positions[i], now
			answered = true
		}
	}
	return results, answered
}

func (p *Poller) place() []Aircraft {
	out := make([]Aircraft, 0, len(p.picture))
	for _, pos := range p.picture {
		out = append(out, Aircraft{
			Position:   pos,
			DistanceKm: geo.DistanceKm(p.Center, pos.Point()),
			BearingDeg: geo.BearingDeg(p.Center, pos.Point()),
		})
	}
	slices.SortStableFunc(out, func(a, b Aircraft) int {
		switch {
		case a.DistanceKm < b.DistanceKm:
			return -1
		case a.DistanceKm > b.DistanceKm:
			return 1
		}
		return 0
	})
	return out
}

func (p *Poller) record(now time.Time, answered bool, results []FeedResult, aircraft []Aircraft) {
	s := &p.stats
	s.Polls++
	for i, r := range results {
		fs := &s.Feeds[i]
		switch {
		case r.Resting:
		case r.Waiting:
			fs.Waits++
		case r.Err != nil:
			fs.Errors++
			fs.LastError, fs.LastErrorAt = r.Err.Error(), now
		default:
			fs.OK++
		}
	}
	if answered {
		s.MaxGap = s.GapUntil(now)
		s.Updates++
		s.LastUpdate = now
	}

	s.MaxAtOnce = max(s.MaxAtOnce, len(aircraft))
	for _, a := range aircraft {
		p.seen[a.Hex] = struct{}{}
	}
	s.Distinct = len(p.seen)
	if dup := duplicate(aircraft); dup != "" {
		s.Duplicates++
		s.LastDuplicate = dup + " at " + now.Format(time.TimeOnly)
	}
}

// Two airborne entries this close are one aircraft under two addresses (say ADS-B and a TIS-B rebroadcast): aircraft
// in the air are kept kilometres or 1000 ft apart, while two systems' fixes of one aircraft differ by metres to a few
// hundred.
const (
	dupKm = 0.3
	dupFt = 200
)

// duplicate describes the first pair of entries that look like one aircraft listed twice: one callsign on two
// addresses, or two airborne aircraft closer than any two aircraft fly. Merging by address rules out the plain case.
func duplicate(aircraft []Aircraft) string {
	owner := make(map[string]string)
	for _, a := range aircraft {
		if a.Callsign == "" {
			continue
		}
		if other, ok := owner[a.Callsign]; ok {
			return fmt.Sprintf("%s on %s and %s", a.Callsign, other, a.Hex)
		}
		owner[a.Callsign] = a.Hex
	}
	for i, a := range aircraft {
		if a.AltFt == nil {
			continue
		}
		for _, b := range aircraft[i+1:] {
			if b.AltFt == nil || abs(*a.AltFt-*b.AltFt) > dupFt {
				continue
			}
			if d := geo.DistanceKm(a.Point(), b.Point()); d <= dupKm {
				return fmt.Sprintf("%s and %s %.0f m apart at %d ft", a.Hex, b.Hex, d*1000, *a.AltFt)
			}
		}
	}
	return ""
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (p *Poller) interval() time.Duration {
	if p.Interval > 0 {
		return p.Interval
	}
	return DefaultInterval
}

func (p *Poller) maxAge() time.Duration {
	if p.MaxAge > 0 {
		return p.MaxAge
	}
	return DefaultMaxAge
}

// backoff leaves a failing feed alone for longer after each consecutive failure: it is asked again on the next poll
// after the first failure, then sits out 1, 3, 7, ... polls, up to maxBackoff. Error answers count against the
// networks' rate limits too. Skips are counted in polls, not time, so ticker jitter can't decide whether a feed is
// asked.
type backoff struct {
	failures int
	skip     int
	// until is a time the feed asked to be left alone until (a 429's Retry-After).
	until time.Time
}

// ready reports whether to ask the feed in this poll, using up one skipped poll if not.
func (b *backoff) ready(now time.Time) bool {
	if b.skip > 0 {
		b.skip--
		return false
	}
	return !now.Before(b.until)
}

func (b *backoff) fail(now time.Time, interval, atLeast time.Duration) {
	b.failures++
	limit := max(int(maxBackoff/interval), 1)
	b.skip = 0
	for n := 1; n < b.failures && b.skip < limit; n++ {
		b.skip = 2*b.skip + 1
	}
	b.skip = min(b.skip, limit)
	b.until = now.Add(atLeast)
}
