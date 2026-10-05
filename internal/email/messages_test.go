package email_test

// The delivery log: recorded messages, the sync and queue transports, the
// worker re-run entry point, and the admin retry. The repository memory
// store backs the log, exactly like the production wiring passes it.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/repository"
)

// captureDispatcher records dispatched messages; queue mode hands delivery
// to this port instead of sending inline.
type captureDispatcher struct {
	sent []email.Message
}

func (d *captureDispatcher) Dispatch(_ context.Context, msg email.Message) {
	d.sent = append(d.sent, msg)
}

func newMessageService(t *testing.T) (*email.Service, *captureMailer, *captureDispatcher) {
	t.Helper()
	store := repository.NewMemoryStore()
	sender := &captureMailer{}
	dispatcher := &captureDispatcher{}
	svc := email.NewService(store, store, sender, auth.HashPassword, "https://app.example.com/", discardLogger()).
		WithMessages(store, dispatcher)
	// The flows resolve the recipient first; seed the account they target.
	accounts := auth.NewService(store, nil, time.Hour)
	if _, _, err := accounts.Register(t.Context(), "ada@example.com", "Ada", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc, sender, dispatcher
}

func TestDeliverRecordsMessageAndUsesQueue(t *testing.T) {
	svc, sender, dispatcher := newMessageService(t)

	// deliver is exercised through the public flow: a password reset.
	if err := svc.RequestPasswordReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}

	if len(sender.sent) != 0 {
		t.Fatalf("queue mode must not send inline, sent %d", len(sender.sent))
	}
	if len(dispatcher.sent) != 1 {
		t.Fatalf("dispatch: want 1 message, got %d", len(dispatcher.sent))
	}
	msg := dispatcher.sent[0]
	if msg.Status != email.StatusQueued || msg.Kind != email.KindReset {
		t.Fatalf("recorded message: %+v", msg)
	}
	if msg.To != "ada@example.com" {
		t.Fatalf("recorded recipient: %q", msg.To)
	}

	// The delivery log shows the row (via the service's admin surface).
	messages, err := svc.Messages(t.Context(), "", 10, 0)
	if err != nil || len(messages) != 1 {
		t.Fatalf("list: %d messages, err %v", len(messages), err)
	}

	// The worker's entry point delivers and marks the row sent.
	if err := svc.DeliverQueued(t.Context(), msg.ID); err != nil {
		t.Fatalf("deliver queued: %v", err)
	}
	done, err := svc.GetMessage(t.Context(), msg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if done.Status != email.StatusSent || done.Attempts != 1 || done.SentAt == nil {
		t.Fatalf("delivered message: %+v", done)
	}
}

func TestSyncModeSendsInlineAndRecordsOutcome(t *testing.T) {
	store := repository.NewMemoryStore()
	sender := &captureMailer{}
	// No dispatcher: sync mode.
	svc := email.NewService(store, store, sender, auth.HashPassword, "https://app.example.com/", discardLogger()).
		WithMessages(store, nil)
	// The flow resolves the recipient first; seed the account it targets.
	accounts := auth.NewService(store, nil, time.Hour)
	if _, _, err := accounts.Register(t.Context(), "ada@example.com", "Ada", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := svc.RequestPasswordReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sync mode: want inline send, got %d", len(sender.sent))
	}
	messages, err := svc.Messages(t.Context(), email.StatusSent, 10, 0)
	if err != nil || len(messages) != 1 {
		t.Fatalf("sent log: %d entries, err %v", len(messages), err)
	}
}

func TestDeliverQueuedMarksDeadAfterBudget(t *testing.T) {
	store := repository.NewMemoryStore()
	sender := &captureMailer{failNext: true}
	dispatcher := &captureDispatcher{}
	svc := email.NewService(store, store, sender, auth.HashPassword, "https://app.example.com/", discardLogger()).
		WithMessages(store, dispatcher)
	// failNext resets after one failure: make every attempt fail.
	failAlways := func() { sender.failNext = true }
	accounts := auth.NewService(store, nil, time.Hour)
	if _, _, err := accounts.Register(t.Context(), "ada@example.com", "Ada", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := svc.RequestPasswordReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}
	msg := dispatcher.sent[0]

	// Attempts 1..4 fail and keep the row failed; the worker keeps retrying.
	for attempt := 1; attempt < email.MaxDeliveryAttempts; attempt++ {
		failAlways()
		if err := svc.DeliverQueued(t.Context(), msg.ID); err == nil {
			t.Fatalf("attempt %d: want the send error back", attempt)
		}
		row, err := svc.GetMessage(t.Context(), msg.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if row.Status != email.StatusFailed || row.Attempts != attempt {
			t.Fatalf("attempt %d: status %s, attempts %d", attempt, row.Status, row.Attempts)
		}
	}

	// The last allowed attempt marks the row dead and stops asking for
	// retries (nil), so the task does not burn asynq's own budget.
	failAlways()
	if err := svc.DeliverQueued(t.Context(), msg.ID); err != nil {
		t.Fatalf("final attempt must stop retrying, got %v", err)
	}
	row, err := svc.GetMessage(t.Context(), msg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != email.StatusDead || row.Attempts != email.MaxDeliveryAttempts {
		t.Fatalf("dead row: status %s, attempts %d", row.Status, row.Attempts)
	}
	if row.LastError == "" {
		t.Fatal("dead row must keep the last error")
	}
}

func TestRetryMessage(t *testing.T) {
	svc, _, dispatcher := newMessageService(t)
	if err := svc.RequestPasswordReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}
	msg := dispatcher.sent[0]

	// A queued message with a dispatcher is re-dispatched.
	retried, err := svc.RetryMessage(t.Context(), msg.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.Status != email.StatusQueued || len(dispatcher.sent) != 2 {
		t.Fatalf("retry did not re-dispatch: %+v (dispatched %d)", retried, len(dispatcher.sent))
	}

	// A sent message refuses retry.
	if err := svc.DeliverQueued(t.Context(), msg.ID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if _, err := svc.RetryMessage(t.Context(), msg.ID); !errors.Is(err, kernel.ErrConflict) {
		t.Fatalf("retry of sent message: want conflict, got %v", err)
	}
	if _, err := svc.RetryMessage(t.Context(), "no-such-message"); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("retry of unknown message: want not found, got %v", err)
	}
}

func TestPruneMessages(t *testing.T) {
	svc, _, dispatcher := newMessageService(t)
	if err := svc.RequestPasswordReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}
	if len(dispatcher.sent) != 1 {
		t.Fatalf("dispatch: want 1, got %d", len(dispatcher.sent))
	}
	n, err := svc.PruneMessages(t.Context(), time.Now().Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune: %d rows, err %v", n, err)
	}
	if _, err := svc.GetMessage(t.Context(), dispatcher.sent[0].ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("pruned message must be gone, got %v", err)
	}
}
