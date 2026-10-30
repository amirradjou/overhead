// Command overhead runs the overhead poller and enrichment service (see README.md).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/amirradjou/overhead/internal/buildinfo"
)

const usage = `usage: overhead <command> [flags]

commands:
  watch     poll the feeds and show the planes round home as a live table (no broker needed)
  poll      poll the feeds and publish position events and enrichment jobs to RabbitMQ
  enrich    work the enrichment queue: type, operator and route (adsbdb), photo (planespotters)
  live      keep the live picture and serve it to the screen: /api/now, /api/events (SSE)
  history   write the positions to one-minute batch files
  verify    check the batch files for lost or duplicated events
  version   print the version

Run "overhead <command> -h" for a command's flags.

environment:
  OVERHEAD_LAT, OVERHEAD_LON   the centre of the circle (default: Toronto Pearson)
  OVERHEAD_AMQP                RabbitMQ (default amqp://overhead:overhead@localhost:5673/, compose.yaml)
  OVERHEAD_DATA                where enrich and history keep their files (default ./data)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The first Ctrl-C ends the run and prints its summary; once it has, a second one exits at once.
	go func() {
		<-ctx.Done()
		stop()
	}()

	var err error
	switch os.Args[1] {
	case "version":
		fmt.Println(buildinfo.String())
	case "watch":
		err = runWatch(ctx, os.Args[2:])
	case "poll":
		err = runPoll(ctx, os.Args[2:])
	case "enrich":
		err = runEnrich(ctx, os.Args[2:])
	case "live":
		err = runLive(ctx, os.Args[2:])
	case "history":
		err = runHistory(ctx, os.Args[2:])
	case "verify":
		err = runVerify(ctx, os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		if _, ok := errors.AsType[usageError](err); ok {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "overhead:", err)
		os.Exit(1)
	}
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }
