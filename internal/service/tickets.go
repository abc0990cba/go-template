package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// ErrInvalidTransition rejects actions the ticket's current status does not
// allow; handlers map it to HTTP 409.
var ErrInvalidTransition = errors.New("invalid transition")

// Ticket actions — the vocabulary of the state machine.
const (
	TicketActionSubmit   = "submit"
	TicketActionApprove  = "approve"
	TicketActionReject   = "reject"
	TicketActionWithdraw = "withdraw"
	TicketActionComplete = "complete"
	TicketActionReopen   = "reopen"
)

// ticketActions maps every action to the statuses it may leave. Anything
// not listed for the current status is ErrInvalidTransition. Withdraw stays
// with the requester through every active state — draft, in review, even
// after approval: only a terminal status (rejected, withdrawn, completed)
// closes the door, because there the outcome was produced by a decision,
// not by the requester.
var ticketActions = map[string][]string{
	TicketActionSubmit:   {model.TicketDraft},
	TicketActionApprove:  {model.TicketInReview},
	TicketActionReject:   {model.TicketInReview},
	TicketActionWithdraw: {model.TicketDraft, model.TicketInReview, model.TicketApproved},
	TicketActionComplete: {model.TicketApproved},
	TicketActionReopen:   {model.TicketRejected},
}

// ticketPriorities are the accepted priority values.
var ticketPriorities = []string{"low", "medium", "high", "urgent"}

// Actor identifies who acts on a ticket and in which Casbin domain: the
// guards authorize authorship and step roles against both.
type Actor struct {
	ID     string
	Domain string
}

// RoleChecker answers the approval guards' role and permission questions.
// *RBACService implements it; the narrow interface keeps tests honest.
type RoleChecker interface {
	HasRole(sub, dom, role string) bool
	UsersWithRole(dom, role string) []string
	RolesForUser(sub, dom string) []string
	Enforce(sub, dom, obj, act string) bool
	IsAdmin(sub, dom string) bool
}

// TicketsService runs the ticket approval state machine. Requester actions
// (submit, withdraw, complete, reopen) require authorship; approver actions
// (approve, reject) require the current step's Casbin role plus the
// tickets:approve permission. Every transition records a timeline event and
// an analytics event, and notifies the opposite side.
type TicketsService struct {
	store         TicketsRepo
	rbac          RoleChecker
	notifications *NotificationsService
	events        EventSink
	notifier      func(model.Ticket)
	logger        *slog.Logger
}

// WithNotifier registers the realtime callback fired on every status change.
func (s *TicketsService) WithNotifier(notify func(model.Ticket)) *TicketsService {
	s.notifier = notify
	return s
}

// WithNotifications registers the notification service; submit then pings
// the waiting approvers, decisions ping the requester.
func (s *TicketsService) WithNotifications(n *NotificationsService) *TicketsService {
	s.notifications = n
	return s
}

func (s *TicketsService) notify(ticket model.Ticket) {
	if s.notifier != nil {
		s.notifier(ticket)
	}
}

// emit records an analytics event attributed to the acting user.
func (s *TicketsService) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// record appends one timeline entry to the ticket.
func (s *TicketsService) record(ctx context.Context, ticketID, actorID, action, detail string) {
	if err := s.store.AddTicketEvent(ctx, model.TicketEvent{
		ID: newID(), TicketID: ticketID, ActorID: actorID, Action: action,
		Detail: detail, CreatedAt: time.Now(),
	}); err != nil && s.logger != nil {
		s.logger.Error("record ticket event failed", "err", err)
	}
}

// --- Ticket types -------------------------------------------------------

// TicketTypes lists the types available in the domain: platform-wide plus
// the organization's own.
func (s *TicketsService) TicketTypes(ctx context.Context, orgID string) ([]model.TicketType, error) {
	return s.store.ListTicketTypes(ctx, orgID)
}

