package tickets

import (
	"context"

	"github.com/your-org/go-template/internal/kernel"
)

// Store persists ticket types, tickets, approval steps, comments, and
// timeline events. Listings are scope-aware: the caller narrows by creator
// (requester's own tickets), organization, or the approver's roles.
type Store interface {
	CreateTicketType(ctx context.Context, typ TicketType) error
	GetTicketType(ctx context.Context, id string) (TicketType, error)
	GetTicketTypeByKey(ctx context.Context, orgID, key string) (TicketType, error)
	UpdateTicketType(ctx context.Context, typ TicketType) error
	DeleteTicketType(ctx context.Context, id string) error
	ListTicketTypes(ctx context.Context, orgID string) ([]TicketType, error)

	// CreateTicket stores the ticket and assigns its per-organization
	// sequence number, which is returned.
	CreateTicket(ctx context.Context, ticket Ticket) (int, error)
	GetTicket(ctx context.Context, id string) (Ticket, error)
	UpdateTicket(ctx context.Context, ticket Ticket) error
	DeleteTicket(ctx context.Context, id string) error
	ListTickets(ctx context.Context, opts kernel.TicketOptions) ([]Ticket, string, error)
	CountTicketsByStatus(ctx context.Context, orgID, createdBy string) (map[string]int64, error)

	// ReplaceApprovals rewrites the ticket's approval steps (submit, reopen).
	ReplaceApprovals(ctx context.Context, ticketID string, steps []TicketApproval) error
	ListApprovals(ctx context.Context, ticketID string) ([]TicketApproval, error)
	UpdateApproval(ctx context.Context, approval TicketApproval) error
	// ListTicketsForApprover pages tickets whose current pending step
	// matches one of the given roles.
	ListTicketsForApprover(ctx context.Context, orgID string, roles []string, opts kernel.ListOptions) ([]Ticket, string, error)
	CountTicketsForApprover(ctx context.Context, orgID string, roles []string) (int64, error)
	// ListDecidedApprovals returns the user's past decisions with their
	// tickets, newest first.
	ListDecidedApprovals(ctx context.Context, userID string, limit, offset int) ([]TicketDecision, error)

	AddTicketComment(ctx context.Context, comment TicketComment) error
	ListTicketComments(ctx context.Context, ticketID string, limit, offset int) ([]TicketComment, error)

	AddTicketEvent(ctx context.Context, event TicketEvent) error
	ListTicketEvents(ctx context.Context, ticketID string, limit, offset int) ([]TicketEvent, error)
}
