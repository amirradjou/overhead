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
  watch     poll the feeds and show the planes round home as a live table
  version   print the version

Run "overhead <command> -h" for a command's flags.

environment:
  OVERHEAD_LAT, OVERHEAD_LON   the centre of the circle (default: Toronto Pearson)
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
