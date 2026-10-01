package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

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
