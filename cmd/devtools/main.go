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
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// streamEnvelope mirrors the enrichment wire format decoded by the backend
// (internal/streams.DecodeProfileEvent / DecodeContactEvent).
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

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "devtools: "+format+"\n", args...)
	os.Exit(1)
}
