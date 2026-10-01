// External test package: the memory store (repository) implements
// tickets.Store. The RBAC dependency is the narrow RoleChecker interface,
// so the guards are exercised against a deterministic fake instead of
// Casbin.
package tickets_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/tickets"
)

// fakeRBAC answers the guards' questions from fixed tables.
type fakeRBAC struct {
	roles   map[string][]string
	admins  map[string]bool
	approve map[string]bool
	readAll map[string]bool
}

func (f *fakeRBAC) HasRole(sub, _, role string) bool { return slices.Contains(f.roles[sub], role) }

func (f *fakeRBAC) UsersWithRole(_, role string) []string {
	var out []string
	for user, userRoles := range f.roles {
		if slices.Contains(userRoles, role) {
			out = append(out, user)
		}
	}
	return out
}

func (f *fakeRBAC) RolesForUser(sub, _ string) []string { return f.roles[sub] }

func (f *fakeRBAC) Enforce(sub, _, _, act string) bool {
	switch act {
	case "approve":
		return f.approve[sub]
	case "read_all":
		return f.readAll[sub]
	}
	return false
}

func (f *fakeRBAC) IsAdmin(sub, _ string) bool { return f.admins[sub] }

const (
	domain    = "org-1"
	requester = "req"
	approver  = "app"
	admin     = "adm"
	outsider  = "out"
)

type fixture struct {
	svc   *tickets.Service
	store *repository.MemoryStore
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := repository.NewMemoryStore()
	rbac := &fakeRBAC{
		roles:   map[string][]string{approver: {"approver"}},
		admins:  map[string]bool{admin: true},
		approve: map[string]bool{approver: true, admin: true},
		readAll: map[string]bool{admin: true},
	}
	svc := tickets.NewService(store, rbac, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &fixture{svc: svc, store: store}
}

// twoStepType creates an active type with a two-step approval chain.
func (f *fixture) twoStepType(t *testing.T) tickets.TicketType {
	t.Helper()
	typ, err := f.svc.CreateTicketType(context.Background(),
		tickets.Actor{ID: admin, Domain: domain},
		tickets.TicketType{
			Key:  "expense",
			Name: "Expense",
			Steps: []tickets.TicketStep{
				{Name: "Manager", ApproverRole: "approver"},
				{Name: "Final", ApproverRole: "approver"},
			},
			Active: true,
		})
	if err != nil {
		t.Fatalf("create ticket type: %v", err)
	}
	return typ
}

func (f *fixture) createTicket(t *testing.T, typ tickets.TicketType) tickets.Ticket {
	t.Helper()
	ticket, err := f.svc.Create(context.Background(),
		tickets.Actor{ID: requester, Domain: domain}, typ.ID, "Trip to Berlin", "train tickets", "low")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if ticket.Status != tickets.TicketDraft || ticket.Number == 0 {
		t.Fatalf("new ticket: want draft with number, got %+v", ticket)
	}
	return ticket
}

func TestCreateValidatesInput(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	actor := tickets.Actor{ID: requester, Domain: domain}

	if _, err := f.svc.Create(ctx, actor, typ.ID, "", "", ""); !kernel.IsValidationError(err) {
		t.Errorf("missing title: want validation error, got %v", err)
	}
	if _, err := f.svc.Create(ctx, actor, typ.ID, "x", "", "cosmic"); !kernel.IsValidationError(err) {
		t.Errorf("bad priority: want validation error, got %v", err)
	}
	if _, err := f.svc.Create(ctx, actor, "no-such-type", "x", "", ""); !kernel.IsValidationError(err) {
		t.Errorf("unknown type: want validation error, got %v", err)
	}
}

func TestSubmitRejectsNonAuthorAndInactiveChains(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	actor := tickets.Actor{ID: requester, Domain: domain}

	// Someone else cannot submit someone else's draft.
	if _, err := f.svc.Action(ctx, tickets.Actor{ID: approver, Domain: domain}, ticket.ID, tickets.TicketActionSubmit, ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("non-author submit: want ErrForbidden, got %v", err)
	}

	// A type retired after filing leaves the draft without an active
	// chain: submit is refused instead of stalling forever.
	inactive := false
	if _, err := f.svc.UpdateTicketType(ctx, typ.ID, "", "", nil, &inactive); err != nil {
		t.Fatalf("retire type: %v", err)
	}
	if _, err := f.svc.Action(ctx, actor, ticket.ID, tickets.TicketActionSubmit, ""); !kernel.IsValidationError(err) {
		t.Errorf("submit on retired type: want validation error, got %v", err)
	}
}

func TestMultiStepApprovalChain(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	requesterActor := tickets.Actor{ID: requester, Domain: domain}
	approverActor := tickets.Actor{ID: approver, Domain: domain}

	// A draft is invisible to the approver, so the visibility guard
	// answers before the transition map does.
	if _, err := f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionApprove, ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("approver on draft: want ErrForbidden, got %v", err)
	}
	// The author sees the draft, but approving one is not a transition.
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionApprove, ""); !errors.Is(err, tickets.ErrInvalidTransition) {
		t.Fatalf("approve draft: want ErrInvalidTransition, got %v", err)
	}

	// Submit materializes the chain and moves to step 0.
	ticket, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionSubmit, "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if ticket.Status != tickets.TicketInReview || ticket.CurrentStep != 0 {
		t.Fatalf("submit: want in_review at step 0, got %s/%d", ticket.Status, ticket.CurrentStep)
	}
	approvals, err := f.store.ListApprovals(ctx, ticket.ID)
	if err != nil || len(approvals) != 2 {
		t.Fatalf("approvals after submit: want 2, got %d (%v)", len(approvals), err)
	}
	for _, step := range approvals {
		if step.Status != tickets.ApprovalPending {
			t.Errorf("step %d: want pending, got %s", step.StepIndex, step.Status)
		}
	}

	// The requester has no approve permission.
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionApprove, ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("requester approve: want ErrForbidden, got %v", err)
	}

	// First approval advances to the second step, still in review.
	ticket, err = f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionApprove, "looks fine")
	if err != nil {
		t.Fatalf("approve step 1: %v", err)
	}
	if ticket.Status != tickets.TicketInReview || ticket.CurrentStep != 1 {
		t.Fatalf("after step 1: want in_review at step 1, got %s/%d", ticket.Status, ticket.CurrentStep)
	}

	// Final approval closes the chain.
	ticket, err = f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionApprove, "")
	if err != nil {
		t.Fatalf("approve step 2: %v", err)
	}
	if ticket.Status != tickets.TicketApproved || ticket.DecidedBy != approver {
		t.Fatalf("after step 2: want approved by approver, got %s by %s", ticket.Status, ticket.DecidedBy)
	}
	approvals, _ = f.store.ListApprovals(ctx, ticket.ID)
	if approvals[0].Status != tickets.ApprovalApproved || approvals[1].Status != tickets.ApprovalApproved {
		t.Errorf("both steps must be approved: %+v", approvals)
	}

	// The requester completes the outcome.
	ticket, err = f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionComplete, "")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if ticket.Status != tickets.TicketCompleted {
		t.Errorf("complete: want completed, got %s", ticket.Status)
	}

	// The timeline tells the whole story.
	events, err := f.store.ListTicketEvents(ctx, ticket.ID, 100, 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	actions := make([]string, 0, len(events))
	for _, event := range events {
		actions = append(actions, event.Action)
	}
	for _, want := range []string{"created", "submitted", "step_approved", "approved", "completed"} {
		if !slices.Contains(actions, want) {
			t.Errorf("timeline missing %q: %v", want, actions)
		}
	}

	// Terminal statuses take no more transitions.
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionWithdraw, ""); !errors.Is(err, tickets.ErrInvalidTransition) {
		t.Errorf("withdraw completed: want ErrInvalidTransition, got %v", err)
	}
}

