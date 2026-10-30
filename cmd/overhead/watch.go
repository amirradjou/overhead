package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amirradjou/overhead/internal/buildinfo"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/geo"
	"github.com/amirradjou/overhead/internal/poller"
)

// Toronto Pearson. The default centre is the airport, not anyone's house; set OVERHEAD_LAT/OVERHEAD_LON for home.
var defaultCenter = geo.Point{Lat: 43.6777, Lon: -79.6248}

func runWatch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	center, err := envCenter()
	if err != nil {
		return usageError{err.Error()}
	}
	fs.Float64Var(&center.Lat, "lat", center.Lat, "latitude of the centre (OVERHEAD_LAT)")
	fs.Float64Var(&center.Lon, "lon", center.Lon, "longitude of the centre (OVERHEAD_LON)")
	radius := fs.Float64("radius", 30, "radius in km")
	interval := fs.Duration("interval", poller.DefaultInterval, "time between polls")
	feedList := fs.String("feeds", "adsb.lol,adsb.fi", "comma-separated feeds: "+strings.Join(feeds.Names(), ", "))
	runFor := fs.Duration("for", 0, "stop after this long and print the summary (0: until Ctrl-C)")
	maxGap := fs.Duration("max-gap", 30*time.Second, "the summary fails the run if updates ever stopped for longer")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead watch [-lat N -lon N] [-radius km] [-feeds a,b] [-for 1h]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return usageError{err.Error()}
	}
	switch {
	case fs.NArg() > 0:
		return usageError{"overhead watch: unexpected arguments " + strings.Join(fs.Args(), " ")}
	case !center.Valid():
		return usageError{fmt.Sprintf("overhead watch: %v, %v is not a coordinate", center.Lat, center.Lon)}
	case *radius <= 0 || *radius > 250*geo.KmPerNM:
		return usageError{"overhead watch: -radius must be between 0 and 463 km (the feeds' 250 nm limit)"}
	case *interval < time.Second:
		// adsb.fi and airplanes.live allow one request a second; stay well under it.
		return usageError{"overhead watch: -interval must be at least 1s"}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	userAgent := "overhead/" + buildinfo.String() + " (+https://github.com/amirradjou/overhead)"
	var fl []feeds.Feed
	spacing := make(map[string]time.Duration)
	for name := range strings.SplitSeq(*feedList, ",") {
		name = strings.TrimSpace(name)
		if _, dup := spacing[name]; dup || name == "" {
			continue
		}
		f, err := feeds.ByName(name, client, userAgent)
		if err != nil {
			return usageError{"overhead watch: " + err.Error()}
		}
		fl = append(fl, f)
		spacing[name] = feeds.Spacing(name)
	}
	if len(fl) == 0 {
		return usageError{"overhead watch: -feeds is empty"}
	}

	if *runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *runFor)
		defer cancel()
	}
	p := &poller.Poller{Feeds: fl, Spacing: spacing, Center: center, RadiusKm: *radius, Interval: *interval}
	tty := isTerminal(os.Stdout)
	stats := p.Run(ctx, func(s poller.Snapshot) {
		if tty {
			fmt.Print(clearScreen)
			renderTable(os.Stdout, s, center, *radius)
		} else {
			renderLine(os.Stdout, s)
		}
	})
	return renderSummary(os.Stdout, stats, time.Now(), *maxGap)
}

func envCenter() (geo.Point, error) {
	c := defaultCenter
	for _, v := range []struct {
		name string
		dst  *float64
	}{{"OVERHEAD_LAT", &c.Lat}, {"OVERHEAD_LON", &c.Lon}} {
		s := strings.TrimSpace(os.Getenv(v.name))
		if s == "" {
			continue
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return c, fmt.Errorf("%s=%q is not a number", v.name, s)
		}
		*v.dst = f
	}
	return c, nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
