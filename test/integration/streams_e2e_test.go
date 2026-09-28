//go:build integration

// The broker end-to-end: real Kafka and NATS containers, the real adapters,
// the real consumers, and the real relay. Publish into both streams, watch
// the user row merge exactly once, park a poison message, replay it, and
// read back what the outbox relay published.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/your-org/go-template/internal/model"
	kafkaplatform "github.com/your-org/go-template/internal/platform/kafka"
	natsplatform "github.com/your-org/go-template/internal/platform/nats"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

const (
	e2eProfileTopic = "user-profile.v1"
	e2eEventsTopic  = "user-events.v1"
	e2eContactSub   = "users.contact.v1"
	e2eStream       = "USERS"
)

type streamEnvelope struct {
	EventID     string          `json:"event_id"`
	ExternalRef string          `json:"external_ref"`
	Seq         int64           `json:"seq"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Data        json.RawMessage `json:"data"`
}

func TestStreamsBrokerEndToEnd(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Postgres: the system of record the streams merge into.
	pg, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("app"), tcpostgres.WithUsername("app"), tcpostgres.WithPassword("app"),
		tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("pg connection string: %v", err)
	}
	store, err := postgres.Open(ctx, dsn, os.DirFS("../../migrations/postgres"))
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(store.Close)

	// Kafka: Redpanda-compatible broker container.
	kafkaContainer, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if err != nil {
		t.Fatalf("start kafka: %v", err)
	}
	t.Cleanup(func() { _ = kafkaContainer.Terminate(ctx) })
	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		t.Fatalf("kafka brokers: %v", err)
	}
	// Topics do not auto-create on this image; make them up front.
	seedTopics(t, brokers, e2eProfileTopic, e2eEventsTopic)

	// NATS: the module starts nats-server with -js (JetStream on).
	natsContainer, err := tcnats.Run(ctx, "nats:2.10-alpine")
	if err != nil {
		t.Fatalf("start nats: %v", err)
	}
	t.Cleanup(func() { _ = natsContainer.Terminate(ctx) })
	natsURL, err := natsContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("nats connection string: %v", err)
	}

	// The user both streams enrich.
	now := time.Now()
	user := model.User{ID: "e2e-u-1", Email: "e2e@example.com", Name: "E2E", ExternalRef: "e2e-ref", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(ctx, user, ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	breakers := reliability.NewBreakers(3, 5*time.Second)
	kafkaClient, err := kafkaplatform.Open(ctx, kafkaplatform.Config{
		Brokers: brokers, TopicProfile: e2eProfileTopic, TopicEvents: e2eEventsTopic, Group: "e2e-enricher",
	}, logger, breakers)
	if err != nil {
		t.Fatalf("open kafka adapter: %v", err)
	}
	t.Cleanup(kafkaClient.Close)
	natsClient, err := natsplatform.Open(ctx, natsplatform.Config{
		URL: natsURL, Stream: e2eStream, SubjectContact: e2eContactSub, SubjectEvents: "users.events.v1", Durable: "e2e-contact",
	}, logger, breakers)
	if err != nil {
		t.Fatalf("open nats adapter: %v", err)
	}
	t.Cleanup(natsClient.Close)

	observer := observStreams{}
	svc := service.NewStreamsService(store, observer, logger)
	publisher := service.MultiPublisher{Publishers: []service.EventPublisher{kafkaClient, natsClient}}
	svc.WithRelayPublisher(publisher)

	// Consumers under ctx cancelled at test end; supervised like the worker.
	consumerCtx, stopConsumers := context.WithCancel(ctx)
	defer stopConsumers()
	go func() { _ = kafkaClient.Consume(consumerCtx, svc.HandleProfileEvent) }()
	go func() { _ = natsClient.Consume(consumerCtx, svc.HandleContactEvent) }()

	// 1. Publish into both streams.
	profileEvent := streamEnvelope{
		EventID: "e2e-k-1", ExternalRef: "e2e-ref", Seq: 1, OccurredAt: time.Now().UTC(),
		Data: json.RawMessage(`{"company":"EndToEnd","job_title":"Tester"}`),
	}
	publishKafka(t, ctx, brokers, e2eProfileTopic, "e2e-ref", mustJSON(profileEvent))
	contactEvent := streamEnvelope{
		EventID: "e2e-n-1", ExternalRef: "e2e-ref", Seq: 1, OccurredAt: time.Now().UTC(),
		Data: json.RawMessage(`{"phone":"+1555E2E"}`),
	}
	publishNATS(t, ctx, natsURL, mustJSON(contactEvent))

	// 2. Both payloads land on the user row.
	waitFor(t, 20*time.Second, "both enrichments applied", func() bool {
		got, err := store.GetUser(ctx, "e2e-u-1")
		if err != nil {
			return false
		}
		return strings.Contains(got.EnrichProfile, "EndToEnd") && strings.Contains(got.EnrichContact, "+1555E2E")
	})

	// 3. Exactly once: redeliver both events; nothing changes, outbox grows.
	publishKafka(t, ctx, brokers, e2eProfileTopic, "e2e-ref", mustJSON(profileEvent))
	publishNATS(t, ctx, natsURL, mustJSON(contactEvent))
	time.Sleep(2 * time.Second)
	got, err := store.GetUser(ctx, "e2e-u-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	// JSONB normalizes whitespace; compare parsed. The redeliveries must
	// not have changed the merged payload.
	var profile map[string]any
	if err := json.Unmarshal([]byte(got.EnrichProfile), &profile); err != nil {
		t.Fatalf("profile not JSON: %q", got.EnrichProfile)
	}
	if profile["company"] != "EndToEnd" || profile["job_title"] != "Tester" {
		t.Fatalf("profile after redelivery = %q, want unchanged", got.EnrichProfile)
	}
	pending, err := store.CountOutboxPending(ctx)
	if err != nil || pending != 2 {
		t.Fatalf("outbox pending = %d err %v, want exactly 2 (one per applied event)", pending, err)
	}

	// 4. Poison on both brokers parks one dead letter each (malformed, and a
	//    valid event for a user that does not exist yet).
	publishKafka(t, ctx, brokers, e2eProfileTopic, "e2e-ref", []byte(`{"broken`))
	publishNATS(t, ctx, natsURL, mustJSON(streamEnvelope{
		EventID: "e2e-n-late", ExternalRef: "late-e2e-ref", Seq: 1, OccurredAt: time.Now().UTC(),
		Data: json.RawMessage(`{"phone":"x"}`),
	}))
	waitFor(t, 20*time.Second, "two dead letters parked", func() bool {
		letters, _, err := store.ListDeadLetters(ctx, repository.DeadLetterOptions{Pending: true})
		if err != nil {
			return false
		}
		malformed, unknown := false, false
		for _, letter := range letters {
			switch letter.Reason {
			case model.DeadLetterMalformed:
				malformed = true
			case model.DeadLetterUnknownUser:
				unknown = true
			}
		}
		return malformed && unknown
	})

	// 5. The late-user story: the parked event's user arrives after the fact.
	//    Replay fails while the user is missing, then lands the event.
	letters, _, err := store.ListDeadLetters(ctx, repository.DeadLetterOptions{Pending: true})
	if err != nil {
		t.Fatalf("list letters: %v", err)
	}
	var unknownID string
	for _, letter := range letters {
		if letter.Reason == model.DeadLetterUnknownUser {
			unknownID = letter.ID
		}
	}
	if unknownID == "" {
		t.Fatal("unknown-user letter not found")
	}
	if _, err := svc.ReplayDeadLetter(ctx, unknownID); err == nil {
		t.Fatal("replay of a ghost ref should fail with unknown external_ref")
	}
	stamped, _ := store.GetDeadLetter(ctx, unknownID)
	if stamped.ReplayedAt != nil {
		t.Fatal("failed replay must not stamp the letter")
	}
	// The user shows up; replay applies the event exactly once.
	late := now.Add(time.Second)
	lateUser := model.User{ID: "e2e-u-2", Email: "late@example.com", Name: "Late", ExternalRef: "late-e2e-ref", CreatedAt: late, UpdatedAt: late}
	if err := store.CreateUser(ctx, lateUser, ""); err != nil {
		t.Fatalf("create late user: %v", err)
	}
	if _, err := svc.ReplayDeadLetter(ctx, unknownID); err != nil {
		t.Fatalf("replay after user exists: %v", err)
	}
	if _, err := svc.ReplayDeadLetter(ctx, unknownID); err == nil {
		t.Fatal("second replay must be rejected as already replayed")
	}
	lateGot, err := store.GetUser(ctx, "e2e-u-2")
	if err != nil || !strings.Contains(lateGot.EnrichContact, "x") {
		t.Fatalf("late user contact = %q err %v, want the replayed payload", lateGot.EnrichContact, err)
	}

	// 6. The outbox relay publishes the user.enriched events to Kafka:
	//    two from the original applies, one from the replay.
	published, err := svc.RelayOutboxOnce(ctx, publisher, service.RelayConfig{Batch: 100, LeaseSeconds: 60, MaxAttempts: 3})
	if err != nil || published != 3 {
		t.Fatalf("relay published = %d err %v, want 3", published, err)
	}
	waitFor(t, 15*time.Second, "relayed events visible on user-events.v1", func() bool {
		return countKafkaRecords(t, brokers, e2eEventsTopic) >= 3
	})
	for _, record := range readKafkaRecords(t, brokers, e2eEventsTopic) {
		var payload map[string]any
		if err := json.Unmarshal(record, &payload); err != nil {
			t.Fatalf("relayed payload not JSON: %s", string(record))
		}
		if payload["event_type"] == "" && payload["source"] == nil {
			t.Logf("relayed: %s", string(record))
		}
	}

	// 7. Health pings answer for both brokers.
	if err := kafkaClient.Ping(ctx); err != nil {
		t.Fatalf("kafka ping: %v", err)
	}
	if err := natsClient.Ping(ctx); err != nil {
		t.Fatalf("nats ping: %v", err)
	}
}

// --- helpers -----------------------------------------------------------------

// mustJSON marshals v or fails the test.
func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err) // static test payloads; a marshal failure is a test bug
	}
	return raw
}