func TestRejectionSkipsLaterStepsAndReopenResets(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	requesterActor := tickets.Actor{ID: requester, Domain: domain}
	approverActor := tickets.Actor{ID: approver, Domain: domain}

	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionSubmit, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// A rejection without a comment is not a decision.
	if _, err := f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionReject, ""); !kernel.IsValidationError(err) {
		t.Fatalf("reject without comment: want validation error, got %v", err)
	}

	ticket, err := f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionReject, "too expensive")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if ticket.Status != tickets.TicketRejected || ticket.DecidedBy != approver {
		t.Fatalf("reject: want rejected by approver, got %s by %s", ticket.Status, ticket.DecidedBy)
	}
	// The undecided second step never happened.
	approvals, _ := f.store.ListApprovals(ctx, ticket.ID)
	if approvals[1].Status != tickets.ApprovalSkipped {
		t.Errorf("later step: want skipped, got %s", approvals[1].Status)
	}

	// Only the author may reopen.
	if _, err := f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionReopen, ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("non-author reopen: want ErrForbidden, got %v", err)
	}
	ticket, err = f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionReopen, "")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if ticket.Status != tickets.TicketDraft || ticket.CurrentStep != 0 || ticket.DecidedBy != "" {
		t.Fatalf("reopen: want clean draft, got %+v", ticket)
	}
	approvals, _ = f.store.ListApprovals(ctx, ticket.ID)
	if len(approvals) != 0 {
		t.Errorf("reopen: want approvals cleared, got %d", len(approvals))
	}
}

