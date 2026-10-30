package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
	"github.com/amirradjou/overhead/internal/poller"
)

// clearScreen moves the cursor home and clears the terminal, so each poll redraws the table in place.
const clearScreen = "\033[H\033[2J"

func row(w io.Writer, cols ...any) {
	line := fmt.Sprintf("%-8s %-7s %-4s %-8s %7s %6s %5s %6s %4s  %-4s  %-14s %4s  %s", cols...)
	fmt.Fprintln(w, strings.TrimRight(line, " "))
}

// renderTable draws one poll as a full-screen table, nearest aircraft first.
func renderTable(w io.Writer, s poller.Snapshot, center geo.Point, radiusKm float64) {
	fmt.Fprintf(w, "overhead · %d aircraft within %g km of %.4f, %.4f · %s\n",
		len(s.Aircraft), radiusKm, center.Lat, center.Lon, s.At.Local().Format(time.TimeOnly))
	fmt.Fprintf(w, "%s · longest gap %v · %d polls\n\n",
		feedSummary(s.Feeds, 60), s.Stats.GapUntil(s.At).Round(100*time.Millisecond), s.Stats.Polls)
	row(w, "CALLSIGN", "HEX", "TYPE", "REG", "ALT ft", "V/S", "GS kt", "km", "BRG", "SQWK", "FEED", "AGE", "")
	for _, a := range s.Aircraft {
		row(w, a.Callsign, a.Hex, a.TypeCode, a.Registration,
			altitude(a.Position), signed(a.VertRateFpm), optFloat(a.SpeedKt),
			fmt.Sprintf("%.1f", a.DistanceKm), fmt.Sprintf("%.0f", a.BearingDeg),
			a.Squawk, a.Source, age(s.At, a.At), notes(a.Position))
	}
	credits := make([]string, len(s.Feeds))
	for i, f := range s.Feeds {
		credits[i] = feeds.Credit(f.Name)
	}
	fmt.Fprintf(w, "\nData: %s\n", strings.Join(credits, " · "))
}

// renderLine writes one poll as a log line, for when stdout is not a terminal.
func renderLine(w io.Writer, s poller.Snapshot) {
	line := fmt.Sprintf("%s  %d aircraft  %s", s.At.Local().Format(time.TimeOnly), len(s.Aircraft), feedSummary(s.Feeds, 0))
	if len(s.Aircraft) > 0 {
		a := s.Aircraft[0]
		line += fmt.Sprintf("  nearest %s %.1f km", orElse(a.Callsign, a.Hex), a.DistanceKm)
	}
	fmt.Fprintln(w, line)
}

// renderSummary prints the run's numbers and checks them against M0: every feed contributed, aircraft were seen, no gap
// over maxGap, no duplicate aircraft. It returns an error when the check fails.
func renderSummary(w io.Writer, s poller.Stats, end time.Time, maxGap time.Duration) error {
	gap := s.GapUntil(end)
	fmt.Fprintf(w, "\nran %v: %d polls, %d with data from at least one feed\n",
		end.Sub(s.Started).Round(time.Second), s.Polls, s.Updates)
	fmt.Fprintf(w, "longest gap between updates: %v (limit %v)\n", gap.Round(100*time.Millisecond), maxGap)
	for _, f := range s.Feeds {
		line := fmt.Sprintf("  %-14s %d ok, %d errors, %d polls sat out", f.Name, f.OK, f.Errors, f.Waits)
		if f.LastError != "" {
			line += fmt.Sprintf("; last error at %s: %s", f.LastErrorAt.Local().Format(time.TimeOnly), feeds.Printable(f.LastError))
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintf(w, "aircraft: %d distinct, at most %d at once\n", s.Distinct, s.MaxAtOnce)
	if s.Duplicates == 0 {
		fmt.Fprintln(w, "duplicates: none (in no poll one callsign on two addresses, or two airborne aircraft within 300 m and 200 ft)")
	} else {
		fmt.Fprintf(w, "duplicates: in %d polls; last: %s\n", s.Duplicates, s.LastDuplicate)
	}

	var failed []string
	for _, f := range s.Feeds {
		if f.OK == 0 {
			failed = append(failed, f.Name+" never answered")
		}
	}
	if s.Distinct == 0 {
		failed = append(failed, "no aircraft seen")
	}
	if gap > maxGap {
		failed = append(failed, fmt.Sprintf("updates stopped for %v", gap.Round(time.Second)))
	}
	if s.Duplicates > 0 {
		failed = append(failed, "duplicate aircraft")
	}
	if len(failed) > 0 {
		fmt.Fprintln(w, "check: FAIL")
		return errors.New("check failed: " + strings.Join(failed, "; "))
	}
	fmt.Fprintln(w, "check: pass")
	return nil
}

// feedSummary says how each feed did in one poll; limit > 0 shortens error messages to fit a table header.
func feedSummary(results []poller.FeedResult, limit int) string {
	parts := make([]string, len(results))
	for i, r := range results {
		switch {
		case r.Resting:
			parts[i] = fmt.Sprintf("%s %d (earlier)", r.Name, r.Count)
		case r.Waiting:
			parts[i] = r.Name + " backing off"
		case r.Err != nil:
			parts[i] = r.Name + " error: " + clip(feeds.Printable(reason(r)), limit)
		default:
			parts[i] = fmt.Sprintf("%s %d (%dms)", r.Name, r.Count, r.Took.Milliseconds())
		}
	}
	return strings.Join(parts, " · ")
}

// reason is the error without the feed name and URL the transport wraps it in.
func reason(r poller.FeedResult) string {
	if ue, ok := errors.AsType[*url.Error](r.Err); ok {
		return ue.Err.Error()
	}
	return strings.TrimPrefix(r.Err.Error(), r.Name+": ")
}

func clip(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

func altitude(p feeds.Position) string {
	switch {
	case p.OnGround:
		return "ground"
	case p.AltFt == nil:
		return ""
	}
	return fmt.Sprint(*p.AltFt)
}

func signed(n *int) string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%+d", *n)
}

func optFloat(f *float64) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%.0f", *f)
}

func age(now, at time.Time) string {
	return fmt.Sprintf("%ds", max(0, int(now.Sub(at).Seconds())))
}

func notes(p feeds.Position) string {
	var n []string
	if p.Emergency != "" {
		n = append(n, "EMERGENCY "+p.Emergency)
	}
	if p.Military {
		n = append(n, "military")
	}
	if p.Private {
		n = append(n, "private")
	}
	return strings.Join(n, ", ")
}

func orElse(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
