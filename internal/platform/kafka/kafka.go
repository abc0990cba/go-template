// Package kafka implements the Kafka side of the enrichment streams on
// franz-go: a consumer-group subscriber that commits offsets only after the
// apply transaction wins (at-least-once delivery, exactly-once application),
// and a breaker-guarded producer for the outbox relay.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/streams"
)

// Config carries the client wiring from config.Config.
type Config struct {
	Brokers      []string
	TopicProfile string // consumed: user profile enrichment
	TopicEvents  string // produced: relayed domain events
	Group        string
}

// Client wraps one franz-go client for both consumption and production.
type Client struct {
	cl       *kgo.Client
	log      *slog.Logger
	events   string
	breakers *reliability.Breakers
}

var _ streams.Publisher = (*Client)(nil)

// Open connects and verifies the cluster is reachable. The consumer group
// and topic are created implicitly by the broker configuration.
func Open(ctx context.Context, cfg Config, logger *slog.Logger, breakers *reliability.Breakers) (*Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka: no brokers configured")
	}
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.Group),
		kgo.ConsumeTopics(cfg.TopicProfile),
		// Offsets move only when the apply transaction committed, so a
		// crash anywhere replays instead of losing events.
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := cl.Ping(pingCtx); err != nil {
		cl.Close()
		return nil, fmt.Errorf("kafka ping: %w", err)
	}
	if err := ensureTopics(ctx, cl, cfg.TopicProfile, cfg.TopicEvents); err != nil {
		cl.Close()
		return nil, err
	}
	logger.Info("kafka connected", "brokers", cfg.Brokers, "group", cfg.Group, "topic", cfg.TopicProfile)
	return &Client{cl: cl, log: logger, events: cfg.TopicEvents, breakers: breakers}, nil
}

// ensureTopics creates the consumed and produced topics when missing, the
// Kafka twin of the NATS stream bootstrap: several brokers ship with topic
// auto-creation disabled, and a fresh environment must work on first start.
// Existing topics are left untouched.
func ensureTopics(ctx context.Context, cl *kgo.Client, topics ...string) error {
	adm := kadm.NewClient(cl)
	for _, topic := range topics {
		if topic == "" {
			continue
		}
		if _, err := adm.CreateTopic(ctx, 1, 1, nil, topic); err != nil &&
			!strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "ALREADY_EXISTS") {
			return fmt.Errorf("kafka ensure topic %s: %w", topic, err)
		}
	}
	return nil
}

// Ping reports broker reachability for the health endpoints.
func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.cl.Ping(pingCtx)
}

// Close leaves the group and releases connections; uncommitted offsets
// redeliver elsewhere, which is exactly the at-least-once contract.
func (c *Client) Close() {
	c.cl.Close()
}

// MessageHandler processes one message. Terminal errors (already
// dead-lettered) let the loop commit past the message; anything else stops
// the batch uncommitted so the broker redelivers.
type MessageHandler func(ctx context.Context, topic string, payload []byte, headers map[string]string) error

// Consume polls, applies, and commits in order until ctx is cancelled. A
// transient failure blocks its partition briefly — natural backpressure
// while the database is unhealthy — and the uncommitted records redeliver.
func (c *Client) Consume(ctx context.Context, handle MessageHandler) error {
	const maxPoll = 100
	backoff := 500 * time.Millisecond
	for {
		fetches := c.cl.PollRecords(ctx, maxPoll)
		fetches.EachError(func(topic string, partition int32, err error) {
			if ctx.Err() == nil {
				c.log.Error("kafka fetch error", "topic", topic, "partition", partition, "err", err)
			}
		})
		var (
			commit  []*kgo.Record
			blocked bool
		)
		fetches.EachRecord(func(rec *kgo.Record) {
			if blocked {
				return // keep partition order: retry from the failing record
			}
			err := c.handle(ctx, rec, handle)
			switch {
			case err == nil:
				commit = append(commit, rec)
			case streams.IsTerminal(err):
				// Already parked in the dead-letter queue; committing is
				// what stops the poison message from looping forever.
				c.log.Warn("kafka message parked", "topic", rec.Topic, "key", string(rec.Key), "err", err)
				commit = append(commit, rec)
			default:
				blocked = true
			}
		})
		c.cl.AllowRebalance()
		if len(commit) > 0 {
			if err := c.cl.CommitRecords(ctx, commit...); err != nil && ctx.Err() == nil {
				c.log.Error("kafka commit failed", "err", err)
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if blocked {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 10*time.Second)
			continue
		}
		backoff = 500 * time.Millisecond
	}
}

func (c *Client) handle(ctx context.Context, rec *kgo.Record, handle MessageHandler) error {
	headers := headersToMap(rec.Headers)
	hctx, span := observ.SpanFromHeaders(ctx, headers, "kafka.consume "+rec.Topic)
	defer span.End()
	return handle(hctx, rec.Topic, rec.Value, headers)
}

// PublishUserEvent relays one outbox event to the events topic, keyed by the
// aggregate so per-entity ordering holds downstream.
func (c *Client) PublishUserEvent(ctx context.Context, event streams.OutboxEvent) error {
	return c.Publish(ctx, c.events, event.AggregateID, []byte(event.Payload), event.Headers)
}

// Publish sends one record behind the kafka.produce circuit breaker; trace
// context rides in the headers.
func (c *Client) Publish(ctx context.Context, topic, key string, payload []byte, headers map[string]string) error {
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: payload}
	for k, v := range headers {
		rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	observ.InjectHeaders(ctx, headers)
	return c.breakers.Do(ctx, "kafka.produce", func() error {
		return c.produce(ctx, rec)
	})
}

// produce waits for the broker's ack (or ctx cancellation).
func (c *Client) produce(ctx context.Context, rec *kgo.Record) error {
	acked := make(chan error, 1)
	c.cl.Produce(ctx, rec, func(_ *kgo.Record, err error) { acked <- err })
	select {
	case err := <-acked:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func headersToMap(headers []kgo.RecordHeader) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for _, h := range headers {
		out[h.Key] = string(h.Value)
	}
	return out
}

// Probe is a ping-only Kafka client: it never joins a consumer group, so a
// readiness probe cannot steal partitions from the real consumers (a group
// member that does not poll blocks the whole group).
type Probe struct {
	cl *kgo.Client
}

// OpenProbe connects and verifies the cluster answers, without touching any
// consumer group.
func OpenProbe(ctx context.Context, brokers []string) (*Probe, error) {
	if len(brokers) == 0 {
		return nil, errors.New("kafka: no brokers configured")
	}
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := cl.Ping(pingCtx); err != nil {
		cl.Close()
		return nil, fmt.Errorf("kafka ping: %w", err)
	}
	return &Probe{cl: cl}, nil
}

// Ping reports broker reachability for the health endpoints.
func (p *Probe) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.cl.Ping(pingCtx)
}

// Close releases the client.
func (p *Probe) Close() { p.cl.Close() }
