// Package nats implements the NATS JetStream side of the enrichment streams:
// a durable pull consumer that acks only after the apply transaction wins,
// and a breaker-guarded publisher with server-side message-id deduplication
// for the outbox relay.
package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/service"
)

// Config carries the client wiring from config.Config.
type Config struct {
	URL            string
	Stream         string
	SubjectContact string // consumed: contact enrichment
	SubjectEvents  string // produced: relayed domain events
	Durable        string
}

// Client wraps the connection and its JetStream context.
type Client struct {
	conn     *nats.Conn
	js       nats.JetStreamContext
	cfg      Config
	log      *slog.Logger
	breakers *reliability.Breakers
}

var _ service.EventPublisher = (*Client)(nil)

// Open connects (reconnecting forever with jitter afterwards), ensures the
// stream and the durable consumer exist, and verifies JetStream answers.
func Open(_ context.Context, cfg Config, logger *slog.Logger, breakers *reliability.Breakers) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("nats: no url configured")
	}
	// nats.Connect owns its dial loop; the fixed Timeout bounds the initial
	// handshake and MaxReconnects(-1) keeps retrying afterwards.
	conn, err := nats.Connect(cfg.URL,
		nats.Name("go-template"),
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.ReconnectJitter(1*time.Second, 5*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("nats disconnected; reconnecting", "err", err)
			}
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Info("nats reconnected", "url", c.ConnectedUrl())
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := conn.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}
	if err := ensureTopology(js, cfg); err != nil {
		conn.Close()
		return nil, err
	}
	logger.Info("nats connected", "url", cfg.URL, "stream", cfg.Stream, "subject", cfg.SubjectContact, "durable", cfg.Durable)
	return &Client{conn: conn, js: js, cfg: cfg, log: logger, breakers: breakers}, nil
}

// ensureTopology idempotently creates the stream and the durable pull
// consumer so a fresh environment works on first start.
func ensureTopology(js nats.JetStreamContext, cfg Config) error {
	subjects := []string{cfg.SubjectContact, cfg.SubjectEvents}
	if _, err := js.StreamInfo(cfg.Stream); err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			return fmt.Errorf("nats stream info: %w", err)
		}
		if _, err := js.AddStream(&nats.StreamConfig{
			Name:     cfg.Stream,
			Subjects: subjects,
			Storage:  nats.FileStorage,
		}); err != nil {
			return fmt.Errorf("nats add stream: %w", err)
		}
	}
	if _, err := js.ConsumerInfo(cfg.Stream, cfg.Durable); err != nil {
		if !errors.Is(err, nats.ErrConsumerNotFound) {
			return fmt.Errorf("nats consumer info: %w", err)
		}
		if _, err := js.AddConsumer(cfg.Stream, &nats.ConsumerConfig{
			Durable:       cfg.Durable,
			FilterSubject: cfg.SubjectContact,
			AckPolicy:     nats.AckExplicitPolicy,
			AckWait:       30 * time.Second,
			MaxDeliver:    -1, // unlimited: poison messages are parked + acked by the handler path
		}); err != nil && !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
			return fmt.Errorf("nats add consumer: %w", err)
		}
	}
	return nil
}

// Ping reports JetStream readiness for the health endpoints.
func (c *Client) Ping(ctx context.Context) error {
	if c.conn.Status() != nats.CONNECTED {
		return errors.New("nats: not connected")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := c.js.AccountInfo(nats.Context(pingCtx))
	return err
}

// Close drains subscriptions and closes the connection.
func (c *Client) Close() {
	_ = c.conn.Drain()
}

// MessageHandler processes one message; terminal errors (already
// dead-lettered) are acked, transient ones are nacked for redelivery.
type MessageHandler func(ctx context.Context, subject string, payload []byte, headers map[string]string) error

// Consume fetches ordered batches from the durable consumer until ctx is
// cancelled. A transient failure nacks the rest of the batch with a small
// delay so ordering resumes without waiting out the ack wait.
func (c *Client) Consume(ctx context.Context, handle MessageHandler) error {
	sub, err := c.js.PullSubscribe(c.cfg.SubjectContact, c.cfg.Durable, nats.BindStream(c.cfg.Stream))
	if err != nil {
		return fmt.Errorf("nats pull subscribe: %w", err)
	}
	const batch = 50
	backoff := 500 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return nil
		}
		msgs, err := sub.Fetch(batch, nats.MaxWait(2*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			c.log.Error("nats fetch failed; backing off", "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 10*time.Second)
			continue
		}
		backoff = 500 * time.Millisecond
		nakRest := false
		for _, msg := range msgs {
			if nakRest {
				_ = msg.NakWithDelay(backoff)
				continue
			}
			switch c.handle(ctx, msg, handle) {
			case outcomeDone, outcomeParked:
				_ = msg.Ack()
			case outcomeTransient:
				_ = msg.NakWithDelay(backoff)
				nakRest = true
			}
		}
	}
}

type outcome int

const (
	outcomeDone outcome = iota
	outcomeParked
	outcomeTransient
)

func (c *Client) handle(ctx context.Context, msg *nats.Msg, handle MessageHandler) outcome {
	headers := headersToMap(msg.Header)
	hctx, span := observ.SpanFromHeaders(ctx, headers, "nats.consume "+msg.Subject)
	defer span.End()
	err := handle(hctx, msg.Subject, msg.Data, headers)
	switch {
	case err == nil:
		return outcomeDone
	case service.IsTerminalStreamError(err):
		// Already parked in the dead-letter queue; acking stops the loop.
		c.log.Warn("nats message parked", "subject", msg.Subject, "err", err)
		return outcomeParked
	default:
		return outcomeTransient
	}
}

// PublishUserEvent relays one outbox event. Nats-Msg-Id gives the stream
// server-side deduplication, so a relay crash between publish and mark
// cannot double-publish.
func (c *Client) PublishUserEvent(ctx context.Context, event model.OutboxEvent) error {
	msg := nats.NewMsg(c.cfg.SubjectEvents)
	msg.Data = []byte(event.Payload)
	for k, v := range event.Headers {
		msg.Header.Set(k, v)
	}
	observ.InjectHeaders(ctx, event.Headers)
	msg.Header.Set("Nats-Msg-Id", event.ID)
	return c.breakers.Do(ctx, "nats.publish", func() error {
		_, err := c.js.PublishMsg(msg)
		return err
	})
}

func headersToMap(header nats.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := make(map[string]string, len(header))
	for k, v := range header {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}