// CreateTicketType adds a type after validating the approval chain.
func (s *TicketsService) CreateTicketType(ctx context.Context, actor Actor, typ model.TicketType) (model.TicketType, error) {
	typ.OrgID = actor.Domain
	if err := validateTicketType(typ); err != nil {
		return model.TicketType{}, err
	}
	typ.ID = newID()
	typ.CreatedBy = actor.ID
	now := time.Now()
	typ.CreatedAt, typ.UpdatedAt = now, now
	if err := s.store.CreateTicketType(ctx, typ); err != nil {
		return model.TicketType{}, err
	}
	return typ, nil
}

// UpdateTicketType rewrites display fields, the chain, and the active flag.
func (s *TicketsService) UpdateTicketType(ctx context.Context, id, name, description string, steps []model.TicketStep, active *bool) (model.TicketType, error) {
	typ, err := s.store.GetTicketType(ctx, id)
	if err != nil {
		return model.TicketType{}, err
	}
	if name != "" {
		typ.Name = name
	}
	typ.Description = description
	if steps != nil {
		typ.Steps = steps
	}
	if active != nil {
		typ.Active = *active
	}
	if err := validateTicketType(typ); err != nil {
		return model.TicketType{}, err
	}
	typ.UpdatedAt = time.Now()
	if err := s.store.UpdateTicketType(ctx, typ); err != nil {
		return model.TicketType{}, err
	}
	return typ, nil
}

// DeleteTicketType removes the type; existing tickets keep their type_key
// snapshot, so history stays readable.
func (s *TicketsService) DeleteTicketType(ctx context.Context, id string) error {
	return s.store.DeleteTicketType(ctx, id)
}

func validateTicketType(typ model.TicketType) error {
	if typ.Key == "" || typ.Name == "" {
		return validationError("type key and name are required")
	}
	if len(typ.Steps) == 0 {
		return validationError("a ticket type needs at least one approval step")
	}
	for i, step := range typ.Steps {
		if step.ApproverRole == "" {
			return validationError(fmt.Sprintf("step %d needs an approver role", i+1))
		}
	}
	return nil
}

// --- Tickets ------------------------------------------------------------

// Create files a new draft ticket of the given type in the actor's domain.
func (s *TicketsService) Create(ctx context.Context, actor Actor, typeID, title, description, priority string) (model.Ticket, error) {
	if title == "" {
		return model.Ticket{}, validationError("title is required")
	}
	if priority == "" {
		priority = "medium"
	}
	if !slices.Contains(ticketPriorities, priority) {
		return model.Ticket{}, validationError("priority must be one of low, medium, high, urgent")
	}
	typ, err := s.store.GetTicketType(ctx, typeID)
	if err != nil {
		return model.Ticket{}, validationError("unknown ticket type")
	}
	if !typ.Active {
		return model.Ticket{}, validationError("ticket type is retired")
	}
	if typ.OrgID != "" && typ.OrgID != actor.Domain {
		return model.Ticket{}, ErrForbidden
	}
	now := time.Now()
	ticket := model.Ticket{
		ID: newID(), OrgID: actor.Domain, TypeID: typ.ID, TypeKey: typ.Key,
		Title: title, Description: description, Priority: priority,
		Status: model.TicketDraft, CreatedBy: actor.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	number, err := s.store.CreateTicket(ctx, ticket)
	if err != nil {
		return model.Ticket{}, err
	}
	ticket.Number = number
	s.record(ctx, ticket.ID, actor.ID, "created", title)
	s.emit(ctx, "ticket_created", actor.ID)
	s.notify(ticket)
	return ticket, nil
}

// Get returns the ticket with its approval steps when the actor may see it:
// the author, an org-wide reader, or someone involved in the approval.
func (s *TicketsService) Get(ctx context.Context, actor Actor, id string) (model.Ticket, []model.TicketApproval, error) {
	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		return model.Ticket{}, nil, err
	}
	approvals, err := s.store.ListApprovals(ctx, id)
	if err != nil {
		return model.Ticket{}, nil, err
	}
	if !s.canView(actor, ticket, approvals) {
		return model.Ticket{}, nil, ErrForbidden
	}
	return ticket, approvals, nil
}

// List pages tickets. The options' CreatedBy field scopes the listing to
// the requester's own tickets; the handler sets it after checking scope.
func (s *TicketsService) List(ctx context.Context, opts repository.TicketOptions) ([]model.Ticket, string, error) {
	return s.store.ListTickets(ctx, opts)
}

