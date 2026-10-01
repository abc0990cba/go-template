package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// --- broker helpers ----------------------------------------------------------

// rawKafkaProducer builds a publish-only Kafka client (no consumer group)
// and makes sure the given topics exist: several brokers ship with topic
// auto-creation disabled, and the demo must work everywhere.
func rawKafkaProducer(brokers string, ensureTopics ...string) *kgo.Client {
	cl, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		fatalf("kafka client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cl.Ping(ctx); err != nil {
		fatalf("kafka unreachable at %s: %v (run `make demo-up`)", brokers, err)
	}
	adm := kadm.NewClient(cl)
	for _, topic := range ensureTopics {
		if _, err := adm.CreateTopic(ctx, 1, 1, nil, topic); err != nil &&
			!strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "ALREADY_EXISTS") {
			fatalf("create topic %s: %v", topic, err)
		}
	}
	return cl
}

func produceSync(ctx context.Context, cl *kgo.Client, rec *kgo.Record) error {
	acked := make(chan error, 1)
	cl.Produce(ctx, rec, func(_ *kgo.Record, err error) { acked <- err })
	select {
	case err := <-acked:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- kafka-profile -------------------------------------------------------------

func cmdKafkaProfile(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("kafka-profile", flag.ExitOnError)
	var (
		brokers = fs.String("brokers", envOr("KAFKA_BROKERS", "localhost:19092"), "comma-separated Kafka brokers")
		topic   = fs.String("topic", defaultProfileTopic, "Kafka topic for profile events")
		ref     = fs.String("ref", "demo-u-0001", "external_ref of the user to enrich")
		seq     = fs.Int64("seq", 1, "event sequence number (per-source, monotonic)")
		count   = fs.Int("count", 1, "how many events to publish")
		company = fs.String("company", "Acme Corp", "company field value")
		bad     = fs.Bool("bad", false, "publish a malformed (poison) message instead")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cl := rawKafkaProducer(*brokers, *topic)
	defer cl.Close()

	for i := range *count {
		payload, eventID := profileEvent(*ref, *seq+int64(i), *company, *bad)
		rec := &kgo.Record{Topic: *topic, Key: []byte(*ref), Value: payload}
		if err := produceSync(ctx, cl, rec); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		log.Info("published to kafka", "topic", *topic, "event_id", eventID, "seq", *seq+int64(i), "bad", *bad)
	}
	if *bad {
		fmt.Println("Poison message published: the worker parks it in dead letters. Check the Streams page in the app or run `devtools dlq-list`.")
		return nil
	}
	fmt.Printf("Published %d profile event(s) for %s. Open the user's page in the app: the \"Profile (Kafka)\" card now shows the fields.\n",
		*count, *ref)
	return nil
}

func profileEvent(ref string, seq int64, company string, bad bool) ([]byte, string) {
	eventID := fmt.Sprintf("devtools-profile-%s-%d-%d", ref, seq, time.Now().UnixNano())
	if bad {
		// Truncated JSON: decode fails, the consumer parks the message.
		return []byte(`{"event_id":"` + eventID + `","external_ref":"` + ref + `","seq":` + itoa(seq) + `,"data":{"company"`), eventID
	}
	env := streamEnvelope{
		EventID: eventID, ExternalRef: ref, Seq: seq, OccurredAt: time.Now().UTC(),
		Data: json.RawMessage(fmt.Sprintf(`{"company":%q,"job_title":"Engineer","enriched_by":"devtools"}`, company)),
	}
	out, _ := json.Marshal(env)
	return out, eventID
}

// --- consume-user-events ------------------------------------------------------------

func cmdConsumeUserEvents(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("consume-user-events", flag.ExitOnError)
	var (
		brokers = fs.String("brokers", envOr("KAFKA_BROKERS", "localhost:19092"), "comma-separated Kafka brokers")
		topic   = fs.String("topic", defaultEventsTopic, "events topic the outbox relay publishes to")
		timeout = fs.Duration("timeout", 15*time.Second, "how long to listen")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	// A fresh consumer group starting at the beginning of retention: this
	// prints every event the relay has published recently. The topic may not
	// exist yet if nothing was relayed — create it so the read does not spin.
	group := fmt.Sprintf("devtools-events-%d", time.Now().UnixNano())
	cl := rawKafkaProducer(*brokers, *topic)
	defer cl.Close()
	_ = group
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(*brokers, ",")...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(*topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return fmt.Errorf("kafka client: %w", err)
	}
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	fmt.Printf("Listening on %s for %s (Ctrl+C to stop earlier)...\n", *topic, *timeout)
	seen := 0
	for {
		fetches := consumer.PollRecords(ctx, 50)
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, ferr := range errs {
				if !errors.Is(ferr.Err, context.DeadlineExceeded) {
					log.Warn("fetch error", "err", ferr.Err)
				}
			}
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			seen++
			fmt.Printf("  event: key=%s payload=%s\n", string(rec.Key), string(rec.Value))
		})
		if ctx.Err() != nil {
			break
		}
	}
	if seen == 0 {
		fmt.Println("No relayed events seen. Trigger one first: make demo-enrichment, then rerun this within retention.")
		return nil
	}
	fmt.Printf("Saw %d relayed event(s): the transactional outbox relay is publishing after commit.\n", seen)
	return nil
}
