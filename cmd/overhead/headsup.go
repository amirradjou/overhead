package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/headsup"
	"github.com/amirradjou/overhead/internal/history"
	"github.com/amirradjou/overhead/internal/live"
)

func runHeadsup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("headsup", flag.ContinueOnError)
	c, err := addCircleFlags(fs)
	if err != nil {
		return err
	}
	w, err := addWindowFlags(fs)
	if err != nil {
		return err
	}
	ledgerPath := fs.String("ledger", dataPath("headsup.db"), "sent heads-ups and seen types (bbolt)")
	replay := fs.String("replay", "", "replay this history directory instead of watching the alerts queue")
	day := fs.String("day", "", "with -replay: the local day to replay, YYYY-MM-DD (default: all of it)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead headsup [-replay data/history [-day 2026-10-30]]   "+
			"(OVERHEAD_AMQP, OVERHEAD_TELEGRAM_TOKEN, OVERHEAD_TELEGRAM_CHAT)")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := c.check("headsup"); err != nil {
		return err
	}
	if err := checkWindow("headsup", *w); err != nil {
		return err
	}
	state := &live.State{Center: c.center, RadiusKm: c.radius, Window: *w}

	if *replay != "" {
		from, to := time.Time{}, time.Now().Add(24*time.Hour)
		if *day != "" {
			d, err := time.ParseInLocation(time.DateOnly, *day, time.Local)
			if err != nil {
				return usageError{"overhead headsup: -day: " + err.Error()}
			}
			from, to = d, d.AddDate(0, 0, 1)
		}
		evs, err := history.Read(*replay, from, to)
		if err != nil {
			return err
		}
		for _, n := range headsup.Replay(evs, state, headsup.Rules{}, 0) {
			fmt.Printf("%s  %-9s %s\n", n.At.Local().Format(time.DateTime), n.Kind, n.Text)
		}
		return nil
	}

	if err := os.MkdirAll(dataPath(), 0o755); err != nil {
		return err
	}
	ledger, err := headsup.OpenLedger(*ledgerPath)
	if err != nil {
		return fmt.Errorf("open ledger %s: %w", *ledgerPath, err)
	}
	defer ledger.Close()
	var send headsup.Sender = headsup.LogSender{}
	if token, chat := env("OVERHEAD_TELEGRAM_TOKEN", ""), env("OVERHEAD_TELEGRAM_CHAT", ""); token != "" && chat != "" {
		send = &headsup.Telegram{Token: token, ChatID: chat, Client: &http.Client{Timeout: 15 * time.Second}}
		slog.Info("heads-ups go to Telegram", "chat", chat)
	} else {
		slog.Info("no OVERHEAD_TELEGRAM_TOKEN/CHAT: heads-ups are only logged")
	}
	watcher := &headsup.Watcher{State: state, Rules: headsup.Rules{Types: ledger}, Ledger: ledger, Send: send}
	return bus.Consume(ctx, amqpURL(), "overhead-headsup", bus.QueueAlerts, 500, watcher.Session)
}
