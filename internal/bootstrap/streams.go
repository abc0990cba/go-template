package bootstrap

// Broker clients for the enrichment streams: optional infrastructure opened
// alongside the operational store. Each side degrades independently — a
// broker that is down logs a warning and disables its consumer, exactly like
// ClickHouse.

import (
	"context"
	"log/slog"

	"github.com/your-org/go-template/internal/config"
	kafkaplatform "github.com/your-org/go-template/internal/platform/kafka"
	natsplatform "github.com/your-org/go-template/internal/platform/nats"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/streams"
)

// Streams bundles the optional broker clients; nil means that side is
// disabled. Publisher fans the outbox relay out over every enabled broker.
type Streams struct {
	Kafka     *kafkaplatform.Client
	NATS      *natsplatform.Client
	Publisher streams.Publisher
}

// Brokers opens the configured broker clients from configuration. Failures
// are logged, not fatal: the app runs without the streams.
func Brokers(ctx context.Context, cfg config.Config, logger *slog.Logger, breakers *reliability.Breakers) *Streams {
	out := &Streams{}
	var publishers []streams.Publisher

	if len(cfg.KafkaBrokers) > 0 {
		kafkaCfg := kafkaplatform.Config{
			Brokers:      cfg.KafkaBrokers,
			TopicProfile: cfg.KafkaTopicProfile,
			TopicEvents:  cfg.KafkaTopicEvents,
			Group:        cfg.KafkaGroup,
		}
		client, err := kafkaplatform.Open(ctx, kafkaCfg, logger, breakers)
		if err != nil {
			logger.Warn("kafka unavailable; profile stream disabled", "err", err)
		} else {
			out.Kafka = client
			publishers = append(publishers, client)
		}
	}
	if cfg.NatsURL != "" {
		natsCfg := natsplatform.Config{
			URL:            cfg.NatsURL,
			Stream:         cfg.NatsStream,
			SubjectContact: cfg.NatsSubjectContact,
			SubjectEvents:  cfg.NatsSubjectEvents,
			Durable:        cfg.NatsDurable,
		}
		client, err := natsplatform.Open(ctx, natsCfg, logger, breakers)
		if err != nil {
			logger.Warn("nats unavailable; contact stream disabled", "err", err)
		} else {
			out.NATS = client
			publishers = append(publishers, client)
		}
	}
	switch len(publishers) {
	case 0:
		out.Publisher = nil
	case 1:
		out.Publisher = publishers[0]
	default:
		out.Publisher = streams.MultiPublisher{Publishers: publishers}
	}
	return out
}

// Close releases every enabled broker client.
func (s *Streams) Close() {
	if s.Kafka != nil {
		s.Kafka.Close()
	}
	if s.NATS != nil {
		s.NATS.Close()
	}
}

// Probes returns readiness checks for a process that does not consume: the
// Kafka probe is ping-only, because a consumer-group client that never polls
// would hold partitions away from the real consumers in the worker.
func Probes(ctx context.Context, cfg config.Config, logger *slog.Logger) ([]func(context.Context) error, func()) {
	var (
		probes   []func(context.Context) error
		closeFns []func()
	)
	if len(cfg.KafkaBrokers) > 0 {
		probe, err := kafkaplatform.OpenProbe(ctx, cfg.KafkaBrokers)
		if err != nil {
			logger.Warn("kafka unavailable; readiness will not include it", "err", err)
		} else {
			probes = append(probes, probe.Ping)
			closeFns = append(closeFns, probe.Close)
		}
	}
	if cfg.NatsURL != "" {
		client, err := natsplatform.Open(ctx, natsplatform.Config{
			URL: cfg.NatsURL, Stream: cfg.NatsStream,
			SubjectContact: cfg.NatsSubjectContact, SubjectEvents: cfg.NatsSubjectEvents,
			Durable: cfg.NatsDurable,
		}, logger, nil)
		if err != nil {
			logger.Warn("nats unavailable; readiness will not include it", "err", err)
		} else {
			probes = append(probes, client.Ping)
			closeFns = append(closeFns, client.Close)
		}
	}
	return probes, func() {
		for _, fn := range closeFns {
			fn()
		}
	}
}
