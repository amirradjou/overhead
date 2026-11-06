// Package bus is overhead's RabbitMQ plumbing: the exchanges and queues of the README's event flow, a publisher that
// waits for the broker's confirms, and a consumer loop that reconnects.
package bus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchanges, queues and routing keys (README: Event flow).
const (
	ExchangePositions = "positions" // topic: position events
	// fanout: what the public instance may show, one feed's own reports (relay.PublicFeed)
	ExchangePositionsPublic = "positions.public"
	ExchangeAircraft        = "aircraft"     // fanout: enrichment results
	ExchangeEnrich          = "enrich"       // direct: enrichment jobs
	ExchangeEnrichRetry     = "enrich.retry" // direct: jobs waiting out a retry delay
	ExchangeEnrichDead      = "enrich.dead"  // fanout: jobs that failed MaxAttempts times

	QueueLive         = "live"
	QueueHistory      = "history"
	QueueAlerts       = "alerts"
	QueueLiveAircraft = "live.aircraft"
	// The public instance of the live API has its own pair, so it is one more consumer: nothing it does can take
	// messages from, or slow, the home screen's. Its positions come from the public exchange, not the merged picture.
	QueueLivePublic         = "live.public"
	QueueLivePublicAircraft = "live.public.aircraft"
	QueueEnrich             = "enrich"
	QueueEnrichRetry        = "enrich.retry"
	QueueEnrichDead         = "enrich.dead"

	KeyPosition = "position"
	KeyJob      = "job"
)

// Queue limits. Changing an argument of an existing queue makes RabbitMQ refuse the declaration
// (PRECONDITION_FAILED): delete the queue first.
const (
	// The live screen wants the latest point only: a capped queue that drops its oldest messages, which expire anyway.
	liveMaxLength = 2000
	liveTTL       = time.Minute
	// alertsMaxLength bounds the alerts queue at about three days of positions while nothing consumes it.
	alertsMaxLength = 1_000_000
	// RetryDelay is how long a failed enrichment job waits before it is tried again.
	RetryDelay = 30 * time.Second
	// MaxAttempts is how many times a job is tried before it is parked in the dead-letter queue.
	MaxAttempts = 5
	// HeaderAttempts counts the failed attempts of an enrichment job.
	HeaderAttempts = "x-overhead-attempts"
)

// Declare creates the exchanges, queues and bindings, or checks that they exist as described.
func Declare(ch *amqp.Channel) error {
	exchanges := []struct{ name, kind string }{
		{ExchangePositions, amqp.ExchangeTopic},
		{ExchangePositionsPublic, amqp.ExchangeFanout},
		{ExchangeAircraft, amqp.ExchangeFanout},
		{ExchangeEnrich, amqp.ExchangeDirect},
		{ExchangeEnrichRetry, amqp.ExchangeDirect},
		{ExchangeEnrichDead, amqp.ExchangeFanout},
	}
	for _, e := range exchanges {
		if err := ch.ExchangeDeclare(e.name, e.kind, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", e.name, err)
		}
	}
	livePositions := amqp.Table{
		"x-max-length": liveMaxLength, "x-overflow": "drop-head", "x-message-ttl": liveTTL.Milliseconds(),
	}
	liveAircraft := amqp.Table{"x-max-length": 10_000, "x-overflow": "drop-head"}
	queues := []struct {
		name, exchange, key string
		args                amqp.Table
	}{
		{QueueLive, ExchangePositions, "#", livePositions},
		{QueueLivePublic, ExchangePositionsPublic, "", livePositions},
		{QueueHistory, ExchangePositions, "#", nil},
		{QueueAlerts, ExchangePositions, "#", amqp.Table{"x-max-length": alertsMaxLength, "x-overflow": "drop-head"}},
		{QueueLiveAircraft, ExchangeAircraft, "", liveAircraft},
		{QueueLivePublicAircraft, ExchangeAircraft, "", liveAircraft},
		{QueueEnrich, ExchangeEnrich, KeyJob, nil},
		// A job sits here for RetryDelay, then dead-letters back onto the enrich exchange.
		{QueueEnrichRetry, ExchangeEnrichRetry, KeyJob, amqp.Table{
			"x-message-ttl":             RetryDelay.Milliseconds(),
			"x-dead-letter-exchange":    ExchangeEnrich,
			"x-dead-letter-routing-key": KeyJob,
		}},
		{QueueEnrichDead, ExchangeEnrichDead, "", nil},
	}
	for _, q := range queues {
		if _, err := ch.QueueDeclare(q.name, true, false, false, false, q.args); err != nil {
			return fmt.Errorf("declare queue %s: %w", q.name, err)
		}
		if err := ch.QueueBind(q.name, q.key, q.exchange, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", q.name, err)
		}
	}
	return nil
}

