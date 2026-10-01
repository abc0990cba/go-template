package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

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
