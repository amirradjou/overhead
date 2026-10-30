package history

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/amirradjou/overhead/internal/events"
)

// Writer consumes the history queue: it stores each minute's batch and only then acknowledges its messages, so a
// crash or a store outage costs nothing (at-least-once), and a redelivered batch overwrites its own file.
type Writer struct {
	Store        Store
	Grace, Quiet time.Duration
	Now          func() time.Time

	mu    sync.Mutex
	stats WriterStats
}

// WriterStats counts what the writer stored.
type WriterStats struct {
	Batches, Events, Failures, Rejected uint64
	LastError                           string
}

// Stats returns the counts so far.
func (w *Writer) Stats() WriterStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stats
}

func (w *Writer) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Session is a bus.Session for the history queue.
func (w *Writer) Session(ctx context.Context, ch *amqp.Channel, deliveries <-chan amqp.Delivery) error {
	b := &Batcher{Grace: w.Grace, Quiet: w.Quiet}
	var retry []*Batch
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		var closed []*Batch
		select {
		case <-ctx.Done():
			return nil // open batches stay unacknowledged and come back to the next session
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("history: deliveries stopped")
			}
			var ev events.Position
			if err := json.Unmarshal(d.Body, &ev); err != nil || !events.ValidRun(ev.Run) || ev.Published.IsZero() {
				slog.Error("history: dropping a malformed message", "err", err, "body", string(d.Body[:min(len(d.Body), 120)]))
				w.count(func(s *WriterStats) { s.Rejected++ })
				_ = d.Reject(false)
				continue
			}
			closed = b.Add(d.DeliveryTag, ev, d.Body, w.now())
		case <-tick.C:
			closed = append(b.Due(w.now()), retry...)
			retry = nil
		}
		for _, bt := range closed {
			err := w.store(ctx, ch, bt)
			b.Touch(w.now())
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				slog.Error("history: storing a batch failed; will retry", "key", bt.Key(), "err", err)
				w.count(func(s *WriterStats) { s.Failures++; s.LastError = err.Error() })
				retry = append(retry, bt)
			}
		}
	}
}

func (w *Writer) store(ctx context.Context, ch *amqp.Channel, bt *Batch) error {
	body, err := bt.Encode()
	if err != nil {
		return err
	}
	if err := w.Store.Put(ctx, bt.Key(), body); err != nil {
		return err
	}
	for _, tag := range bt.Tags {
		if err := ch.Ack(tag, false); err != nil {
			// Acks fail only when the channel has closed, and then every unacknowledged message comes back. If some
			// of this batch's acks went through, the rest regroup under a later first sequence number and a second
			// file holds them again: readers count each run and sequence number once (verify reports it).
			return err
		}
	}
	w.count(func(s *WriterStats) { s.Batches++; s.Events += uint64(len(bt.Tags)) })
	return nil
}

func (w *Writer) count(f func(*WriterStats)) {
	w.mu.Lock()
	f(&w.stats)
	w.mu.Unlock()
}