func seedTopics(t *testing.T, brokers []string, topics ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatalf("admin client: %v", err)
	}
	defer cl.Close()
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("kafka ping: %v", err)
	}
	adm := kadm.NewClient(cl)
	for _, topic := range topics {
		if _, err := adm.CreateTopic(ctx, 1, 1, nil, topic); err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("create topic %s: %v", topic, err)
		}
	}
}

func publishKafka(t *testing.T, ctx context.Context, brokers []string, topic, key string, payload []byte) {
	t.Helper()
	raw := payload
	cl := rawKafkaE2E(t, brokers)
	defer cl.Close()
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: raw}
	acked := make(chan error, 1)
	cl.Produce(ctx, rec, func(_ *kgo.Record, err error) { acked <- err })
	select {
	case err := <-acked:
		if err != nil {
			t.Fatalf("publish to %s: %v", topic, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("publish to %s timed out", topic)
	}
}

func rawKafkaE2E(t *testing.T, brokers []string) *kgo.Client {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatalf("kafka client: %v", err)
	}
	return cl
}

func publishNATS(t *testing.T, ctx context.Context, url string, payload []byte) {
	t.Helper()
	raw := payload
	nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("nats jetstream: %v", err)
	}
	if _, err := js.PublishAsync(e2eContactSub, raw); err != nil {
		t.Fatalf("nats publish: %v", err)
	}
	<-js.PublishAsyncComplete()
}

