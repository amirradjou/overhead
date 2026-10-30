package live

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/amirradjou/overhead/internal/bus"
	"github.com/amirradjou/overhead/internal/events"
)

// Publisher is the part of bus.Publisher the live API needs.
type Publisher interface {
	Publish(ctx context.Context, msgs ...bus.Message) error
}

// askAgain bounds how often the live API asks enrichment about one aircraft it has nothing on.
const askAgain = 10 * time.Minute

// Consumer feeds the State from the live and live.aircraft queues. When a position arrives for an aircraft the state
// knows nothing about (the API restarted and lost its memory), it asks enrichment again; the worker answers that from
// its cache.
type Consumer struct {
	State *State
	Pub   Publisher

	mu    sync.Mutex
	asked map[string]time.Time
}

// Positions is a bus.Session for the live queue.
func (c *Consumer) Positions(ctx context.Context, _ *amqp.Channel, deliveries <-chan amqp.Delivery) error {
	return each(ctx, deliveries, func(body []byte) error {
		var ev events.Position
		if err := json.Unmarshal(body, &ev); err != nil {
			return err
		}
		c.State.Update(ev)
		c.ask(ctx, ev)
		return nil
	})
}

// Aircraft is a bus.Session for the live.aircraft queue.
func (c *Consumer) Aircraft(ctx context.Context, _ *amqp.Channel, deliveries <-chan amqp.Delivery) error {
	return each(ctx, deliveries, func(body []byte) error {
		var a events.Aircraft
		if err := json.Unmarshal(body, &a); err != nil {
			return err
		}
		c.State.SetAircraft(a)
		return nil
	})
}

func (c *Consumer) ask(ctx context.Context, ev events.Position) {
	if c.Pub == nil || len(ev.Hex) == 0 || ev.Hex[0] == '~' || c.State.HasInfo(ev.Hex) {
		return
	}
	c.mu.Lock()
	if c.asked == nil {
		c.asked = make(map[string]time.Time)
	}
	if time.Since(c.asked[ev.Hex]) < askAgain {
		c.mu.Unlock()
		return
	}
	c.asked[ev.Hex] = time.Now()
	c.mu.Unlock()
	body, _ := json.Marshal(events.EnrichJob{Hex: ev.Hex, Callsign: ev.Callsign})
	if err := c.Pub.Publish(ctx, bus.Message{Exchange: bus.ExchangeEnrich, Key: bus.KeyJob, Body: body}); err != nil {
		slog.Warn("live: asking enrichment failed", "hex", ev.Hex, "err", err)
	}
}

// each handles deliveries one by one and acknowledges them; a message that can't be read is dropped.
func each(ctx context.Context, deliveries <-chan amqp.Delivery, handle func([]byte) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("live: deliveries stopped")
			}
			if err := handle(d.Body); err != nil {
				slog.Error("live: dropping a malformed message", "err", err)
				_ = d.Reject(false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}