// StatusCounts returns the ticket counts per status for the scope.
func (s *TicketsService) StatusCounts(ctx context.Context, orgID, createdBy string) map[string]int64 {
	counts, err := s.store.CountTicketsByStatus(ctx, orgID, createdBy)
	if err != nil {
		return map[string]int64{}
	}
	return counts
}

// Update rewrites the draft: author only, drafts only.
func (s *TicketsService) Update(ctx context.Context, actor Actor, id, title, description, priority string) (model.Ticket, error) {
	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		return model.Ticket{}, err
	}
	if ticket.CreatedBy != actor.ID {
		return model.Ticket{}, ErrForbidden
	}
	if ticket.Status != model.TicketDraft {
		return model.Ticket{}, fmt.Errorf("%w: only drafts are editable", ErrInvalidTransition)
	}
	if title != "" {
		ticket.Title = title
	}
	ticket.Description = description
	if priority != "" {
		if !slices.Contains(ticketPriorities, priority) {
			return model.Ticket{}, validationError("priority must be one of low, medium, high, urgent")
		}
		ticket.Priority = priority
	}
	ticket.UpdatedAt = time.Now()
	if err := s.store.UpdateTicket(ctx, ticket); err != nil {
		return model.Ticket{}, err
	}
	s.notify(ticket)
	return ticket, nil
}

// Delete removes a draft: author only, drafts only.
func (s *TicketsService) Delete(ctx context.Context, actor Actor, id string) error {
	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		return err
	}
	if ticket.CreatedBy != actor.ID {
		return ErrForbidden
	}
	if ticket.Status != model.TicketDraft {
		return fmt.Errorf("%w: only drafts can be deleted", ErrInvalidTransition)
	}
	return s.store.DeleteTicket(ctx, id)
}

