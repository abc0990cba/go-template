package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/tickets"
)

// demoTicketTypeSpec is one seeded ticket type with its approval chain.
type demoTicketTypeSpec struct {
	key, name, description string
	steps                  []tickets.TicketStep
}

// demoTicketTypes exercise multi-step chains over both approving roles:
// the org lead holds manager, approver roles go to half the members.
var demoTicketTypes = []demoTicketTypeSpec{
	{"expense", "Expense report", "Reimburse travel, training, or equipment costs", []tickets.TicketStep{
		{Name: "Manager review", ApproverRole: "manager"},
		{Name: "Finance approval", ApproverRole: "approver"},
	}},
	{"access_request", "Access request", "Grant or extend access to a system", []tickets.TicketStep{
		{Name: "Access approval", ApproverRole: "approver"},
	}},
	{"vacation", "Vacation request", "Book paid time off", []tickets.TicketStep{
		{Name: "Manager review", ApproverRole: "manager"},
		{Name: "HR approval", ApproverRole: "approver"},
	}},
}

// demoTicketTitles pools realistic subjects per type.
var demoTicketTitles = map[string][]string{
	"expense": {
		"Team offsite travel reimbursement",
		"Kubernetes certification exam fee",
		"Home office monitor purchase",
		"Client visit mileage expenses",
		"Conference tickets for the platform team",
	},
	"access_request": {
		"Production database read access",
		"CI/CD admin rights for the release week",
		"Analytics warehouse access for the data team",
		"On-call PagerDuty schedule access",
		"VPN split-tunnel exception",
	},
	"vacation": {
		"Annual leave in late October",
		"Long weekend away",
		"Family trip abroad",
		"Study leave for the exam week",
		"Winter holiday block",
	},
}

var demoTicketPriorities = []struct {
	name  string
	share int
}{{"low", 25}, {"medium", 45}, {"high", 20}, {"urgent", 10}}

var demoTicketComments = []string{
	"Uploaded the receipts, please take a look.",
	"Could you clarify the cost center for this?",
	"Approved on my side — moving it forward.",
	"This exceeds the quarterly budget line; can we split it?",
	"Thanks for the quick turnaround!",
	"Blocking until the vendor invoice arrives.",
	"Details updated as discussed on the call.",
	"Reminding everyone this needs two approvals.",
}

var demoTicketRejections = []string{
	"Missing itemized receipts; please resubmit with attachments.",
	"Overlaps with the existing subscription — consolidate first.",
	"Not within the current budget cycle, retry next quarter.",
	"Policy requires the manager's sign-off before this step.",
}

// seedDemoTicketTypes writes the platform-wide ticket types.
func seedDemoTicketTypes(ctx context.Context, repos Repos, now time.Time) error {
	for _, spec := range demoTicketTypes {
		typ := tickets.TicketType{
			ID:          fmt.Sprintf("%stt-%s", demoIDPrefix, spec.key),
			Key:         spec.key,
			Name:        spec.name,
			Description: spec.description,
			Steps:       spec.steps,
			Active:      true,
			CreatedBy:   demoIDPrefix + "u-0001",
			CreatedAt:   now.Add(-60 * 24 * time.Hour),
			UpdatedAt:   now.Add(-60 * 24 * time.Hour),
		}
		if err := repos.Tickets.CreateTicketType(ctx, typ); err != nil && !errors.Is(err, kernel.ErrConflict) {
			return err
		}
	}
	return nil
}