func countKafkaRecords(t *testing.T, brokers []string, topic string) int {
	return len(readKafkaRecords(t, brokers, topic))
}

func readKafkaRecords(t *testing.T, brokers []string, topic string) [][]byte {
	t.Helper()
	group := fmt.Sprintf("e2e-read-%d", time.Now().UnixNano())
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("reader client: %v", err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out [][]byte
	for {
		fetches := cl.PollRecords(ctx, 100)
		fetches.EachError(func(_ string, _ int32, err error) {
			t.Logf("read error: %v", err)
		})
		fetches.EachRecord(func(rec *kgo.Record) {
			out = append(out, rec.Value)
		})
		if ctx.Err() != nil || len(fetches.Records()) == 0 {
			break
		}
	}
	return out
}

func waitFor(t *testing.T, timeout time.Duration, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// observStreams is the quiet observer; metrics are covered in unit tests.
type observStreams struct{}

func (observStreams) EnrichmentEvent(string, string)                {}
func (observStreams) EnrichmentApplyDuration(string, time.Duration) {}
func (observStreams) OutboxRelayed(string, int)                     {}
func (observStreams) DeadLetterParked()                             {}
func (observStreams) DeadLetterReplayed()                           {}
func (observStreams) GaugesRefreshed(int, int)                      {}
