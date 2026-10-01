// Package tickets runs the ticket approval state machine: requesters file
// tickets of configurable types and walk them through submit/approve/
// reject/withdraw/complete/reopen, approvers decide step by step through
// their inbox, and every transition leaves a timeline entry, an analytics
// event, and a notification for the opposite side.
package tickets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/notifications"
)

// ErrInvalidTransition rejects actions the ticket's current status does not
// allow; handlers map it to HTTP 409.
var ErrInvalidTransition = errors.New("invalid transition")

// Actor identifies who acts on a ticket and in which Casbin domain: the
// guards authorize authorship and step roles against both.
type Actor struct {
	ID     string
	Domain string
}

// RoleChecker answers the approval guards' role and permission questions.
// The rbac feature's Service satisfies it; the narrow interface keeps tests
// honest.
type RoleChecker interface {
	HasRole(sub, dom, role string) bool
	UsersWithRole(dom, role string) []string
	RolesForUser(sub, dom string) []string
	Enforce(sub, dom, obj, act string) bool
	IsAdmin(sub, dom string) bool
}

// ticketPriorities are the accepted priority values.
var ticketPriorities = []string{"low", "medium", "high", "urgent"}

// Service runs the ticket approval state machine. Requester actions
// (submit, withdraw, complete, reopen) require authorship; approver actions
// (approve, reject) require the current step's Casbin role plus the
// tickets:approve permission. Every transition records a timeline event and
// an analytics event, and notifies the opposite side.
type Service struct {
	store         Store
	rbac          RoleChecker
	notifications *notifications.Service
	events        analytics.Sink
	notifier      func(Ticket)
	logger        *slog.Logger
}

// NewService wires the ticket service.
func NewService(store Store, rbac RoleChecker, events analytics.Sink, logger *slog.Logger) *Service {
	return &Service{store: store, rbac: rbac, events: events, logger: logger}
}

// WithNotifier registers the realtime callback fired on every status change.
func (s *Service) WithNotifier(notify func(Ticket)) *Service {
	s.notifier = notify
	return s
}

// WithNotifications registers the notification service; submit then pings
// the waiting approvers, decisions ping the requester.
func (s *Service) WithNotifications(n *notifications.Service) *Service {
	s.notifications = n
	return s
}

func (s *Service) notify(ticket Ticket) {
	if s.notifier != nil {
		s.notifier(ticket)
	}
}

// emit records an analytics event attributed to the acting user.
func (s *Service) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// record appends one timeline entry to the ticket.
func (s *Service) record(ctx context.Context, ticketID, actorID, action, detail string) {
	if err := s.store.AddTicketEvent(ctx, TicketEvent{
		ID: kernel.NewID(), TicketID: ticketID, ActorID: actorID, Action: action,
		Detail: detail, CreatedAt: time.Now(),
	}); err != nil && s.logger != nil {
		s.logger.Error("record ticket event failed", "err", err)
	}
}

// RecordAttachment appends the timeline entry for an uploaded attachment and
// notifies the ticket's watchers; failures never fail the upload.
func (s *Service) RecordAttachment(ctx context.Context, actorID, ticketID, filename string) {
	s.record(ctx, ticketID, actorID, "attachment_added", filename)
	if ticket, err := s.store.GetTicket(ctx, ticketID); err == nil {
		s.notify(ticket)
	}
}

// VisibleTicketOrg reports the ticket's organization when actorID may see
// it in domain; an error otherwise. The files feature's attachment checks
// consume it through the narrow TicketView interface (primitive-only
// parameters, so the structural fit survives without sharing the Actor
// type across packages).
func (s *Service) VisibleTicketOrg(ctx context.Context, actorID, domain, ticketID string) (string, error) {
	ticket, _, err := s.Get(ctx, Actor{ID: actorID, Domain: domain}, ticketID)
	if err != nil {
		return "", err
	}
	return ticket.OrgID, nil
}

// The files feature's attachment visibility rides on this service through a
// narrow consumer-side interface; prove the structural fit.
var _ files.TicketView = (*Service)(nil)

// --- Tickets ------------------------------------------------------------

