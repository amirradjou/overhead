package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/amirradjou/overhead/internal/poller"
)

func runWatch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	settings, err := addPollFlags(fs)
	if err != nil {
		return err
	}
	runFor := fs.Duration("for", 0, "stop after this long and print the summary (0: until Ctrl-C)")
	maxGap := fs.Duration("max-gap", 30*time.Second, "the summary fails the run if updates ever stopped for longer")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: overhead watch [-lat N -lon N] [-radius km] [-feeds a,b] [-for 1h]")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	p, err := settings.poller("watch")
	if err != nil {
		return err
	}

	if *runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *runFor)
		defer cancel()
	}
	tty := isTerminal(os.Stdout)
	stats := p.Run(ctx, func(s poller.Snapshot) {
		if tty {
			fmt.Print(clearScreen)
			renderTable(os.Stdout, s, settings.center, settings.radius)
		} else {
			renderLine(os.Stdout, s)
		}
	})
	return renderSummary(os.Stdout, stats, time.Now(), *maxGap)
}