// seedDemoTickets writes tickets across the full state machine: consistent
// approval steps for the status (pending at the current step, skipped after
// a rejection, all approved on the happy path), a timeline per ticket, and
// a discussion on some of them. Most tickets land in the demo orgs; the
// rest live in the platform domain where the seeded admins act.
func seedDemoTickets(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time, orgs []demoOrgContext) error {
	// The platform cast mirrors the documented demo accounts: u-0004/u-0006
	// file tickets, u-0002 decides the manager steps, u-0005 (+ the admin
	// u-0001) the approver steps — every loginable account owns a slice of
	// the flow.
	platform := demoOrgContext{
		id:        rbac.PlatformDomain,
		members:   []string{demoIDPrefix + "u-0004", demoIDPrefix + "u-0005", demoIDPrefix + "u-0006", demoIDPrefix + "u-0001"},
		approvers: []string{demoIDPrefix + "u-0005", demoIDPrefix + "u-0001"},
		managers:  []string{demoIDPrefix + "u-0002"},
	}
	for i := 1; i <= demoTicketN; i++ {
		spec := demoTicketTypes[rng.Intn(len(demoTicketTypes))]
		status := pickWeighted(rng, demoTicketStatuses)
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoTraceSpan))))
		cast := platform
		if len(orgs) > 0 && rng.Intn(5) != 0 {
			cast = orgs[rng.Intn(len(orgs))]
		}

		ticket := tickets.Ticket{
			ID:          demoIDPrefix + "t-" + pad4(i),
			OrgID:       cast.id,
			TypeID:      fmt.Sprintf("%stt-%s", demoIDPrefix, spec.key),
			TypeKey:     spec.key,
			Title:       pick(rng, demoTicketTitles[spec.key]),
			Description: "Seeded demo ticket for the " + spec.name + " flow.",
			Priority:    pickWeighted(rng, demoTicketPriorities),
			Status:      tickets.TicketDraft,
			CreatedBy:   pick(rng, cast.members),
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}
		// Org leads occasionally file their own, so the manager logins have
		// a "my tickets" history too.
		if len(cast.managers) > 0 && rng.Intn(4) == 0 {
			ticket.CreatedBy = cast.managers[0]
		}
		number, err := repos.Tickets.CreateTicket(ctx, ticket)
		if err != nil {
			return err
		}
		ticket.Number = number

		submittedAt := createdAt.Add(time.Duration(rng.Int63n(int64(2 * time.Hour))))
		actor := func(role string) string { return cast.decideAs(rng, role) }
		steps := make([]tickets.TicketApproval, 0, len(spec.steps))
		for idx, step := range spec.steps {
			steps = append(steps, tickets.TicketApproval{
				ID:       fmt.Sprintf("%sta-%04d-%02d", demoIDPrefix, i, idx),
				TicketID: ticket.ID, StepIndex: idx, Name: step.Name,
				ApproverRole: step.ApproverRole, Status: tickets.ApprovalPending, CreatedAt: submittedAt,
			})
		}
		advance := func(idx int, status, comment string, at time.Time) {
			steps[idx].Status = status
			steps[idx].DecidedBy = actor(steps[idx].ApproverRole)
			steps[idx].DecidedAt = &at
			steps[idx].Comment = comment
		}

		events := []tickets.TicketEvent{
			{ID: fmt.Sprintf("%ste-%04d-00", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "created", Detail: ticket.Title, CreatedAt: createdAt},
		}

		switch status {
		case tickets.TicketDraft:
			ticket.UpdatedAt = createdAt
		case tickets.TicketInReview:
			ticket.Status = tickets.TicketInReview
			ticket.UpdatedAt = submittedAt
			events = append(events, tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-01", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "submitted", CreatedAt: submittedAt})
			if rng.Intn(2) == 1 && len(steps) > 1 {
				// Mid-chain: the first step already approved.
				decidedAt := submittedAt.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
				advance(0, tickets.ApprovalApproved, "Looks fine to me.", decidedAt)
				ticket.CurrentStep = 1
				events = append(events, tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-02", demoIDPrefix, i), TicketID: ticket.ID, ActorID: steps[0].DecidedBy, Action: "step_approved", Detail: steps[0].Name, CreatedAt: decidedAt})
			}
		case tickets.TicketApproved, tickets.TicketCompleted:
			ticket.Status = status
			at := submittedAt
			for idx := range steps {
				at = at.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
				advance(idx, tickets.ApprovalApproved, pick(rng, []string{"", "Approved.", "All good.", ""}), at)
				events = append(events, tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-%02d", demoIDPrefix, i, idx+1), TicketID: ticket.ID, ActorID: steps[idx].DecidedBy, Action: "approved", Detail: steps[idx].Name, CreatedAt: at})
			}
			ticket.DecidedBy = steps[len(steps)-1].DecidedBy
			ticket.DecidedAt = &at
			ticket.UpdatedAt = at
			if status == tickets.TicketCompleted {
				completedAt := at.Add(time.Duration(rng.Int63n(int64(72 * time.Hour))))
				ticket.UpdatedAt = completedAt
				events = append(events, tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-99", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "completed", Detail: "result delivered by the requester", CreatedAt: completedAt})
			}
		case tickets.TicketRejected:
			ticket.Status = tickets.TicketRejected
			decidedAt := submittedAt.Add(time.Duration(rng.Int63n(int64(72 * time.Hour))))
			stepIdx := rng.Intn(len(steps))
			for idx := 0; idx < stepIdx; idx++ {
				advance(idx, tickets.ApprovalApproved, "", submittedAt.Add(time.Duration(rng.Int63n(int64(24*time.Hour)))))
			}
			advance(stepIdx, tickets.ApprovalRejected, pick(rng, demoTicketRejections), decidedAt)
			for idx := stepIdx + 1; idx < len(steps); idx++ {
				steps[idx].Status = tickets.ApprovalSkipped
			}
			ticket.CurrentStep = stepIdx
			ticket.DecidedBy = steps[stepIdx].DecidedBy
			ticket.DecidedAt = &decidedAt
			ticket.UpdatedAt = decidedAt
			events = append(events,
				tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-01", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "submitted", CreatedAt: submittedAt},
				tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-90", demoIDPrefix, i), TicketID: ticket.ID, ActorID: steps[stepIdx].DecidedBy, Action: "rejected", Detail: steps[stepIdx].Comment, CreatedAt: decidedAt},
			)
		case tickets.TicketWithdrawn:
			ticket.Status = tickets.TicketWithdrawn
			withdrawnAt := submittedAt.Add(time.Duration(rng.Int63n(int64(24 * time.Hour))))
			for idx := range steps {
				steps[idx].Status = tickets.ApprovalSkipped
			}
			ticket.DecidedBy = ticket.CreatedBy
			ticket.DecidedAt = &withdrawnAt
			ticket.UpdatedAt = withdrawnAt
			events = append(events,
				tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-01", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "submitted", CreatedAt: submittedAt},
				tickets.TicketEvent{ID: fmt.Sprintf("%ste-%04d-95", demoIDPrefix, i), TicketID: ticket.ID, ActorID: ticket.CreatedBy, Action: "withdrawn", Detail: "withdrawn by the requester", CreatedAt: withdrawnAt},
			)
		}

		if err := repos.Tickets.ReplaceApprovals(ctx, ticket.ID, steps); err != nil {
			return err
		}
		if err := repos.Tickets.UpdateTicket(ctx, ticket); err != nil {
			return err
		}
		for _, event := range events {
			if err := repos.Tickets.AddTicketEvent(ctx, event); err != nil {
				return err
			}
		}
		// A discussion on roughly half of the tickets.
		for n := rng.Intn(3); n > 0; n-- {
			author := ticket.CreatedBy
			if rng.Intn(2) == 1 {
				author = actor(pick(rng, []string{"manager", "approver"}))
			}
			commentAt := createdAt.Add(time.Duration(rng.Int63n(int64(72 * time.Hour))))
			comment := tickets.TicketComment{
				ID:       fmt.Sprintf("%stc-%04d-%02d", demoIDPrefix, i, n),
				TicketID: ticket.ID, AuthorID: author,
				Body: pick(rng, demoTicketComments), CreatedAt: commentAt,
			}
			if err := repos.Tickets.AddTicketComment(ctx, comment); err != nil {
				return err
			}
		}
	}
	return nil
}
