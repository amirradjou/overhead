package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

// Defaults match compose.yaml's ports on the host.
const (
	defaultAMQP = "amqp://overhead:overhead@localhost:5673/"
	defaultData = "data"
)

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func amqpURL() string { return env("OVERHEAD_AMQP", defaultAMQP) }

func dataPath(parts ...string) string {
	return filepath.Join(append([]string{env("OVERHEAD_DATA", defaultData)}, parts...)...)
}

func userAgent() string {
	return "overhead/" + buildinfo.String() + " (+https://github.com/amirradjou/overhead)"
}

// parse parses args, turning flag errors into usage errors; it returns flag.ErrHelp for -h.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{err.Error()}
	}
	if fs.NArg() > 0 {
		return usageError{"overhead " + fs.Name() + ": unexpected arguments " + strings.Join(fs.Args(), " ")}
	}
	return nil
}

// circle is the area overhead watches.
type circle struct {
	center geo.Point
	radius float64
}

func addCircleFlags(fs *flag.FlagSet) (*circle, error) {
	c := &circle{}
	var err error
	if c.center, err = envCenter(); err != nil {
		return nil, usageError{err.Error()}
	}
	fs.Float64Var(&c.center.Lat, "lat", c.center.Lat, "latitude of the centre (OVERHEAD_LAT)")
	fs.Float64Var(&c.center.Lon, "lon", c.center.Lon, "longitude of the centre (OVERHEAD_LON)")
	// 60 km sees a plane a few minutes before it reaches the window (README: Poller).
	fs.Float64Var(&c.radius, "radius", 60, "radius in km")
	return c, nil
}

func (c *circle) check(cmd string) error {
	switch {
	case !c.center.Valid():
		return usageError{fmt.Sprintf("overhead %s: %v, %v is not a coordinate", cmd, c.center.Lat, c.center.Lon)}
	case c.radius <= 0 || c.radius > 250*geo.KmPerNM:
		return usageError{"overhead " + cmd + ": -radius must be between 0 and 463 km (the feeds' 250 nm limit)"}
	}
	return nil
}

// pollSettings are the flags of the commands that poll the feeds.
type pollSettings struct {
	*circle
	interval time.Duration
	feeds    string
}

func addPollFlags(fs *flag.FlagSet) (*pollSettings, error) {
	c, err := addCircleFlags(fs)
	if err != nil {
		return nil, err
	}
	p := &pollSettings{circle: c}
	fs.DurationVar(&p.interval, "interval", poller.DefaultInterval, "time between polls")
	fs.StringVar(&p.feeds, "feeds", "adsb.lol,adsb.fi", "comma-separated feeds: "+strings.Join(feeds.Names(), ", "))
	return p, nil
}

// poller checks the settings and builds the poller they describe.
func (p *pollSettings) poller(cmd string) (*poller.Poller, error) {
	if err := p.check(cmd); err != nil {
		return nil, err
	}
	if p.interval < time.Second {
		// adsb.fi and airplanes.live allow one request a second; stay well under it.
		return nil, usageError{"overhead " + cmd + ": -interval must be at least 1s"}
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var fl []feeds.Feed
	spacing := make(map[string]time.Duration)
	for name := range strings.SplitSeq(p.feeds, ",") {
		name = strings.TrimSpace(name)
		if _, dup := spacing[name]; dup || name == "" {
			continue
		}
		f, err := feeds.ByName(name, client, userAgent())
		if err != nil {
			return nil, usageError{"overhead " + cmd + ": " + err.Error()}
		}
		fl = append(fl, f)
		spacing[name] = feeds.Spacing(name)
	}
	if len(fl) == 0 {
		return nil, usageError{"overhead " + cmd + ": -feeds is empty"}
	}
	return &poller.Poller{Feeds: fl, Spacing: spacing, Center: p.center, RadiusKm: p.radius, Interval: p.interval}, nil
}

// envFloat reads a number from the environment, or returns fallback when the variable is unset.
func envFloat(name string, fallback float64) (float64, error) {
	s := strings.TrimSpace(os.Getenv(name))
	if s == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a number", name, s)
	}
	return f, nil
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
