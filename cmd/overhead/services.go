package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/enrich"
	"github.com/amirradjou/overhead/internal/history"
	"github.com/amirradjou/overhead/internal/live"
	"github.com/amirradjou/overhead/internal/poller"
	"github.com/amirradjou/overhead/internal/relay"
)

// every calls f each d until ctx ends.
func every(ctx context.Context, d time.Duration, f func()) {
	tick := time.NewTicker(d)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			f()
		}
	}
}

func runPoll(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("poll", flag.ContinueOnError)
	settings, err := addPollFlags(fs)
	if err != nil {
		return err
	}
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead poll [-lat N -lon N] [-radius km] [-feeds a,b]   (OVERHEAD_AMQP)")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	p, err := settings.poller("poll")
	if err != nil {
		return err
	}
	pub := &bus.Publisher{URL: amqpURL(), Name: "overhead-poll"}
	defer pub.Close()
	rel := &relay.Relay{Run: relay.NewRun(time.Now()), Pub: pub}
	slog.Info("polling", "run", rel.Run, "center", fmt.Sprintf("%.4f,%.4f", settings.center.Lat, settings.center.Lon),
		"radius_km", settings.radius, "feeds", settings.feeds)

	var lastLog time.Time
	lastErr := ""
	stats := p.Run(ctx, func(s poller.Snapshot) {
		err := rel.Handle(ctx, s)
		switch {
		case err != nil && ctx.Err() == nil && err.Error() != lastErr:
			slog.Warn("publishing failed; positions lost until the broker is back", "err", err)
			lastErr = err.Error()
		case err == nil && lastErr != "":
			slog.Info("publishing again")
			lastErr = ""
		}
		if s.At.Sub(lastLog) >= time.Minute {
			r := rel.Stats()
			slog.Info("poll", "aircraft", len(s.Aircraft), "feeds", feedSummary(s.Feeds, 80),
				"published", r.Positions, "lost", r.Lost, "jobs", r.Jobs)
			lastLog = s.At
		}
	})
	r := rel.Stats()
	slog.Info("stopped", "run", rel.Run, "polls", stats.Polls, "longest_gap", stats.GapUntil(time.Now()).Round(time.Second),
		"published", r.Positions, "lost", r.Lost, "jobs", r.Jobs)
	return nil
}

func runEnrich(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("enrich", flag.ContinueOnError)
	cachePath := fs.String("cache", dataPath("enrich.db"), "cache file (bbolt)")
	adsbdbURL := fs.String("adsbdb", "https://api.adsbdb.com", "adsbdb API base URL")
	photosURL := fs.String("planespotters", "https://api.planespotters.net", "planespotters API base URL")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead enrich [-cache path]   (OVERHEAD_AMQP, OVERHEAD_DATA)")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dataPath(), 0o755); err != nil {
		return err
	}
	cache, err := enrich.OpenCache(*cachePath)
	if err != nil {
		return fmt.Errorf("open cache %s: %w", *cachePath, err)
	}
	defer cache.Close()
	if n, err := cache.Purge(); err == nil && n > 0 {
		slog.Info("purged expired cache entries", "count", n)
	}
	hc := &http.Client{Timeout: 20 * time.Second}
	worker := &enrich.Worker{
		Aircraft: enrich.NewADSBDB(*adsbdbURL, hc, userAgent()),
		Photos:   enrich.NewPlanespotters(*photosURL, hc, userAgent()),
		Cache:    cache,
	}
	pub := &bus.Publisher{URL: amqpURL(), Name: "overhead-enrich-results"}
	defer pub.Close()
	consumer := &enrich.Consumer{Worker: worker, Pub: pub}

	report := func(msg string) {
		c := worker.Counts()
		perPlane := 0.0
		if c.NewAircraft > 0 {
			perPlane = float64(c.Calls) / float64(c.NewAircraft)
		}
		slog.Info(msg, "jobs", c.Jobs, "new_aircraft", c.NewAircraft, "api_calls", c.Calls,
			"calls_per_new_plane", fmt.Sprintf("%.2f", perPlane), "cache_hits", c.CacheHits, "failed", c.Failures)
	}
	go every(ctx, 5*time.Minute, func() { report("enrich") })
	err = bus.Consume(ctx, amqpURL(), "overhead-enrich", bus.QueueEnrich, 8, consumer.Session)
	report("stopped")
	return err
}

