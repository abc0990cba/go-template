package tickets

// Ticket domain types: the ticket itself, its configurable type carrying
// the approval chain, the approval steps, discussion comments, timeline
// events, and the approver's decision history view.

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Ticket lifecycle statuses. rejected and withdrawn are distinct terminal
// outcomes: rejected was refused by an approver, withdrawn was closed by
// the requester themselves.
const (
	TicketDraft     = "draft"
	TicketInReview  = "in_review"
	TicketApproved  = "approved"
	TicketCompleted = "completed"
	TicketRejected  = "rejected"
	TicketWithdrawn = "withdrawn"
)

// Ticket approval step and decision statuses.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalSkipped  = "skipped"
)

// TicketStep is one approval step of a ticket type: the displayed stage
// name and the Casbin role whose holders may decide it.
type TicketStep struct {
	Name         string `json:"name"`
	ApproverRole string `json:"approver_role"`
}

// TicketType is a configurable kind of ticket carrying its ordered approval
// chain. OrgID "" means the type is available platform-wide.
type TicketType struct {
	ID          string       `json:"id"`
	OrgID       string       `json:"org_id,omitempty"`
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Steps       []TicketStep `json:"steps"`
	Active      bool         `json:"active"`
	CreatedBy   string       `json:"created_by,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Ticket is a request filed by a user and moved through the approval state
// machine. Number is a per-organization human-readable sequence; DecidedBy
// names whoever produced the terminal outcome (an approver for rejected,
// the requester for withdrawn).
type Ticket struct {
	ID          string     `json:"id"`
	Number      int        `json:"number"`
	OrgID       string     `json:"org_id,omitempty"`
	TypeID      string     `json:"type_id"`
	TypeKey     string     `json:"type_key"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	Status      string     `json:"status"`
	CurrentStep int        `json:"current_step"`
	CreatedBy   string     `json:"created_by"`
	DecidedBy   string     `json:"decided_by,omitempty"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TicketApproval is one step of a ticket's approval chain with its decision.
// Steps materialize as pending rows at submit time and are decided in order.
type TicketApproval struct {
	ID           string     `json:"id"`
	TicketID     string     `json:"ticket_id"`
	StepIndex    int        `json:"step_index"`
	Name         string     `json:"name"`
	ApproverRole string     `json:"approver_role"`
	Status       string     `json:"status"`
	DecidedBy    string     `json:"decided_by,omitempty"`
	DecidedAt    *time.Time `json:"decided_at"`
	Comment      string     `json:"comment,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TicketComment is one discussion entry attached to a ticket.
type TicketComment struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketEvent is one timeline entry of a ticket: who did what, when.
type TicketEvent struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketDecision pairs an approval decision with the ticket it belongs to,
// for the approver's decided-history view.
type TicketDecision struct {
	Approval TicketApproval `json:"approval"`
	Ticket   Ticket         `json:"ticket"`
}

// --- Ticket types -------------------------------------------------------

// TicketTypes lists the types available in the domain: platform-wide plus
// the organization's own.
func (s *Service) TicketTypes(ctx context.Context, orgID string) ([]TicketType, error) {
	return s.store.ListTicketTypes(ctx, orgID)
}

// CreateTicketType adds a type after validating the approval chain.
func (s *Service) CreateTicketType(ctx context.Context, actor Actor, typ TicketType) (TicketType, error) {
	typ.OrgID = actor.Domain
	if err := validateTicketType(typ); err != nil {
		return TicketType{}, err
	}
	typ.ID = kernel.NewID()
	typ.CreatedBy = actor.ID
	now := time.Now()
	typ.CreatedAt, typ.UpdatedAt = now, now
	if err := s.store.CreateTicketType(ctx, typ); err != nil {
		return TicketType{}, err
	}
	return typ, nil
}

// UpdateTicketType rewrites display fields, the chain, and the active flag.
func (s *Service) UpdateTicketType(ctx context.Context, id, name, description string, steps []TicketStep, active *bool) (TicketType, error) {
	typ, err := s.store.GetTicketType(ctx, id)
	if err != nil {
		return TicketType{}, err
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
		return TicketType{}, err
	}
	typ.UpdatedAt = time.Now()
	if err := s.store.UpdateTicketType(ctx, typ); err != nil {
		return TicketType{}, err
	}
	return typ, nil
}

// DeleteTicketType removes the type; existing tickets keep their type_key
// snapshot, so history stays readable.
func (s *Service) DeleteTicketType(ctx context.Context, id string) error {
	return s.store.DeleteTicketType(ctx, id)
}

func validateTicketType(typ TicketType) error {
	if typ.Key == "" || typ.Name == "" {
		return kernel.Invalid("type key and name are required")
	}
	if len(typ.Steps) == 0 {
		return kernel.Invalid("a ticket type needs at least one approval step")
	}
	for i, step := range typ.Steps {
		if step.ApproverRole == "" {
			return kernel.Invalid(fmt.Sprintf("step %d needs an approver role", i+1))
		}
	}
	return nil
}
