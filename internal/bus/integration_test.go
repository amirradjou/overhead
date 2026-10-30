package bus_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/feeds"
	"github.com/amirradjou/overhead/internal/history"
	"github.com/amirradjou/overhead/internal/poller"
	"github.com/amirradjou/overhead/internal/relay"
)

// These tests need a RabbitMQ to themselves: OVERHEAD_TEST_AMQP names a vhost they may purge, e.g.
// amqp://overhead:overhead@localhost:5673/overhead-test (see CLAUDE.md for creating it). CI starts one.
func brokerURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("OVERHEAD_TEST_AMQP")
	if url == "" {
		t.Skip("OVERHEAD_TEST_AMQP not set")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Declare(ch); err != nil {
		t.Fatal(err)
	}
	// Declaring twice must agree with itself (queue arguments are checked by the broker).
	if err := bus.Declare(ch); err != nil {
		t.Fatalf("second Declare: %v", err)
	}
	for _, q := range []string{bus.QueueLive, bus.QueueHistory, bus.QueueAlerts, bus.QueueLiveAircraft, bus.QueueEnrich, bus.QueueEnrichRetry, bus.QueueEnrichDead} {
		if _, err := ch.QueuePurge(q, false); err != nil {
			t.Fatal(err)
		}
	}
	return url
}

func queueDepth(t *testing.T, url, queue string) int {
	t.Helper()
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	q, err := ch.QueueDeclarePassive(queue, true, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return q.Messages
}

// TestHistoryConsumerOutageLosesNothing is M1's "stopping any consumer for 10 minutes loses no positions", for the
// consumer that must keep every position: the history writer is stopped mid-minute while the poller keeps
// publishing, then started again, and the stored batches must hold every sequence number exactly once.
func TestHistoryConsumerOutageLosesNothing(t *testing.T) {
	url := brokerURL(t)
	ctx := context.Background()
	pub := &bus.Publisher{URL: url, Name: "test-poll"}
	defer pub.Close()
	rel := &relay.Relay{Run: relay.NewRun(time.Now()), Pub: pub}

	// Positions published half an hour ago, ten seconds apart, three aircraft each: every minute is long over, so
	// batches close as soon as the stream goes quiet.
	base := time.Now().Add(-30 * time.Minute).Truncate(time.Minute)
	publish := func(from, to int) {
		t.Helper()
		for i := from; i < to; i++ {
			at := base.Add(time.Duration(i) * 10 * time.Second)
			s := poller.Snapshot{At: at}
			for k := range 3 {
				s.Aircraft = append(s.Aircraft, poller.Aircraft{Position: feeds.Position{Hex: fmt.Sprintf("abc00%d", k), At: at}})
			}
			if err := rel.Handle(ctx, s); err != nil {
				t.Fatal(err)
			}
		}
	}
	dir := t.TempDir()
	run := func(ctx context.Context, w *history.Writer) *sync.WaitGroup {
		var wg sync.WaitGroup
		wg.Go(func() { _ = bus.Consume(ctx, url, "test-history", bus.QueueHistory, 5000, w.Session) })
		return &wg
	}
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// The writer runs, then stops in the middle of a minute: that minute's messages are unacknowledged.
	ctx1, stop := context.WithCancel(ctx)
	w1 := &history.Writer{Store: history.DirStore{Root: dir}, Grace: time.Millisecond, Quiet: 300 * time.Millisecond}
	wg := run(ctx1, w1)
	publish(0, 20) // 60 events, minutes 0 to 3
	waitFor("the first batches", func() bool { return w1.Stats().Events >= 36 })
	stop()
	wg.Wait()

	// The poller keeps publishing while nothing consumes the history queue.
	publish(20, 80) // 180 more events
	if n := queueDepth(t, url, bus.QueueHistory); n < 180 {
		t.Fatalf("history queue holds %d messages while its consumer is down, want at least 180", n)
	}

	w2 := &history.Writer{Store: history.DirStore{Root: dir}, Grace: time.Millisecond, Quiet: 300 * time.Millisecond}
	ctx2, stop2 := context.WithCancel(ctx)
	wg = run(ctx2, w2)
	defer func() { stop2(); wg.Wait() }()

	var reports []history.RunReport
	waitFor("all 240 events stored", func() bool {
		var err error
		reports, err = history.Verify(dir)
		return err == nil && len(reports) == 1 && reports[0].Last == 240 && reports[0].MissingCount() == 0
	})
	if r := reports[0]; r.Events != 240 || r.Duplicates != 0 {
		t.Fatalf("history = %+v, want 240 events, none missing or duplicated", r)
	}
	if n := queueDepth(t, url, bus.QueueHistory); n != 0 {
		t.Fatalf("%d messages left unacknowledged", n)
	}
	// The other durable queues kept everything too while nothing consumed them.
	if n := queueDepth(t, url, bus.QueueAlerts); n != 240 {
		t.Fatalf("alerts queue holds %d, want 240", n)
	}
}

// TestEnrichJobsWaitForTheirWorker: jobs published while no worker runs stay queued.
func TestEnrichJobsWaitForTheirWorker(t *testing.T) {
	url := brokerURL(t)
	pub := &bus.Publisher{URL: url, Name: "test-enrich"}
	defer pub.Close()
	for i := range 5 {
		body := fmt.Appendf(nil, `{"hex":"abc%03d","callsign":"ACA%d"}`, i, i)
		if err := pub.Publish(context.Background(), bus.Message{Exchange: bus.ExchangeEnrich, Key: bus.KeyJob, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	if n := queueDepth(t, url, bus.QueueEnrich); n != 5 {
		t.Fatalf("enrich queue holds %d jobs, want 5", n)
	}
}