func runLive(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("live", flag.ContinueOnError)
	c, err := addCircleFlags(fs)
	if err != nil {
		return err
	}
	addr := fs.String("addr", env("OVERHEAD_ADDR", ":8090"), "listen address (OVERHEAD_ADDR)")
	web := fs.String("web", "", "serve the screen's static files from this directory (web/out)")
	public := fs.Bool("public", false, "commercial flights only (the README privacy rule for the public demo)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead live [-addr :8090] [-web web/out] [-public]   (OVERHEAD_AMQP)")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := c.check("live"); err != nil {
		return err
	}
	// Everything below stops with ctx; a listener that fails cancels it, so the command exits with its error rather
	// than draining the queues with no one to serve.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	state := &live.State{Center: c.center, RadiusKm: c.radius, Public: *public}
	srv := &live.Server{State: state}
	if *web != "" {
		srv.Web = http.FileServer(http.Dir(*web))
	}
	pub := &bus.Publisher{URL: amqpURL(), Name: "overhead-live-asks"}
	defer pub.Close()
	consumer := &live.Consumer{State: state, Pub: pub}

	var wg sync.WaitGroup
	wg.Go(func() { srv.Run(ctx) })
	wg.Go(func() { _ = bus.Consume(ctx, amqpURL(), "overhead-live", bus.QueueLive, 500, consumer.Positions) })
	wg.Go(func() {
		_ = bus.Consume(ctx, amqpURL(), "overhead-live-aircraft", bus.QueueLiveAircraft, 100, consumer.Aircraft)
	})
	wg.Go(func() {
		every(ctx, 10*time.Minute, func() {
			cov := state.Coverage()
			slog.Info("coverage", "flights", cov.Flights, "with_type", cov.WithType, "with_route", cov.WithRoute,
				"share_with_both", fmt.Sprintf("%.1f%%", 100*cov.Share), "streams", srv.Clients(), "positions", state.Received())
		})
	})

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Streams outlive any write timeout; each write sets its own deadline. Requests end with ctx.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	slog.Info("serving", "addr", *addr, "public", *public, "web", *web)
	select {
	case err = <-errc:
		stop()
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = hs.Shutdown(shutdown)
	}
	wg.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runHistory(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	dir := fs.String("dir", dataPath("history"), "directory for the batch files")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead history [-dir data/history]   (OVERHEAD_AMQP, OVERHEAD_DATA)")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	w := &history.Writer{Store: history.DirStore{Root: *dir}}
	report := func(msg string) {
		s := w.Stats()
		slog.Info(msg, "batches", s.Batches, "events", s.Events, "failures", s.Failures, "rejected", s.Rejected)
	}
	go every(ctx, 5*time.Minute, func() { report("history") })
	slog.Info("writing history", "dir", *dir)
	// The prefetch holds well over a minute of positions, so a batch is never starved of its own messages.
	err := bus.Consume(ctx, amqpURL(), "overhead-history", bus.QueueHistory, 5000, w.Session)
	report("stopped")
	return err
}

func runVerify(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dir := fs.String("dir", dataPath("history"), "directory of batch files")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead verify [-dir data/history]")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	reports, err := history.Verify(*dir)
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		return fmt.Errorf("no history under %s", *dir)
	}
	bad := false
	for _, r := range reports {
		line := fmt.Sprintf("%s: %d events, up to seq %d", r.Run, r.Events, r.Last)
		if n := r.MissingCount(); n > 0 {
			bad = true
			var ranges []string
			for _, m := range r.Missing[:min(len(r.Missing), 10)] {
				ranges = append(ranges, m.String())
			}
			line += fmt.Sprintf("; MISSING %d: %s", n, strings.Join(ranges, ", "))
		}
		if r.Duplicates > 0 {
			bad = true
			line += fmt.Sprintf("; DUPLICATES %d", r.Duplicates)
		}
		fmt.Println(line)
	}
	if bad {
		return errors.New("history has gaps or duplicates")
	}
	fmt.Println("history complete: no missing or duplicated events")
	return nil
}