// Action applies one state-machine transition with its guards. Visibility
// comes first: an outsider learns nothing about a foreign ticket, not even
// whether the action would have been a valid transition.
func (s *TicketsService) Action(ctx context.Context, actor Actor, ticketID, action, comment string) (model.Ticket, error) {
	ticket, err := s.store.GetTicket(ctx, ticketID)
	if err != nil {
		return model.Ticket{}, err
	}
	approvals, err := s.store.ListApprovals(ctx, ticket.ID)
	if err != nil {
		return model.Ticket{}, err
	}
	if !s.canView(actor, ticket, approvals) {
		return model.Ticket{}, ErrForbidden
	}
	from, known := ticketActions[action]
	if !known {
		return model.Ticket{}, validationError("unknown action " + action)
	}
	if !slices.Contains(from, ticket.Status) {
		return model.Ticket{}, fmt.Errorf("%w: cannot %s a %s ticket", ErrInvalidTransition, action, ticket.Status)
	}

	switch action {
	case TicketActionSubmit:
		if ticket.CreatedBy != actor.ID {
			return model.Ticket{}, ErrForbidden
		}
		typ, err := s.store.GetTicketType(ctx, ticket.TypeID)
		if err != nil {
			return model.Ticket{}, err
		}
		if len(typ.Steps) == 0 || !typ.Active {
			return model.Ticket{}, validationError("ticket type has no active approval chain")
		}
		now := time.Now()
		steps := make([]model.TicketApproval, 0, len(typ.Steps))
		for i, step := range typ.Steps {
			steps = append(steps, model.TicketApproval{
				ID: newID(), TicketID: ticket.ID, StepIndex: i,
				Name: step.Name, ApproverRole: step.ApproverRole,
				Status: model.ApprovalPending, CreatedAt: now,
			})
		}
		if err := s.store.ReplaceApprovals(ctx, ticket.ID, steps); err != nil {
			return model.Ticket{}, err
		}
		ticket.Status = model.TicketInReview
		ticket.CurrentStep = 0
		s.record(ctx, ticket.ID, actor.ID, "submitted", "")
		s.notifyStepApprovers(ctx, actor, ticket, steps[0])
		s.emit(ctx, "ticket_submitted", actor.ID)

	case TicketActionApprove, TicketActionReject:
		if !s.canDecide(actor, ticket, approvals) {
			return model.Ticket{}, ErrForbidden
		}
		if action == TicketActionReject && comment == "" {
			return model.Ticket{}, validationError("a rejection requires a comment")
		}
		now := time.Now()
		current := approvals[ticket.CurrentStep]
		current.Status = model.ApprovalApproved
		if action == TicketActionReject {
			current.Status = model.ApprovalRejected
		}
		current.DecidedBy = actor.ID
		current.DecidedAt = &now
		current.Comment = comment
		if err := s.store.UpdateApproval(ctx, current); err != nil {
			return model.Ticket{}, err
		}
		if action == TicketActionReject {
			// A refusal ends the flow: later steps never happen.
			for _, step := range approvals {
				if step.StepIndex > ticket.CurrentStep && step.Status == model.ApprovalPending {
					step.Status = model.ApprovalSkipped
					if err := s.store.UpdateApproval(ctx, step); err != nil {
						return model.Ticket{}, err
					}
				}
			}
			ticket.Status = model.TicketRejected
			ticket.DecidedBy = actor.ID
			ticket.DecidedAt = &now
			s.record(ctx, ticket.ID, actor.ID, "rejected", comment)
			s.notifyRequester(ctx, ticket, "ticket.rejected",
				"Ticket "+ticketLabel(ticket)+" was rejected", comment)
			s.emit(ctx, "ticket_rejected", actor.ID)
		} else if ticket.CurrentStep == len(approvals)-1 {
			ticket.Status = model.TicketApproved
			ticket.DecidedBy = actor.ID
			ticket.DecidedAt = &now
			s.record(ctx, ticket.ID, actor.ID, "approved", comment)
			s.notifyRequester(ctx, ticket, "ticket.approved",
				"Ticket "+ticketLabel(ticket)+" was approved", comment)
			s.emit(ctx, "ticket_approved", actor.ID)
		} else {
			ticket.CurrentStep++
			s.record(ctx, ticket.ID, actor.ID, "step_approved", current.Name)
			s.notifyStepApprovers(ctx, actor, ticket, approvals[ticket.CurrentStep])
			s.emit(ctx, "ticket_approved", actor.ID)
		}

	case TicketActionWithdraw:
		if ticket.CreatedBy != actor.ID {
			return model.Ticket{}, ErrForbidden
		}
		s.skipPendingSteps(ctx, ticket)
		now := time.Now()
		ticket.Status = model.TicketWithdrawn
		ticket.DecidedBy = actor.ID
		ticket.DecidedAt = &now
		s.record(ctx, ticket.ID, actor.ID, "withdrawn", "withdrawn by the requester")
		s.emit(ctx, "ticket_withdrawn", actor.ID)

	case TicketActionComplete:
		if ticket.CreatedBy != actor.ID {
			return model.Ticket{}, ErrForbidden
		}
		ticket.Status = model.TicketCompleted
		s.record(ctx, ticket.ID, actor.ID, "completed", "result delivered by the requester")
		s.emit(ctx, "ticket_completed", actor.ID)

	case TicketActionReopen:
		if ticket.CreatedBy != actor.ID {
			return model.Ticket{}, ErrForbidden
		}
		if err := s.store.ReplaceApprovals(ctx, ticket.ID, nil); err != nil {
			return model.Ticket{}, err
		}
		ticket.Status = model.TicketDraft
		ticket.CurrentStep = 0
		ticket.DecidedBy = ""
		ticket.DecidedAt = nil
		s.record(ctx, ticket.ID, actor.ID, "reopened", "")
		s.emit(ctx, "ticket_reopened", actor.ID)
	}

	ticket.UpdatedAt = time.Now()
	if err := s.store.UpdateTicket(ctx, ticket); err != nil {
		return model.Ticket{}, err
	}
	s.notify(ticket)
	return ticket, nil
}

// --- Approver inbox -----------------------------------------------------

