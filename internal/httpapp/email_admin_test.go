package httpapp

// The email delivery-log admin surface over the memory store.

import (
	"context"
	"net/http"
	"testing"

	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/repository"
)

func TestEmailDeliveryLogAdminSurface(t *testing.T) {
	store := repository.NewMemoryStore()
	server := newStreamTestServer(t, store, nil)
	admin := registerAndLogin(t, server, "mail-admin@example.com")

	// The memory store doubles as the MessageStore here, mirroring the
	// production memory wiring; seed one delivery-log row directly.
	msg := email.Message{
		ID: kernel.NewID(), To: "ada@example.com", Subject: "Confirm your email",
		Body: "<p>hi</p>", Kind: email.KindVerify, Status: email.StatusQueued,
	}
	if err := store.CreateEmailMessage(context.Background(), msg); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	// The log lists it for a superadmin.
	status, body := callJSON(t, server, http.MethodGet, "/api/emails", "", admin)
	if status != http.StatusOK {
		t.Fatalf("list: want 200, got %d (%v)", status, body)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("list: want 1 entry, got %v", body)
	}
	first, _ := data[0].(map[string]any)
	if first["id"] != msg.ID || first["status"] != email.StatusQueued {
		t.Fatalf("entry: %v", first)
	}

	// Single get.
	status, body = callJSON(t, server, http.MethodGet, "/api/emails/"+msg.ID, "", admin)
	if status != http.StatusOK {
		t.Fatalf("get: want 200, got %d (%v)", status, body)
	}

	// Retry re-queues; with no dispatcher the send runs inline on this
	// request. There is no SMTP behind the test, so the attempt records a
	// failure — the endpoint still answers 200 with the row (an admin retry
	// must not 500 on a dead SMTP).
	status, body = callJSON(t, server, http.MethodPost, "/api/emails/"+msg.ID+"/retry", "", admin)
	if status != http.StatusOK {
		t.Fatalf("retry: want 200, got %d (%v)", status, body)
	}
	if body["id"] != msg.ID {
		t.Fatalf("retry body: %v", body)
	}

	// Unknown ids answer 404 problem+json.
	status, body = callJSON(t, server, http.MethodGet, "/api/emails/nope", "", admin)
	if status != http.StatusNotFound {
		t.Fatalf("unknown id: want 404, got %d (%v)", status, body)
	}
	if title, _ := body["title"].(string); title != "not found" {
		t.Fatalf("unknown id: want problem+json, got %v", body)
	}
}

func TestAdminUnlockEndpoint(t *testing.T) {
	server := newTestServer(t)
	admin := registerAndLogin(t, server, "unlock-admin@example.com")

	// Unlocking any id answers 200 uniformly — the endpoint must not leak
	// which accounts are locked (or exist).
	status, body := callJSON(t, server, http.MethodPost, "/api/users/some-user-id/unlock", "", admin)
	if status != http.StatusOK {
		t.Fatalf("unlock: want 200, got %d (%v)", status, body)
	}
	if body["status"] != "unlocked" {
		t.Fatalf("unlock body: %v", body)
	}
}
