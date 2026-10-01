package tickets

// The approver's side: the inbox of tickets waiting on the caller's roles,
// the decision history, comments, and timeline events, plus the visibility
// and decision guards every route leans on.

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Inbox pages the tickets waiting on one of the actor's roles at their
// current step — the approver's queue.
func (s *Service) Inbox(ctx context.Context, actor Actor, opts kernel.ListOptions) ([]Ticket, string, error) {
	roles := s.rbac.RolesForUser(actor.ID, actor.Domain)
	if len(roles) == 0 {
		return nil, "", nil
	}
	return s.store.ListTicketsForApprover(ctx, actor.Domain, roles, opts)
}

// InboxCount reports how many tickets currently wait on the actor's roles.
func (s *Service) InboxCount(ctx context.Context, actor Actor) int64 {
	roles := s.rbac.RolesForUser(actor.ID, actor.Domain)
	if len(roles) == 0 {
		return 0
	}
	n, err := s.store.CountTicketsForApprover(ctx, actor.Domain, roles)
	if err != nil {
		return 0
	}
	return n
}

// Decisions lists the approval steps the actor has decided, newest first,
// each paired with its ticket snapshot.
func (s *Service) Decisions(ctx context.Context, actor Actor, limit, offset int) ([]TicketDecision, error) {
	return s.store.ListDecidedApprovals(ctx, actor.ID, limit, offset)
}

// --- Comments and timeline ----------------------------------------------

// AddComment attaches a discussion entry; anyone who can see the ticket can
// comment.
func (s *Service) AddComment(ctx context.Context, actor Actor, ticketID, body string) (TicketComment, error) {
	if body == "" {
		return TicketComment{}, kernel.Invalid("comment body is required")
	}
	if _, _, err := s.Get(ctx, actor, ticketID); err != nil {
		return TicketComment{}, err
	}
	comment := TicketComment{
		ID: kernel.NewID(), TicketID: ticketID, AuthorID: actor.ID, Body: body, CreatedAt: time.Now(),
	}
	if err := s.store.AddTicketComment(ctx, comment); err != nil {
		return TicketComment{}, err
	}
	detail := body
	if len(detail) > 140 {
		detail = detail[:140]
	}
	s.record(ctx, comment.TicketID, actor.ID, "commented", detail)
	s.emit(ctx, "ticket_commented", actor.ID)
	return comment, nil
}

// Comments pages a ticket's discussion newest first.
func (s *Service) Comments(ctx context.Context, actor Actor, ticketID string, limit, offset int) ([]TicketComment, error) {
	if _, _, err := s.Get(ctx, actor, ticketID); err != nil {
		return nil, err
	}
	return s.store.ListTicketComments(ctx, ticketID, limit, offset)
}

// Events pages a ticket's timeline newest first.
func (s *Service) Events(ctx context.Context, actor Actor, ticketID string, limit, offset int) ([]TicketEvent, error) {
	if _, _, err := s.Get(ctx, actor, ticketID); err != nil {
		return nil, err
	}
	return s.store.ListTicketEvents(ctx, ticketID, limit, offset)
}

// --- Guards -------------------------------------------------------------

// canView answers whether the actor may see the ticket: author, org-wide
// reader, the current step's decider, or a past decider.
func (s *Service) canView(actor Actor, ticket Ticket, approvals []TicketApproval) bool {
	if ticket.CreatedBy == actor.ID {
		return true
	}
	if s.rbac.Enforce(actor.ID, actor.Domain, "tickets", "read_all") {
		return true
	}
	for _, step := range approvals {
		if step.DecidedBy == actor.ID {
			return true
		}
	}
	return ticket.Status == TicketInReview && s.canDecide(actor, ticket, approvals)
}

// canDecide answers whether the actor may decide the ticket's current step:
// the step's Casbin role (or an org admin) plus the tickets:approve
// permission.
func (s *Service) canDecide(actor Actor, ticket Ticket, approvals []TicketApproval) bool {
	if ticket.Status != TicketInReview || ticket.CurrentStep >= len(approvals) {
		return false
	}
	current := approvals[ticket.CurrentStep]
	if current.Status != ApprovalPending {
		return false
	}
	if !s.rbac.Enforce(actor.ID, actor.Domain, "tickets", "approve") {
		return false
	}
	return s.rbac.HasRole(actor.ID, actor.Domain, current.ApproverRole) || s.rbac.IsAdmin(actor.ID, actor.Domain)
}

// --- Notifications ------------------------------------------------------

func (s *Service) notifyStepApprovers(ctx context.Context, actor Actor, ticket Ticket, step TicketApproval) {
	if s.notifications == nil {
		return
	}
	for _, userID := range s.rbac.UsersWithRole(actor.Domain, step.ApproverRole) {
		if userID == actor.ID {
			continue
		}
		_ = s.notifications.Create(ctx, userID, "ticket.submitted",
			"Ticket "+ticketLabel(ticket)+" awaits approval",
			ticket.Title, "/approvals")
	}
}

func (s *Service) notifyRequester(ctx context.Context, ticket Ticket, typ, title, body string) {
	if s.notifications == nil || ticket.CreatedBy == "" {
		return
	}
	detail := title
	if body != "" {
		detail = body
	}
	_ = s.notifications.Create(ctx, ticket.CreatedBy, typ, title, detail, "/tickets/"+ticket.ID)
}

// ticketLabel is the human-readable reference used in titles and links.
func ticketLabel(ticket Ticket) string {
	return fmt.Sprintf("TCK-%04d", ticket.Number)
}
