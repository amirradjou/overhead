package headsup

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/events"
	"github.com/amirradjou/overhead/internal/live"
)

// DefaultEvery is how often the picture is checked: the live screen's pace.
const DefaultEvery = 2 * time.Second

// Watcher keeps its own picture from the alerts queue, checks it against the rules and sends each new notice.
type Watcher struct {
	State  *live.State
	Rules  Rules
	Ledger Deduper
	Send   Sender
	Every  time.Duration

	// retryAt holds back notices whose send failed, so a phone service that is down is not asked every tick.
	retryAt map[string]time.Time
}

// retryAfter is how long a notice that failed to send waits before it is tried again.
const retryAfter = time.Minute

// Tick checks the picture at now and sends the notices not sent before; it returns those it sent.
func (w *Watcher) Tick(ctx context.Context, now time.Time) []Notice {
	var sent []Notice
	for _, n := range w.Rules.Check(w.State.View(now)) {
		if now.Before(w.retryAt[n.Key()]) {
			continue
		}
		first, err := w.Ledger.First(n)
		if err != nil {
			slog.Error("heads-up: ledger", "err", err)
			continue
		}
		if !first {
			continue
		}
		if err := w.Send.Send(ctx, n.Text); err != nil {
			// Not sent, so not recorded: an emergency must not be lost to one timeout. Tried again in a minute.
			slog.Warn("heads-up: sending failed; will retry", "kind", n.Kind, "hex", n.Hex, "err", err)
			if err := w.Ledger.Forget(n); err != nil {
				slog.Error("heads-up: ledger", "err", err)
			}
			if w.retryAt == nil {
				w.retryAt = make(map[string]time.Time)
			}
			w.retryAt[n.Key()] = now.Add(retryAfter)
			continue
		}
		delete(w.retryAt, n.Key())
		sent = append(sent, n)
	}
	return sent
}

// Session is a bus.Session for the alerts queue.
func (w *Watcher) Session(ctx context.Context, _ *amqp.Channel, deliveries <-chan amqp.Delivery) error {
	every := w.Every
	if every <= 0 {
		every = DefaultEvery
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("heads-up: deliveries stopped")
			}
			var ev events.Position
			if err := json.Unmarshal(d.Body, &ev); err != nil {
				_ = d.Reject(false)
				continue
			}
			w.State.Update(ev)
			_ = d.Ack(false)
		case now := <-tick.C:
			w.Tick(ctx, now)
		}
	}
}

// Replay runs stored positions through a fresh picture on a simulated clock, checking every `every` as the live
// watcher would, and returns the notices it would have sent (M5's "Done when").
func Replay(evs []events.Position, state *live.State, rules Rules, every time.Duration) []Notice {
	if len(evs) == 0 {
		return nil
	}
	if every <= 0 {
		every = DefaultEvery
	}
	var sent []Notice
	w := &Watcher{State: state, Rules: rules, Ledger: &MemLedger{}, Send: discard{}}
	clock := evs[0].Published
	for _, ev := range evs {
		for !clock.Add(every).After(ev.Published) {
			clock = clock.Add(every)
			sent = append(sent, w.Tick(context.Background(), clock)...)
		}
		state.Update(ev)
	}
	return append(sent, w.Tick(context.Background(), clock.Add(every))...)
}

type discard struct{}

func (discard) Send(context.Context, string) error { return nil }

// ensure Watcher.Session fits the bus.
var _ bus.Session = (*Watcher)(nil).Session
