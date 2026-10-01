package tickets

// The state machine: every action is mapped to the statuses it may leave,
// and Action applies one transition with its authorship and step-role
// guards.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

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
	TicketActionSubmit:   {TicketDraft},
	TicketActionApprove:  {TicketInReview},
	TicketActionReject:   {TicketInReview},
	TicketActionWithdraw: {TicketDraft, TicketInReview, TicketApproved},
	TicketActionComplete: {TicketApproved},
	TicketActionReopen:   {TicketRejected},
}

// Action applies one state-machine transition with its guards. Visibility
// comes first: an outsider learns nothing about a foreign ticket, not even
// whether the action would have been a valid transition.
func (s *Service) Action(ctx context.Context, actor Actor, ticketID, action, comment string) (Ticket, error) {
	ticket, err := s.store.GetTicket(ctx, ticketID)
	if err != nil {
		return Ticket{}, err
	}
	approvals, err := s.store.ListApprovals(ctx, ticket.ID)
	if err != nil {
		return Ticket{}, err
	}
	if !s.canView(actor, ticket, approvals) {
		return Ticket{}, kernel.ErrForbidden
	}
	from, known := ticketActions[action]
	if !known {
		return Ticket{}, kernel.Invalid("unknown action " + action)
	}
	if !slices.Contains(from, ticket.Status) {
		return Ticket{}, fmt.Errorf("%w: cannot %s a %s ticket", ErrInvalidTransition, action, ticket.Status)
	}

	switch action {
	case TicketActionSubmit:
		if ticket.CreatedBy != actor.ID {
			return Ticket{}, kernel.ErrForbidden
		}
		typ, err := s.store.GetTicketType(ctx, ticket.TypeID)
		if err != nil {
			return Ticket{}, err
		}
		if len(typ.Steps) == 0 || !typ.Active {
			return Ticket{}, kernel.Invalid("ticket type has no active approval chain")
		}
		now := time.Now()
		steps := make([]TicketApproval, 0, len(typ.Steps))
		for i, step := range typ.Steps {
			steps = append(steps, TicketApproval{
				ID: kernel.NewID(), TicketID: ticket.ID, StepIndex: i,
				Name: step.Name, ApproverRole: step.ApproverRole,
				Status: ApprovalPending, CreatedAt: now,
			})
		}
		if err := s.store.ReplaceApprovals(ctx, ticket.ID, steps); err != nil {
			return Ticket{}, err
		}
		ticket.Status = TicketInReview
		ticket.CurrentStep = 0
		s.record(ctx, ticket.ID, actor.ID, "submitted", "")
		s.notifyStepApprovers(ctx, actor, ticket, steps[0])
		s.emit(ctx, "ticket_submitted", actor.ID)

	case TicketActionApprove, TicketActionReject:
		if !s.canDecide(actor, ticket, approvals) {
			return Ticket{}, kernel.ErrForbidden
		}
		if action == TicketActionReject && comment == "" {
			return Ticket{}, kernel.Invalid("a rejection requires a comment")
		}
		now := time.Now()
		current := approvals[ticket.CurrentStep]
		current.Status = ApprovalApproved
		if action == TicketActionReject {
			current.Status = ApprovalRejected
		}
		current.DecidedBy = actor.ID
		current.DecidedAt = &now
		current.Comment = comment
		if err := s.store.UpdateApproval(ctx, current); err != nil {
			return Ticket{}, err
		}
		if action == TicketActionReject {
			// A refusal ends the flow: later steps never happen.
			for _, step := range approvals {
				if step.StepIndex > ticket.CurrentStep && step.Status == ApprovalPending {
					step.Status = ApprovalSkipped
					if err := s.store.UpdateApproval(ctx, step); err != nil {
						return Ticket{}, err
					}
				}
			}
			ticket.Status = TicketRejected
			ticket.DecidedBy = actor.ID
			ticket.DecidedAt = &now
			s.record(ctx, ticket.ID, actor.ID, "rejected", comment)
			s.notifyRequester(ctx, ticket, "ticket.rejected",
				"Ticket "+ticketLabel(ticket)+" was rejected", comment)
			s.emit(ctx, "ticket_rejected", actor.ID)
		} else if ticket.CurrentStep == len(approvals)-1 {
			ticket.Status = TicketApproved
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
			return Ticket{}, kernel.ErrForbidden
		}
		s.skipPendingSteps(ctx, ticket)
		now := time.Now()
		ticket.Status = TicketWithdrawn
		ticket.DecidedBy = actor.ID
		ticket.DecidedAt = &now
		s.record(ctx, ticket.ID, actor.ID, "withdrawn", "withdrawn by the requester")
		s.emit(ctx, "ticket_withdrawn", actor.ID)

	case TicketActionComplete:
		if ticket.CreatedBy != actor.ID {
			return Ticket{}, kernel.ErrForbidden
		}
		ticket.Status = TicketCompleted
		s.record(ctx, ticket.ID, actor.ID, "completed", "result delivered by the requester")
		s.emit(ctx, "ticket_completed", actor.ID)

	case TicketActionReopen:
		if ticket.CreatedBy != actor.ID {
			return Ticket{}, kernel.ErrForbidden
		}
		if err := s.store.ReplaceApprovals(ctx, ticket.ID, nil); err != nil {
			return Ticket{}, err
		}
		ticket.Status = TicketDraft
		ticket.CurrentStep = 0
		ticket.DecidedBy = ""
		ticket.DecidedAt = nil
		s.record(ctx, ticket.ID, actor.ID, "reopened", "")
		s.emit(ctx, "ticket_reopened", actor.ID)
	}

	ticket.UpdatedAt = time.Now()
	if err := s.store.UpdateTicket(ctx, ticket); err != nil {
		return Ticket{}, err
	}
	s.notify(ticket)
	return ticket, nil
}

// skipPendingSteps marks every undecided step skipped (withdraw).
func (s *Service) skipPendingSteps(ctx context.Context, ticket Ticket) {
	approvals, err := s.store.ListApprovals(ctx, ticket.ID)
	if err != nil {
		return
	}
	for _, step := range approvals {
		if step.Status == ApprovalPending {
			step.Status = ApprovalSkipped
			_ = s.store.UpdateApproval(ctx, step)
		}
	}
}
