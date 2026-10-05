package email

// The delivery log and its two transports. Every outbound transactional
// message is recorded as one row (the admin delivery log), then either sent
// inline (single-process mode, dispatcher nil) or handed to the queue
// dispatcher, which makes the worker process own retries and backoff.

import (
	"context"
	"errors"
	"html/template"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/mailer"
)

// Delivery log statuses.
const (
	StatusQueued = "queued"
	StatusSent   = "sent"
	StatusFailed = "failed"
	StatusDead   = "dead"
)

// MaxDeliveryAttempts bounds queue-driven retries before the message parks
// as dead — past this, delivery needs a human (or a ResetPassword link with
// a fresh token), not another silent retry.
const MaxDeliveryAttempts = 5

// Message is one recorded outbound email.
type Message struct {
	ID        string     `json:"id"`
	To        string     `json:"to"`
	Subject   string     `json:"subject"`
	Body      string     `json:"-"`
	Kind      string     `json:"kind"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	LastError string     `json:"last_error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
}

// MessageStore persists the delivery log.
type MessageStore interface {
	CreateEmailMessage(ctx context.Context, msg Message) error
	GetEmailMessage(ctx context.Context, id string) (Message, error)
	ListEmailMessages(ctx context.Context, status string, limit, offset int) ([]Message, error)
	UpdateEmailMessageStatus(ctx context.Context, id, status string, attempts int, lastError string, sentAt *time.Time) error
	PruneEmailMessages(ctx context.Context, olderThan time.Time) (int64, error)
}

// Dispatcher carries a recorded message toward its transport. The queue
// implementation hands it to the worker process; a nil dispatcher on the
// service means "send inline before returning".
type Dispatcher interface {
	Dispatch(ctx context.Context, msg Message)
}

// WithMessages turns on the delivery log. The dispatcher is optional: nil
// sends inline on the calling goroutine, preserving the pre-log behavior of
// returning send errors to the flow that triggered the mail.
func (s *Service) WithMessages(store MessageStore, dispatcher Dispatcher) *Service {
	s.messages = store
	s.dispatcher = dispatcher
	return s
}

// deliver records the message (when the log is on) and carries it to the
// transport. Without the log it is exactly the old direct send.
func (s *Service) deliver(ctx context.Context, kind, to, subject string, body template.HTML) error {
	if s.messages == nil {
		return s.mailer.Send(ctx, mailer.Message{To: to, Subject: subject, Body: body})
	}
	msg := Message{
		ID: kernel.NewID(), To: to, Subject: subject, Body: string(body),
		Kind: kind, Status: StatusQueued, CreatedAt: s.now(),
	}
	if err := s.messages.CreateEmailMessage(ctx, msg); err != nil {
		return err
	}
	if s.dispatcher != nil {
		// The queue owns delivery now; the caller's flow is done. Send
		// errors surface on the message row and the admin log, not here.
		s.dispatcher.Dispatch(ctx, msg)
		return nil
	}
	return s.attempt(ctx, msg)
}

// attempt sends one recorded message once and records the outcome. The
// returned error is the send error, for callers that can act on it.
func (s *Service) attempt(ctx context.Context, msg Message) error {
	err := s.send(ctx, msg)
	attempts := msg.Attempts + 1
	if err == nil {
		now := s.now()
		s.recordStatus(ctx, msg.ID, StatusSent, attempts, "", &now)
		observ.EmailMessages.WithLabelValues("sent").Inc()
		return nil
	}
	if attempts >= MaxDeliveryAttempts {
		s.recordStatus(ctx, msg.ID, StatusDead, attempts, err.Error(), nil)
		observ.EmailMessages.WithLabelValues("dead").Inc()
		return err
	}
	s.recordStatus(ctx, msg.ID, StatusFailed, attempts, err.Error(), nil)
	observ.EmailMessages.WithLabelValues("failed").Inc()
	return err
}

// send carries one recorded message to the transport. A nil mailer (a
// process assembled without one) is a recorded failure, not a panic: the
// delivery log must survive infrastructure being unconfigured.
func (s *Service) send(ctx context.Context, msg Message) error {
	if s.mailer == nil {
		return errors.New("no mailer configured")
	}
	return s.mailer.Send(ctx, mailer.Message{
		To: msg.To, Subject: msg.Subject,
		// #nosec G203 -- the body was rendered by mailer.Button at record time
		Body: template.HTML(msg.Body),
	})
}

func (s *Service) recordStatus(ctx context.Context, id, status string, attempts int, lastErr string, sentAt *time.Time) {
	if err := s.messages.UpdateEmailMessageStatus(ctx, id, status, attempts, lastErr, sentAt); err != nil {
		s.log("email delivery log update failed", err, "message_id", id)
	}
}

// DeliverQueued is the worker's re-run entry point for one recorded message.
// A send failure returns an error so asynq retries with backoff; once the
// attempts budget is spent the row goes dead and the handler returns nil —
// the task stops consuming retries an admin can now see and trigger.
func (s *Service) DeliverQueued(ctx context.Context, messageID string) error {
	msg, err := s.messages.GetEmailMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if msg.Status == StatusSent {
		return nil // idempotent redelivery: the log is the source of truth
	}
	err = s.attempt(ctx, msg)
	if msg.Attempts+1 >= MaxDeliveryAttempts {
		return nil
	}
	return err
}

// RetryMessage re-queues a failed or dead message. With a dispatcher the
// worker redelivers; without one the send runs inline on the caller's
// request. The returned error reports lookup and store problems only — an
// inline attempt's send failure is the delivery log's business, reflected
// in the returned row's status and last_error.
func (s *Service) RetryMessage(ctx context.Context, id string) (Message, error) {
	if s.messages == nil {
		return Message{}, kernel.ErrNotFound
	}
	msg, err := s.messages.GetEmailMessage(ctx, id)
	if err != nil {
		return Message{}, err
	}
	if msg.Status == StatusSent {
		return Message{}, kernel.ErrConflict
	}
	if err := s.messages.UpdateEmailMessageStatus(ctx, id, StatusQueued, msg.Attempts, "", nil); err != nil {
		return Message{}, err
	}
	msg.Status = StatusQueued
	if s.dispatcher != nil {
		s.dispatcher.Dispatch(ctx, msg)
		return msg, nil
	}
	// Inline mode: attempt now and answer with the row as it now stands.
	_ = s.attempt(ctx, msg)
	if updated, err := s.messages.GetEmailMessage(ctx, id); err == nil {
		return updated, nil
	}
	return msg, nil
}

// Messages lists the delivery log for the admin surface.
func (s *Service) Messages(ctx context.Context, status string, limit, offset int) ([]Message, error) {
	if s.messages == nil {
		return nil, nil
	}
	return s.messages.ListEmailMessages(ctx, status, limit, offset)
}

// GetMessage returns one delivery-log entry.
func (s *Service) GetMessage(ctx context.Context, id string) (Message, error) {
	if s.messages == nil {
		return Message{}, kernel.ErrNotFound
	}
	return s.messages.GetEmailMessage(ctx, id)
}

// PruneMessages deletes delivery-log rows older than the cutoff.
func (s *Service) PruneMessages(ctx context.Context, olderThan time.Time) (int64, error) {
	if s.messages == nil {
		return 0, nil
	}
	return s.messages.PruneEmailMessages(ctx, olderThan)
}