// Create files a new draft ticket of the given type in the actor's domain.
func (s *Service) Create(ctx context.Context, actor Actor, typeID, title, description, priority string) (Ticket, error) {
	if title == "" {
		return Ticket{}, kernel.Invalid("title is required")
	}
	if priority == "" {
		priority = "medium"
	}
	if !slices.Contains(ticketPriorities, priority) {
		return Ticket{}, kernel.Invalid("priority must be one of low, medium, high, urgent")
	}
	typ, err := s.store.GetTicketType(ctx, typeID)
	if err != nil {
		return Ticket{}, kernel.Invalid("unknown ticket type")
	}
	if !typ.Active {
		return Ticket{}, kernel.Invalid("ticket type is retired")
	}
	if typ.OrgID != "" && typ.OrgID != actor.Domain {
		return Ticket{}, kernel.ErrForbidden
	}
	now := time.Now()
	ticket := Ticket{
		ID: kernel.NewID(), OrgID: actor.Domain, TypeID: typ.ID, TypeKey: typ.Key,
		Title: title, Description: description, Priority: priority,
		Status: TicketDraft, CreatedBy: actor.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	number, err := s.store.CreateTicket(ctx, ticket)
	if err != nil {
		return Ticket{}, err
	}
	ticket.Number = number
	s.record(ctx, ticket.ID, actor.ID, "created", title)
	s.emit(ctx, "ticket_created", actor.ID)
	s.notify(ticket)
	return ticket, nil
}

// Get returns the ticket with its approval steps when the actor may see it:
// the author, an org-wide reader, or someone involved in the approval.
func (s *Service) Get(ctx context.Context, actor Actor, id string) (Ticket, []TicketApproval, error) {
	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		return Ticket{}, nil, err
	}
	approvals, err := s.store.ListApprovals(ctx, id)
	if err != nil {
		return Ticket{}, nil, err
	}
	if !s.canView(actor, ticket, approvals) {
		return Ticket{}, nil, kernel.ErrForbidden
	}
	return ticket, approvals, nil
}

// List pages tickets. The options' CreatedBy field scopes the listing to
// the requester's own tickets; the handler sets it after checking scope.
func (s *Service) List(ctx context.Context, opts kernel.TicketOptions) ([]Ticket, string, error) {
	return s.store.ListTickets(ctx, opts)
}

// StatusCounts returns the ticket counts per status for the scope.
func (s *Service) StatusCounts(ctx context.Context, orgID, createdBy string) map[string]int64 {
	counts, err := s.store.CountTicketsByStatus(ctx, orgID, createdBy)
	if err != nil {
		return map[string]int64{}
	}
	return counts
}

// Update rewrites the draft: author only, drafts only.
func (s *Service) Update(ctx context.Context, actor Actor, id, title, description, priority string) (Ticket, error) {
	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		return Ticket{}, err
	}
	if ticket.CreatedBy != actor.ID {
		return Ticket{}, kernel.ErrForbidden
	}
	if ticket.Status != TicketDraft {
		return Ticket{}, fmt.Errorf("%w: only drafts are editable", ErrInvalidTransition)
	}
	if title != "" {
		ticket.Title = title
	}
	ticket.Description = description
	if priority != "" {
		if !slices.Contains(ticketPriorities, priority) {
			return Ticket{}, kernel.Invalid("priority must be one of low, medium, high, urgent")
		}
		ticket.Priority = priority
	}
	ticket.UpdatedAt = time.Now()
	if err := s.store.UpdateTicket(ctx, ticket); err != nil {
		return Ticket{}, err
	}
	s.notify(ticket)
	return ticket, nil
}

// Delete removes a draft: author only, drafts only.
func (s *Service) Delete(ctx context.Context, actor Actor, id string) error {
	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		return err
	}
	if ticket.CreatedBy != actor.ID {
		return kernel.ErrForbidden
	}
	if ticket.Status != TicketDraft {
		return fmt.Errorf("%w: only drafts can be deleted", ErrInvalidTransition)
	}
	return s.store.DeleteTicket(ctx, id)
}