func TestWithdrawFromActiveStatesOnlyByAuthor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	requesterActor := tickets.Actor{ID: requester, Domain: domain}
	approverActor := tickets.Actor{ID: approver, Domain: domain}

	// Withdraw works from the draft.
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionWithdraw, ""); err != nil {
		t.Fatalf("withdraw draft: %v", err)
	}
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionSubmit, ""); !errors.Is(err, tickets.ErrInvalidTransition) {
		t.Errorf("submit withdrawn: want ErrInvalidTransition, got %v", err)
	}

	// ...and from in review, where pending steps are skipped.
	ticket = f.createTicket(t, typ)
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionSubmit, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := f.svc.Action(ctx, approverActor, ticket.ID, tickets.TicketActionWithdraw, ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("non-author withdraw: want ErrForbidden, got %v", err)
	}
	ticket, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionWithdraw, "")
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if ticket.Status != tickets.TicketWithdrawn || ticket.DecidedBy != requester {
		t.Fatalf("withdraw: want withdrawn by requester, got %s by %s", ticket.Status, ticket.DecidedBy)
	}
	approvals, _ := f.store.ListApprovals(ctx, ticket.ID)
	for _, step := range approvals {
		if step.Status != tickets.ApprovalSkipped {
			t.Errorf("pending step after withdraw: want skipped, got %s", step.Status)
		}
	}
}

func TestVisibilityAndUnknownAction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	requesterActor := tickets.Actor{ID: requester, Domain: domain}
	approverActor := tickets.Actor{ID: approver, Domain: domain}
	outsiderActor := tickets.Actor{ID: outsider, Domain: domain}

	// The author sees the draft; nobody else does — not even the approver,
	// who is not involved yet.
	if _, _, err := f.svc.Get(ctx, requesterActor, ticket.ID); err != nil {
		t.Errorf("get as author: %v", err)
	}
	for _, actor := range []tickets.Actor{approverActor, outsiderActor} {
		if _, _, err := f.svc.Get(ctx, actor, ticket.ID); !errors.Is(err, kernel.ErrForbidden) {
			t.Errorf("get as %s on draft: want ErrForbidden, got %v", actor.ID, err)
		}
	}

	// After submit the waiting approver is involved and may see it.
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionSubmit, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, _, err := f.svc.Get(ctx, approverActor, ticket.ID); err != nil {
		t.Errorf("get as waiting approver: %v", err)
	}

	// read_all grants org-wide sight without involvement.
	if _, _, err := f.svc.Get(ctx, tickets.Actor{ID: admin, Domain: domain}, ticket.ID); err != nil {
		t.Errorf("get as read_all admin: %v", err)
	}

	// Unknown actions are validation errors, after the visibility guard.
	if _, err := f.svc.Action(ctx, outsiderActor, ticket.ID, "explode", ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("outsider unknown action: want ErrForbidden (visibility first), got %v", err)
	}
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, "explode", ""); !kernel.IsValidationError(err) {
		t.Errorf("unknown action: want validation error, got %v", err)
	}
}

func TestInboxServesCurrentStepRoles(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	requesterActor := tickets.Actor{ID: requester, Domain: domain}
	approverActor := tickets.Actor{ID: approver, Domain: domain}

	// No in-review tickets yet.
	if count := f.svc.InboxCount(ctx, approverActor); count != 0 {
		t.Fatalf("inbox before submit: want 0, got %d", count)
	}
	if _, err := f.svc.Action(ctx, requesterActor, ticket.ID, tickets.TicketActionSubmit, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// The approver's queue holds the waiting ticket; the requester (no
	// roles) sees nothing.
	if count := f.svc.InboxCount(ctx, approverActor); count != 1 {
		t.Fatalf("inbox after submit: want 1, got %d", count)
	}
	inbox, _, err := f.svc.Inbox(ctx, approverActor, kernel.ListOptions{Mode: "offset", Limit: 10})
	if err != nil || len(inbox) != 1 || inbox[0].ID != ticket.ID {
		t.Fatalf("inbox: want the submitted ticket, got %d entries (%v)", len(inbox), err)
	}
	if count := f.svc.InboxCount(ctx, requesterActor); count != 0 {
		t.Errorf("requester inbox: want 0, got %d", count)
	}
}

func TestComments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	typ := f.twoStepType(t)
	ticket := f.createTicket(t, typ)
	requesterActor := tickets.Actor{ID: requester, Domain: domain}
	outsiderActor := tickets.Actor{ID: outsider, Domain: domain}

	if _, err := f.svc.AddComment(ctx, requesterActor, ticket.ID, ""); !kernel.IsValidationError(err) {
		t.Errorf("empty comment: want validation error, got %v", err)
	}
	if _, err := f.svc.AddComment(ctx, outsiderActor, ticket.ID, "sneak peek"); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("outsider comment: want ErrForbidden, got %v", err)
	}
	comment, err := f.svc.AddComment(ctx, requesterActor, ticket.ID, "please review")
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	comments, err := f.svc.Comments(ctx, requesterActor, ticket.ID, 10, 0)
	if err != nil || len(comments) != 1 || comments[0].ID != comment.ID {
		t.Fatalf("comments: want 1 entry, got %d (%v)", len(comments), err)
	}
}
