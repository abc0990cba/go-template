// Command devtools is the local testing toolbox for the enrichment streams
// and resilience features. It publishes events into Kafka and NATS, watches
// what the backend processed, and drives the breaker / rate-limit /
// idempotency demos. Each scenario prints what to check in the app.
//
//	devtools kafka-profile --ref demo-u-0001 --seq 1
//	devtools nats-contact  --ref demo-u-0001 --seq 1
//	devtools demo          --ref demo-u-0001
//	devtools demo-dlq
//	devtools consume-user-events --timeout 15s
//	devtools flaky-server --addr :8099
//	devtools demo-breaker
//	devtools hammer --requests 200 --concurrency 10
//	devtools dlq-list
//	devtools dlq-replay --id <dead-letter-id>
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/your-org/go-template/internal/reliability"
)

// streamEnvelope mirrors the enrichment wire format decoded by the backend
// (internal/service.DecodeProfileEvent / DecodeContactEvent).
type streamEnvelope struct {
	EventID     string          `json:"event_id"`
	ExternalRef string          `json:"external_ref"`
	Seq         int64           `json:"seq"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Data        json.RawMessage `json:"data"`
}

const (
	defaultProfileTopic = "user-profile.v1"
	defaultEventsTopic  = "user-events.v1"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "kafka-profile":
		err = cmdKafkaProfile(os.Args[2:], log)
	case "nats-contact":
		err = cmdNatsContact(os.Args[2:], log)
	case "consume-user-events":
		err = cmdConsumeUserEvents(os.Args[2:], log)
	case "demo":
		err = cmdDemo(os.Args[2:], log)
	case "demo-dlq":
		err = cmdDemoDLQ(os.Args[2:], log)
	case "demo-breaker":
		err = cmdDemoBreaker(os.Args[2:], log)
	case "flaky-server":
		err = cmdFlakyServer(os.Args[2:], log)
	case "hammer":
		err = cmdHammer(os.Args[2:], log)
	case "dlq-list":
		err = cmdDLQList(os.Args[2:], log)
	case "dlq-replay":
		err = cmdDLQReplay(os.Args[2:], log)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Error("devtools failed", "err", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `devtools — local testing toolbox for the enrichment streams

Usage:
  devtools kafka-profile       [--brokers localhost:19092] [--topic user-profile.v1] --ref demo-u-0001 --seq 1 [--count 5] [--bad]
  devtools nats-contact        [--url nats://localhost:4222] [--stream USERS] [--subject users.contact.v1] --ref demo-u-0001 --seq 1 [--count 5] [--bad]
  devtools consume-user-events [--brokers localhost:19092] [--topic user-events.v1] [--timeout 15s]
  devtools demo                [--api http://localhost:8080] --ref demo-u-0001
  devtools demo-dlq            [--api http://localhost:8080]
  devtools demo-breaker
  devtools flaky-server        [--addr :8099] [--failures 4]
  devtools hammer              [--target http://localhost:8080/api/settings/public] [--requests 200] [--concurrency 10]
  devtools dlq-list            [--api http://localhost:8080]
  devtools dlq-replay          [--api http://localhost:8080] --id <id>

Environment fallbacks: KAFKA_BROKERS, NATS_URL, API_BASE, DEMO_EMAIL, DEMO_PASSWORD.
`)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func itoa(n int64) string { return fmt.Sprintf("%d", n) }

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

// dialNATS connects and makes sure the stream exists so a fresh environment
// can publish immediately.
func dialNATS(url, stream string, subjects []string) (*nats.Conn, nats.JetStreamContext, error) {
	nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("nats jetstream: %w", err)
	}
	if _, err := js.StreamInfo(stream); errors.Is(err, nats.ErrStreamNotFound) {
		if _, err := js.AddStream(&nats.StreamConfig{Name: stream, Subjects: subjects}); err != nil {
			nc.Close()
			return nil, nil, fmt.Errorf("nats add stream: %w", err)
		}
	} else if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("nats stream info: %w", err)
	}
	return nc, js, nil
}

// --- api client ---------------------------------------------------------------

type apiClient struct {
	base  string
	token string
	http  *http.Client
}

func newAPIClient(base, email, password string) (*apiClient, error) {
	c := &apiClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 10 * time.Second}}
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(context.Background(), http.MethodPost, "/api/auth/login",
		map[string]string{"email": email, "password": password}, &out); err != nil {
		return nil, fmt.Errorf("login as %s: %w", email, err)
	}
	c.token = out.Token
	return c, nil
}

func (c *apiClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// findUserByExternalRef scans the users list for the given correlation id.
func (c *apiClient) findUserByExternalRef(ctx context.Context, ref string) (map[string]any, error) {
	for offset := 0; offset < 1000; offset += 100 {
		var page struct {
			Data []map[string]any `json:"data"`
		}
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/users?limit=100&offset=%d", offset), nil, &page); err != nil {
			return nil, err
		}
		for _, user := range page.Data {
			if user["external_ref"] == ref {
				return user, nil
			}
		}
		if len(page.Data) < 100 {
			break
		}
	}
	return nil, fmt.Errorf("no user with external_ref %q found in the API", ref)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "devtools: "+format+"\n", args...)
	os.Exit(1)
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

// --- nats-contact ----------------------------------------------------------------

func cmdNatsContact(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("nats-contact", flag.ExitOnError)
	var (
		url     = fs.String("url", envOr("NATS_URL", "nats://localhost:4222"), "NATS URL")
		stream  = fs.String("stream", "USERS", "JetStream stream name")
		subject = fs.String("subject", "users.contact.v1", "subject for contact events")
		ref     = fs.String("ref", "demo-u-0001", "external_ref of the user to enrich")
		seq     = fs.Int64("seq", 1, "event sequence number (per-source, monotonic)")
		count   = fs.Int("count", 1, "how many events to publish")
		phone   = fs.String("phone", "+1 555 0100", "phone field value")
		bad     = fs.Bool("bad", false, "publish a malformed (poison) message instead")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	nc, js, err := dialNATS(*url, *stream, []string{*subject, "users.events.v1"})
	if err != nil {
		return err
	}
	defer func() { _ = nc.Drain() }()

	for i := range *count {
		eventID := fmt.Sprintf("devtools-contact-%s-%d-%d", *ref, *seq+int64(i), time.Now().UnixNano())
		var payload []byte
		if *bad {
			payload = []byte(`{"event_id":"` + eventID + `","external_ref":"` + *ref + `","data":`)
		} else {
			env := streamEnvelope{
				EventID: eventID, ExternalRef: *ref, Seq: *seq + int64(i), OccurredAt: time.Now().UTC(),
				Data: json.RawMessage(fmt.Sprintf(`{"phone":%q,"preferred_channel":"email","enriched_by":"devtools"}`, *phone)),
			}
			payload, _ = json.Marshal(env)
		}
		msg := nats.NewMsg(*subject)
		msg.Data = payload
		msg.Header.Set("Nats-Msg-Id", eventID)
		if _, err := js.PublishMsg(msg, nats.Context(ctx)); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		log.Info("published to nats", "subject", *subject, "event_id", eventID, "seq", *seq+int64(i), "bad", *bad)
	}
	if *bad {
		fmt.Println("Poison message published: the worker parks it in dead letters. Check the Streams page in the app or run `devtools dlq-list`.")
		return nil
	}
	fmt.Printf("Published %d contact event(s) for %s. Open the user's page in the app: the \"Contact (NATS)\" card now shows the fields.\n",
		*count, *ref)
	return nil
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

// --- demo (end-to-end enrichment) ---------------------------------------------------

func cmdDemo(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	var (
		apiBase = fs.String("api", envOr("API_BASE", "http://localhost:8080"), "api base url")
		ref     = fs.String("ref", "demo-u-0001", "external_ref of the user to enrich")
		email   = fs.String("email", envOr("DEMO_EMAIL", "demo@go-template.local"), "login email")
		pass    = fs.String("password", envOr("DEMO_PASSWORD", "demo-password-123"), "login password")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	seq := time.Now().Unix()%1000 + 1
	if err := cmdKafkaProfile([]string{"--ref", *ref, "--seq", itoa(seq), "--company", "Devtools Demo " + itoa(seq)}, log); err != nil {
		return err
	}
	if err := cmdNatsContact([]string{"--ref", *ref, "--seq", itoa(seq), "--phone", "+1 555 01" + itoa(seq%100)}, log); err != nil {
		return err
	}

	api, err := newAPIClient(*apiBase, *email, *pass)
	if err != nil {
		return err
	}
	fmt.Println("Waiting for the worker to apply both events...")
	var user map[string]any
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		user, err = api.findUserByExternalRef(ctx, *ref)
		if err == nil {
			profile, _ := user["enrich_profile"].(string)
			contact, _ := user["enrich_contact"].(string)
			if strings.Contains(profile, "Devtools Demo") && strings.Contains(contact, "enriched_by") {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("user not visible yet (is the worker running with the brokers enabled?): %w", err)
	}
	pretty, _ := json.MarshalIndent(map[string]any{
		"id": user["id"], "email": user["email"], "external_ref": user["external_ref"],
		"enrich_profile": user["enrich_profile"], "enrich_contact": user["enrich_contact"], "enriched_at": user["enriched_at"],
	}, "", "  ")
	fmt.Printf("Merged user state from both streams:\n%s\n", pretty)
	fmt.Println("The same fields render on the user's page in the app (Profile (Kafka) / Contact (NATS) cards).")
	return nil
}

// --- demo-dlq -------------------------------------------------------------------------

func cmdDemoDLQ(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("demo-dlq", flag.ExitOnError)
	var (
		apiBase = fs.String("api", envOr("API_BASE", "http://localhost:8080"), "api base url")
		email   = fs.String("email", envOr("DEMO_EMAIL", "demo@go-template.local"), "login email")
		pass    = fs.String("password", envOr("DEMO_PASSWORD", "demo-password-123"), "login password")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 1. Poison: malformed JSON on Kafka.
	if err := cmdKafkaProfile([]string{"--ref", "demo-u-0002", "--seq", "1", "--bad"}, log); err != nil {
		return err
	}
	// 2. Unknown user: a valid event for a ref no user carries.
	if err := cmdNatsContact([]string{"--ref", "ghost-user", "--seq", "1"}, log); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	api, err := newAPIClient(*apiBase, *email, *pass)
	if err != nil {
		return err
	}
	fmt.Println("Waiting for the worker to park both messages...")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		letters, err := listDeadLetters(ctx, api)
		if err != nil {
			return err
		}
		malformed, unknown := false, false
		for _, letter := range letters {
			switch letter["reason"] {
			case "malformed":
				malformed = true
			case "unknown_user":
				unknown = true
			}
		}
		if malformed && unknown {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	letters, err := listDeadLetters(ctx, api)
	if err != nil {
		return err
	}
	fmt.Printf("Dead letters (%d):\n", len(letters))
	for _, letter := range letters {
		fmt.Printf("  id=%s source=%s reason=%s topic=%s replayed=%v\n",
			letter["id"], letter["source"], letter["reason"], letter["topic"], letter["replayed_at"] != nil)
	}
	fmt.Println("The same rows render on the Streams page in the app. Replay one:")
	fmt.Printf("  devtools dlq-replay --api %s --id <id>\n", *apiBase)
	return nil
}

func listDeadLetters(ctx context.Context, api *apiClient) ([]map[string]any, error) {
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := api.do(ctx, http.MethodGet, "/api/streams/dead-letters?pending=true&limit=50", nil, &page); err != nil {
		return nil, fmt.Errorf("list dead letters: %w", err)
	}
	return page.Data, nil
}

// --- dlq-list / dlq-replay ---------------------------------------------------------------

func cmdDLQList(args []string, _ *slog.Logger) error {
	fs := flag.NewFlagSet("dlq-list", flag.ExitOnError)
	var (
		apiBase = fs.String("api", envOr("API_BASE", "http://localhost:8080"), "api base url")
		email   = fs.String("email", envOr("DEMO_EMAIL", "demo@go-template.local"), "login email")
		pass    = fs.String("password", envOr("DEMO_PASSWORD", "demo-password-123"), "login password")
		all     = fs.Bool("all", false, "include replayed letters")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	api, err := newAPIClient(*apiBase, *email, *pass)
	if err != nil {
		return err
	}
	var query string
	if *all {
		query = "?pending=false&limit=50"
	} else {
		query = "?pending=true&limit=50"
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := api.do(ctx, http.MethodGet, "/api/streams/dead-letters"+query, nil, &page); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Println("No dead letters. Publish a poison message with `make demo-dlq`.")
		return nil
	}
	for _, letter := range page.Data {
		fmt.Printf("id=%s source=%s reason=%s topic=%s attempts=%v replayed=%v\n",
			letter["id"], letter["source"], letter["reason"], letter["topic"], letter["attempts"], letter["replayed_at"] != nil)
	}
	return nil
}

func cmdDLQReplay(args []string, _ *slog.Logger) error {
	fs := flag.NewFlagSet("dlq-replay", flag.ExitOnError)
	var (
		apiBase = fs.String("api", envOr("API_BASE", "http://localhost:8080"), "api base url")
		email   = fs.String("email", envOr("DEMO_EMAIL", "demo@go-template.local"), "login email")
		pass    = fs.String("password", envOr("DEMO_PASSWORD", "demo-password-123"), "login password")
		id      = fs.String("id", "", "dead letter id")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("--id is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	api, err := newAPIClient(*apiBase, *email, *pass)
	if err != nil {
		return err
	}
	var letter map[string]any
	if err := api.do(ctx, http.MethodPost, "/api/streams/dead-letters/"+*id+"/replay", nil, &letter); err != nil {
		return err
	}
	fmt.Printf("Replayed dead letter %s (source=%s reason=%s); refreshed state is on the user's page in the app.\n",
		letter["id"], letter["source"], letter["reason"])
	return nil
}

// --- flaky-server + demo-breaker ---------------------------------------------------------

// flakyServer answers 500 for the first failures requests, then 200.
type flakyServer struct {
	failures int64
	count    atomic.Int64
}

func (f *flakyServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if f.count.Add(1) <= f.failures {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream having a bad day"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"results":[{"name":"Recovered User"}]}`))
}

func cmdFlakyServer(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("flaky-server", flag.ExitOnError)
	var (
		addr     = fs.String("addr", ":8099", "listen address")
		failures = fs.Int64("failures", 4, "how many requests to fail before recovering")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	srv := &http.Server{Addr: *addr, Handler: &flakyServer{failures: *failures}, ReadHeaderTimeout: 3 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	log.Info("flaky server listening", "addr", *addr, "failing_first", *failures,
		"use_as", "APP_ENRICHMENT_URL=http://localhost"+*addr+"/api/?results=1")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// cmdDemoBreaker demonstrates the circuit breaker state machine in-process,
// using the exact same reliability package the backend wraps its outbound
// calls with. Run the real-server variant by pointing APP_ENRICHMENT_URL at
// `devtools flaky-server` and triggering an enrichment job.
func cmdDemoBreaker(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("demo-breaker", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Same defaults as the backend: 3 consecutive failures trip, 2s probe.
	flaky := &flakyServer{failures: 4}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: flaky, ReadHeaderTimeout: 3 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	defer func() { _ = srv.Close() }()

	breakers := reliability.NewBreakers(3, 2*time.Second)
	events := make(chan string, 32)
	breakers.SetOnChange(func(name string, from, to reliability.CircuitState) {
		events <- fmt.Sprintf("breaker %s: %s -> %s", name, from, to)
	})
	fmt.Printf("Firing guarded requests at a flaky upstream (%s)...\n", listener.Addr())
	client := &http.Client{Timeout: 2 * time.Second}
	for i := range 10 {
		var ( //nolint:revive // two returns from one guarded call
			status int
		)
		err := breakers.Do(context.Background(), "enrichment.http", func() error {
			resp, err := client.Get(fmt.Sprintf("http://%s/profile", listener.Addr()))
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			status = resp.StatusCode
			if resp.StatusCode >= 500 {
				return fmt.Errorf("upstream status %d", resp.StatusCode)
			}
			return nil
		})
		log.Info("request", "n", i+1, "status", status, "err", err)
		time.Sleep(400 * time.Millisecond)
	}
	time.Sleep(2200 * time.Millisecond) // let the recovery probe land
	close(events)
	var seen []string
	for event := range events {
		seen = append(seen, event)
	}
	sort.Strings(seen)
	fmt.Println("Breaker transitions:")
	for _, event := range seen {
		fmt.Println("  " + event)
	}
	fmt.Println("The backend does the same around its outbound calls — watch app_cb_state{name=\"enrichment.http\"} on /metrics.")
	return nil
}

// --- hammer (rate limiting) -----------------------------------------------------------------

func cmdHammer(args []string, _ *slog.Logger) error {
	fs := flag.NewFlagSet("hammer", flag.ExitOnError)
	var (
		target      = fs.String("target", envOr("API_BASE", "http://localhost:8080")+"/api/settings/public", "url to hammer")
		requests    = fs.Int("requests", 200, "total requests")
		concurrency = fs.Int("concurrency", 10, "parallel workers")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var (
		mu       sync.Mutex
		statuses = map[int]int{}
		wg       sync.WaitGroup
		retry    atomic.Value
	)
	work := make(chan int)
	for range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			for range work {
				resp, err := client.Get(*target)
				if err != nil {
					continue
				}
				if resp.StatusCode == http.StatusTooManyRequests {
					retry.Store(resp.Header.Get("Retry-After"))
				}
				_ = resp.Body.Close()
				mu.Lock()
				statuses[resp.StatusCode]++
				mu.Unlock()
			}
		}()
	}
	for i := range *requests {
		work <- i
	}
	close(work)
	wg.Wait()

	codes := make([]int, 0, len(statuses))
	for code := range statuses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	fmt.Printf("Hammered %s with %d requests:\n", *target, *requests)
	for _, code := range codes {
		fmt.Printf("  %d responses: %d\n", code, statuses[code])
	}
	if retryAfter, ok := retry.Load().(string); ok && retryAfter != "" {
		fmt.Printf("429s carried Retry-After: %ss\n", retryAfter)
	}
	if statuses[http.StatusTooManyRequests] == 0 {
		fmt.Println("No 429s observed — raise the rate or lower APP_API_RATE_RPS on the api.")
	} else {
		fmt.Println("Rate limiting is shedding excess traffic; app_rate_limit_rejected_total{scope=\"api\"} counts it on /metrics.")
	}
	return nil
}