// Inbox pages the tickets waiting on one of the actor's roles at their
// current step — the approver's queue.
func (s *TicketsService) Inbox(ctx context.Context, actor Actor, opts repository.ListOptions) ([]model.Ticket, string, error) {
	roles := s.rbac.RolesForUser(actor.ID, actor.Domain)
	if len(roles) == 0 {
		return nil, "", nil
	}
	return s.store.ListTicketsForApprover(ctx, actor.Domain, roles, opts)
}

// InboxCount reports how many tickets currently wait on the actor's roles.
func (s *TicketsService) InboxCount(ctx context.Context, actor Actor) int64 {
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
func (s *TicketsService) Decisions(ctx context.Context, actor Actor, limit, offset int) ([]model.TicketDecision, error) {
	return s.store.ListDecidedApprovals(ctx, actor.ID, limit, offset)
}

// --- Comments -----------------------------------------------------------

// AddComment attaches a discussion entry; anyone who can see the ticket can
// comment.
func (s *TicketsService) AddComment(ctx context.Context, actor Actor, ticketID, body string) (model.TicketComment, error) {
	if body == "" {
		return model.TicketComment{}, validationError("comment body is required")
	}
	if _, _, err := s.Get(ctx, actor, ticketID); err != nil {
		return model.TicketComment{}, err
	}
	comment := model.TicketComment{
		ID: newID(), TicketID: ticketID, AuthorID: actor.ID, Body: body, CreatedAt: time.Now(),
	}
	if err := s.store.AddTicketComment(ctx, comment); err != nil {
		return model.TicketComment{}, err
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
func (s *TicketsService) Comments(ctx context.Context, actor Actor, ticketID string, limit, offset int) ([]model.TicketComment, error) {
	if _, _, err := s.Get(ctx, actor, ticketID); err != nil {
		return nil, err
	}
	return s.store.ListTicketComments(ctx, ticketID, limit, offset)
}

// Events pages a ticket's timeline newest first.
func (s *TicketsService) Events(ctx context.Context, actor Actor, ticketID string, limit, offset int) ([]model.TicketEvent, error) {
	if _, _, err := s.Get(ctx, actor, ticketID); err != nil {
		return nil, err
	}
	return s.store.ListTicketEvents(ctx, ticketID, limit, offset)
}

// --- Guards -------------------------------------------------------------

// canView answers whether the actor may see the ticket: author, org-wide
// reader, the current step's decider, or a past decider.
func (s *TicketsService) canView(actor Actor, ticket model.Ticket, approvals []model.TicketApproval) bool {
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
	return ticket.Status == model.TicketInReview && s.canDecide(actor, ticket, approvals)
}

// canDecide answers whether the actor may decide the ticket's current step:
// the step's Casbin role (or an org admin) plus the tickets:approve
// permission.
func (s *TicketsService) canDecide(actor Actor, ticket model.Ticket, approvals []model.TicketApproval) bool {
	if ticket.Status != model.TicketInReview || ticket.CurrentStep >= len(approvals) {
		return false
	}
	current := approvals[ticket.CurrentStep]
	if current.Status != model.ApprovalPending {
		return false
	}
	if !s.rbac.Enforce(actor.ID, actor.Domain, "tickets", "approve") {
		return false
	}
	return s.rbac.HasRole(actor.ID, actor.Domain, current.ApproverRole) || s.rbac.IsAdmin(actor.ID, actor.Domain)
}

// --- Helpers ------------------------------------------------------------

func (s *TicketsService) skipPendingSteps(ctx context.Context, ticket model.Ticket) {
	approvals, err := s.store.ListApprovals(ctx, ticket.ID)
	if err != nil {
		return
	}
	for _, step := range approvals {
		if step.Status == model.ApprovalPending {
			step.Status = model.ApprovalSkipped
			_ = s.store.UpdateApproval(ctx, step)
		}
	}
}

func (s *TicketsService) notifyStepApprovers(ctx context.Context, actor Actor, ticket model.Ticket, step model.TicketApproval) {
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

func (s *TicketsService) notifyRequester(ctx context.Context, ticket model.Ticket, typ, title, body string) {
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
func ticketLabel(ticket model.Ticket) string {
	return fmt.Sprintf("TCK-%04d", ticket.Number)
}
