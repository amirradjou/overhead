package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/events"
)

// Publisher is the part of bus.Publisher the consumer needs.
type Publisher interface {
	Publish(ctx context.Context, msgs ...bus.Message) error
}

// Consumer works the enrich queue: each job's result goes out on the aircraft exchange; a job that failed for a
// reason that may pass goes to the retry queue (back after bus.RetryDelay), and after bus.MaxAttempts tries to the
// dead-letter queue.
type Consumer struct {
	Worker *Worker
	Pub    Publisher
}

// Session is a bus.Session for the enrich queue.
func (c *Consumer) Session(ctx context.Context, _ *amqp.Channel, deliveries <-chan amqp.Delivery) error {
	for {
		var d amqp.Delivery
		select {
		case <-ctx.Done():
			return nil
		case dd, ok := <-deliveries:
			if !ok {
				return errors.New("enrich: deliveries stopped")
			}
			d = dd
		}
		var job events.EnrichJob
		if err := json.Unmarshal(d.Body, &job); err != nil || job.Hex == "" {
			slog.Error("enrich: dropping a malformed job", "err", err)
			_ = d.Reject(false)
			continue
		}
		result, err := c.Worker.Enrich(ctx, job)
		if ctx.Err() != nil {
			return nil // unacknowledged: the job is redelivered to the next run
		}
		msgs, err := c.messages(d, job, result, err)
		if err != nil {
			_ = d.Reject(false)
			continue
		}
		if err := c.Pub.Publish(ctx, msgs...); err != nil {
			_ = d.Nack(false, true)
			return err
		}
		_ = d.Ack(false)
	}
}

func (c *Consumer) messages(d amqp.Delivery, job events.EnrichJob, result events.Aircraft, lookupErr error) ([]bus.Message, error) {
	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	msgs := []bus.Message{{Exchange: bus.ExchangeAircraft, Body: body}}
	if lookupErr == nil {
		return msgs, nil
	}
	attempts := bus.Attempts(d.Headers) + 1
	headers := amqp.Table{bus.HeaderAttempts: int32(attempts), "x-overhead-error": lookupErr.Error()}
	if Retryable(lookupErr) && attempts < bus.MaxAttempts {
		slog.Info("enrich: will retry", "hex", job.Hex, "attempt", attempts, "err", lookupErr)
		return append(msgs, bus.Message{Exchange: bus.ExchangeEnrichRetry, Key: bus.KeyJob, Body: d.Body, Headers: headers}), nil
	}
	slog.Warn("enrich: giving up; job parked in the dead-letter queue", "hex", job.Hex, "attempts", attempts, "err", lookupErr)
	return append(msgs, bus.Message{Exchange: bus.ExchangeEnrichDead, Body: d.Body, Headers: headers}), nil
}
