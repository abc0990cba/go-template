package bootstrap

// DemoEnricher is the manual-testing hook behind the admin console's "send
// test event" buttons. It publishes real enrichment events into the input
// topics with producer-only clients — no consumer groups, no state — so the
// worker's stream pipeline can be exercised end to end with one click.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/service"
)

// demoEnvelope mirrors the enrichment wire format the streams decode.
type demoEnvelope struct {
	EventID     string    `json:"event_id"`
	ExternalRef string    `json:"external_ref"`
	Seq         int64     `json:"seq"`
	OccurredAt  time.Time `json:"occurred_at"`
	Data        any       `json:"data"`
}

// DemoEnricher publishes test enrichment events for one user. Zero value is
// not usable; a nil *DemoEnricher means the feature is off (brokers not
// configured or unreachable), which the HTTP layer reports as 503.
type DemoEnricher struct {
	kafka          *kgo.Client
	profileTopic   string
	nc             *nats.Conn
	js             nats.JetStreamContext
	contactSubject string
}

var _ service.EnrichDemoPublisher = (*DemoEnricher)(nil)

// OpenDemoEnricher connects producer-only clients and ensures the topics and
// the JetStream stream exist. It returns nil (with a warning) when neither
// broker is configured or either fails — the demo buttons then stay off.
func OpenDemoEnricher(ctx context.Context, cfg config.Config, logger *slog.Logger) *DemoEnricher {
	d := &DemoEnricher{profileTopic: cfg.KafkaTopicProfile, contactSubject: cfg.NatsSubjectContact}
	if len(cfg.KafkaBrokers) > 0 {
		cl, err := kgo.NewClient(kgo.SeedBrokers(cfg.KafkaBrokers...))
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			err = cl.Ping(pingCtx)
			if err == nil {
				if err = ensureKafkaTopics(ctx, cl, cfg.KafkaTopicProfile); err != nil {
					cl.Close()
				} else {
					d.kafka = cl
				}
			} else {
				cl.Close()
			}
		}
		if err != nil {
			logger.Warn("kafka unavailable; the demo enrich buttons stay off", "err", err)
		}
	}
	if cfg.NatsURL != "" {
		nc, err := nats.Connect(cfg.NatsURL, nats.Timeout(5*time.Second))
		if err == nil {
			js, jsErr := nc.JetStream()
			if jsErr == nil {
				if _, infoErr := js.StreamInfo(cfg.NatsStream); infoErr != nil {
					if _, addErr := js.AddStream(&nats.StreamConfig{
						Name: cfg.NatsStream, Subjects: []string{cfg.NatsSubjectContact, cfg.NatsSubjectEvents},
					}); addErr != nil {
						jsErr = addErr
					}
				}
				if jsErr == nil {
					d.nc, d.js = nc, js
				}
			}
			if d.js == nil {
				nc.Close()
				err = jsErr
			}
		}
		if err != nil {
			logger.Warn("nats unavailable; the demo enrich buttons stay off", "err", err)
		}
	}
	if d.kafka == nil && d.js == nil {
		return nil
	}
	return d
}

// Close releases the underlying clients.
func (d *DemoEnricher) Close() {
	if d == nil {
		return
	}
	if d.kafka != nil {
		d.kafka.Close()
	}
	if d.nc != nil {
		_ = d.nc.Drain()
	}
}

// PublishProfile sends one Kafka profile event carrying the next sequence
// number; it returns the event id.
func (d *DemoEnricher) PublishProfile(ctx context.Context, externalRef string, seq int64) (string, error) {
	if d.kafka == nil {
		return "", fmt.Errorf("kafka demo publisher is not configured")
	}
	eventID := fmt.Sprintf("ui-demo-profile-%s-%d", externalRef, time.Now().UnixNano())
	raw, err := json.Marshal(demoEnvelope{
		EventID: eventID, ExternalRef: externalRef, Seq: seq, OccurredAt: time.Now().UTC(),
		Data: map[string]any{
			"company":     fmt.Sprintf("UI Demo %d", time.Now().UnixMilli()%100000),
			"job_title":   "Demo Engineer",
			"enriched_by": "admin-ui",
		},
	})
	if err != nil {
		return "", err
	}
	rec := &kgo.Record{Topic: d.profileTopic, Key: []byte(externalRef), Value: raw}
	acked := make(chan error, 1)
	d.kafka.Produce(ctx, rec, func(_ *kgo.Record, err error) { acked <- err })
	select {
	case err := <-acked:
		return eventID, err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// PublishContact sends one NATS contact event with the next sequence number
// and a Nats-Msg-Id for server-side deduplication; it returns the event id.
func (d *DemoEnricher) PublishContact(ctx context.Context, externalRef string, seq int64) (string, error) {
	if d.js == nil {
		return "", fmt.Errorf("nats demo publisher is not configured")
	}
	eventID := fmt.Sprintf("ui-demo-contact-%s-%d", externalRef, time.Now().UnixNano())
	raw, err := json.Marshal(demoEnvelope{
		EventID: eventID, ExternalRef: externalRef, Seq: seq, OccurredAt: time.Now().UTC(),
		Data: map[string]any{
			"phone":             fmt.Sprintf("+1 555 01%02d", time.Now().UnixMilli()%100),
			"preferred_channel": "email",
			"enriched_by":       "admin-ui",
		},
	})
	if err != nil {
		return "", err
	}
	msg := nats.NewMsg(d.contactSubject)
	msg.Data = raw
	msg.Header.Set("Nats-Msg-Id", eventID)
	_, err = d.js.PublishMsg(msg, nats.Context(ctx))
	if err != nil && strings.Contains(err.Error(), "context") {
		return "", ctx.Err()
	}
	return eventID, err
}

// ensureKafkaTopics creates missing topics; brokers with auto-creation
// disabled would otherwise reject the demo publishes.
func ensureKafkaTopics(ctx context.Context, cl *kgo.Client, topics ...string) error {
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