func dial(url, name string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.DialConfig(url, amqp.Config{
		Heartbeat:  10 * time.Second,
		Properties: amqp.Table{"connection_name": name},
	})
	if err != nil {
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err == nil {
		err = Declare(ch)
	}
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, ch, nil
}

// Message is one message to publish.
type Message struct {
	Exchange, Key string
	Body          []byte
	Headers       amqp.Table
}

// Publisher publishes persistent JSON messages and waits for the broker to confirm them. It connects on first use and
// again after a failure, so a broker restart costs the messages published while it was down, not the process.
type Publisher struct {
	URL, Name string

	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Publish sends msgs and returns once the broker has confirmed all of them, or an error if it could not.
func (p *Publisher) Publish(ctx context.Context, msgs ...Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch == nil || p.ch.IsClosed() {
		p.closeLocked()
		conn, ch, err := dial(p.URL, p.Name)
		if err != nil {
			return fmt.Errorf("connect to RabbitMQ: %w", err)
		}
		if err := ch.Confirm(false); err != nil {
			conn.Close()
			return fmt.Errorf("confirm mode: %w", err)
		}
		p.conn, p.ch = conn, ch
	}
	confirms := make([]*amqp.DeferredConfirmation, 0, len(msgs))
	for _, m := range msgs {
		dc, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, m.Exchange, m.Key, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Headers:      m.Headers,
			Body:         m.Body,
		})
		if err != nil {
			p.closeLocked()
			return fmt.Errorf("publish to %s: %w", m.Exchange, err)
		}
		confirms = append(confirms, dc)
	}
	for _, dc := range confirms {
		ok, err := dc.WaitContext(ctx)
		if err != nil {
			p.closeLocked()
			return fmt.Errorf("wait for confirm: %w", err)
		}
		if !ok {
			return errors.New("broker refused a message (nack)")
		}
	}
	return nil
}

// Close closes the connection.
func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *Publisher) closeLocked() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn, p.ch = nil, nil
}

// Session handles one connection's deliveries until they stop (the connection dropped) or ctx ends. It acknowledges
// deliveries itself, on ch; tags are only valid on that channel, so anything unacknowledged is redelivered to the next
// session.
type Session func(ctx context.Context, ch *amqp.Channel, deliveries <-chan amqp.Delivery) error

// Consume runs session on queue, reconnecting with a growing delay (up to 30 s) after the broker goes away, until ctx
// ends.
func Consume(ctx context.Context, url, name, queue string, prefetch int, session Session) error {
	delay := time.Second
	for {
		start := time.Now()
		err := consumeOnce(ctx, url, name, queue, prefetch, session)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(start) > time.Minute {
			delay = time.Second // it had been working: this is a fresh failure
		}
		slog.Warn("consumer lost its connection; reconnecting", "queue", queue, "err", err, "in", delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		delay = min(2*delay, 30*time.Second)
	}
}

func consumeOnce(ctx context.Context, url, name, queue string, prefetch int, session Session) error {
	conn, ch, err := dial(url, name)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.ConsumeWithContext(ctx, queue, name, false, false, false, false, nil)
	if err != nil {
		return err
	}
	slog.Info("consuming", "queue", queue)
	return session(ctx, ch, deliveries)
}

// Attempts reads how many times a delivery has already failed.
func Attempts(h amqp.Table) int {
	switch n := h[HeaderAttempts].(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	}
	return 0
}
