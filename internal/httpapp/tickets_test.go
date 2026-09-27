// Tickets: the requester and approver lifecycles, the distinct terminal
// outcomes (withdrawn vs rejected), and the interface separation.
package httpapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// newTicketTestServer builds the test server and hands back the container
// so tests can grant the requester/approver roles directly.
func newTicketTestServer(t *testing.T) (*httptest.Server, *service.Container) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)
	return server, services
}

// registerUser registers an account and grants the role in the platform
// domain (the first account is already superadmin).
func registerUser(t *testing.T, server *httptest.Server, services *service.Container, email, role string) string {
	t.Helper()
	token := registerAndLogin(t, server, email)
	user, err := services.Auth.UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("lookup %s: %v", email, err)
	}
	if role != "" {
		if err := services.RBAC.AssignRole(user.ID, role, service.PlatformDomain); err != nil {
			t.Fatalf("assign %s to %s: %v", role, email, err)
		}
	}
	return token
}

// createApprovalType makes a single-step ticket type approved by the given role.
func createApprovalType(t *testing.T, server *httptest.Server, adminToken, key, role string) string {
	t.Helper()
	body := fmt.Sprintf(`{"key":%q,"name":%q,"steps":[{"name":"Review","approver_role":%q}]}`, key, key, role)
	status, bodyMap := callJSON(t, server, http.MethodPost, "/api/ticket-types", body, adminToken)
	if status != http.StatusCreated {
		t.Fatalf("create ticket type: want 201, got %d (%v)", status, bodyMap)
	}
	id, _ := bodyMap["id"].(string)
	if id == "" {
		t.Fatal("create ticket type: want non-empty id")
	}
	return id
}

func createTicket(t *testing.T, server *httptest.Server, token, typeID string) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"type_id":%q,"title":"License renewal","description":"Renew the CI license","priority":"high"}`, typeID)
	status, parsed := callJSON(t, server, http.MethodPost, "/api/tickets", body, token)
	if status != http.StatusCreated {
		t.Fatalf("create ticket: want 201, got %d (%v)", status, parsed)
	}
	return parsed
}

func ticketAction(t *testing.T, server *httptest.Server, token, ticketID, action, comment string) (int, map[string]any) {
	t.Helper()
	body := fmt.Sprintf(`{"action":%q,"comment":%q}`, action, comment)
	return callJSON(t, server, http.MethodPost, "/api/tickets/"+ticketID+"/actions", body, token)
}

func dataLen(parsed map[string]any) int {
	data, _ := parsed["data"].([]any)
	return len(data)
}

func TestTicketRequesterAndApproverLifecycle(t *testing.T) {
	server, services := newTicketTestServer(t)
	adminToken := registerAndLogin(t, server, "admin@example.com") // first user: superadmin
	requesterToken := registerUser(t, server, services, "requester@example.com", "requester")
	approverToken := registerUser(t, server, services, "approver@example.com", "approver")

	typeID := createApprovalType(t, server, adminToken, "expense", "approver")

	// The requester files and submits; the approver queue picks it up.
	ticket := createTicket(t, server, requesterToken, typeID)
	ticketID, _ := ticket["id"].(string)
	if number, _ := ticket["number"].(float64); number != 1 {
		t.Errorf("first ticket number: want 1, got %v", ticket["number"])
	}
	if status, _ := ticket["status"].(string); status != "draft" {
		t.Fatalf("new ticket status: want draft, got %v", ticket["status"])
	}

	if status, _ := callJSON(t, server, http.MethodPatch, "/api/tickets/"+ticketID,
		`{"title":"License renewal, updated","priority":"urgent"}`, requesterToken); status != http.StatusOK {
		t.Fatalf("edit draft: want 200, got %d", status)
	}

	// Approving a draft is an invalid transition, not a decision. An
	// outsider who cannot even see the draft gets 403 instead.
	if status, _ := ticketAction(t, server, adminToken, ticketID, "approve", ""); status != http.StatusConflict {
		t.Errorf("approve draft: want 409, got %d", status)
	}

	if status, parsed := ticketAction(t, server, requesterToken, ticketID, "submit", ""); status != http.StatusOK || parsed["status"] != "in_review" {
		t.Fatalf("submit: want 200 in_review, got %d (%v)", status, parsed["status"])
	}

	// The requester cannot decide on their own ticket's approval step.
	if status, _ := ticketAction(t, server, requesterToken, ticketID, "approve", ""); status != http.StatusForbidden {
		t.Errorf("requester approve: want 403, got %d", status)
	}

	// The inbox is role-scoped: the approver sees the ticket, the requester's queue is empty.
	status, inbox := callJSON(t, server, http.MethodGet, "/api/approvals", "", approverToken)
	if status != http.StatusOK || dataLen(inbox) != 1 {
		t.Fatalf("approver inbox: want 200 with 1 ticket, got %d (%v)", status, dataLen(inbox))
	}
	if _, inbox2 := callJSON(t, server, http.MethodGet, "/api/approvals", "", requesterToken); dataLen(inbox2) != 0 {
		t.Errorf("requester inbox: want empty, got %d", dataLen(inbox2))
	}

	// Rejections need a comment.
	if status, _ := ticketAction(t, server, approverToken, ticketID, "reject", ""); status != http.StatusBadRequest {
		t.Errorf("reject without comment: want 400, got %d", status)
	}

	// Approve ends the flow; the requester delivers the result.
	if status, parsed := ticketAction(t, server, approverToken, ticketID, "approve", "License count verified"); status != http.StatusOK || parsed["status"] != "approved" {
		t.Fatalf("approve: want 200 approved, got %d (%v)", status, parsed["status"])
	}
	if status, detail := callJSON(t, server, http.MethodGet, "/api/tickets/"+ticketID, "", approverToken); status != http.StatusOK || detail["ticket"] == nil {
		t.Fatalf("get ticket: want 200 with ticket payload, got %d (%v)", status, detail)
	}
	if status, parsed := ticketAction(t, server, requesterToken, ticketID, "complete", ""); status != http.StatusOK || parsed["status"] != "completed" {
		t.Fatalf("complete: want 200 completed, got %d (%v)", status, parsed["status"])
	}

	// The decision history shows the approver's step.
	if _, decided := callJSON(t, server, http.MethodGet, "/api/approvals?state=decided", "", approverToken); dataLen(decided) != 1 {
		t.Errorf("decided history: want 1 decision, got %d", dataLen(decided))
	}
}

func TestTicketWithdrawnDistinctFromRejected(t *testing.T) {
	server, services := newTicketTestServer(t)
	adminToken := registerAndLogin(t, server, "admin@example.com")
	requesterToken := registerUser(t, server, services, "author@example.com", "requester")
	approverToken := registerUser(t, server, services, "decider@example.com", "approver")
	typeID := createApprovalType(t, server, adminToken, "access", "approver")

	// Withdrawn by the requester.
	withdrawn := createTicket(t, server, requesterToken, typeID)
	withdrawnID, _ := withdrawn["id"].(string)
	if status, parsed := ticketAction(t, server, requesterToken, withdrawnID, "submit", ""); status != http.StatusOK {
		t.Fatalf("submit: want 200, got %d (%v)", status, parsed)
	}
	// Only the author can withdraw.
	if status, _ := ticketAction(t, server, approverToken, withdrawnID, "withdraw", ""); status != http.StatusForbidden {
		t.Errorf("non-author withdraw: want 403, got %d", status)
	}
	if status, parsed := ticketAction(t, server, requesterToken, withdrawnID, "withdraw", ""); status != http.StatusOK || parsed["status"] != "withdrawn" {
		t.Fatalf("withdraw: want 200 withdrawn, got %d (%v)", status, parsed["status"])
	}
	// Withdrawn is terminal: no reopen.
	if status, _ := ticketAction(t, server, requesterToken, withdrawnID, "reopen", ""); status != http.StatusConflict {
		t.Errorf("reopen withdrawn: want 409, got %d", status)
	}

	// Rejected by the approver, with the mandatory comment recorded.
	rejected := createTicket(t, server, requesterToken, typeID)
	rejectedID, _ := rejected["id"].(string)
	if status, _ := ticketAction(t, server, requesterToken, rejectedID, "submit", ""); status != http.StatusOK {
		t.Fatalf("submit: want 200, got %d", status)
	}
	if status, parsed := ticketAction(t, server, approverToken, rejectedID, "reject", "Budget line exhausted"); status != http.StatusOK || parsed["status"] != "rejected" {
		t.Fatalf("reject: want 200 rejected, got %d (%v)", status, parsed["status"])
	}
	_, detail := callJSON(t, server, http.MethodGet, "/api/tickets/"+rejectedID, "", approverToken)
	ticket, _ := detail["ticket"].(map[string]any)
	if ticket == nil || ticket["decided_by"] == "" {
		t.Fatal("rejected ticket: want decided_by recorded")
	}

	// The requester can withdraw even an approved ticket — only terminal
	// statuses close the door.
	approved := createTicket(t, server, requesterToken, typeID)
	approvedID, _ := approved["id"].(string)
	if status, _ := ticketAction(t, server, requesterToken, approvedID, "submit", ""); status != http.StatusOK {
		t.Fatalf("submit: want 200, got %d", status)
	}
	if status, _ := ticketAction(t, server, approverToken, approvedID, "approve", ""); status != http.StatusOK {
		t.Fatalf("approve: want 200, got %d", status)
	}
	if status, parsed := ticketAction(t, server, requesterToken, approvedID, "withdraw", ""); status != http.StatusOK || parsed["status"] != "withdrawn" {
		t.Errorf("withdraw approved: want 200 withdrawn, got %d (%v)", status, parsed["status"])
	}

	// A completed ticket was closed by the requester themselves: terminal.
	completed := createTicket(t, server, requesterToken, typeID)
	completedID, _ := completed["id"].(string)
	if status, _ := ticketAction(t, server, requesterToken, completedID, "submit", ""); status != http.StatusOK {
		t.Fatalf("submit: want 200, got %d", status)
	}
	if status, _ := ticketAction(t, server, approverToken, completedID, "approve", ""); status != http.StatusOK {
		t.Fatalf("approve: want 200, got %d", status)
	}
	if status, _ := ticketAction(t, server, requesterToken, completedID, "complete", ""); status != http.StatusOK {
		t.Fatalf("complete: want 200, got %d", status)
	}
	if status, _ := ticketAction(t, server, requesterToken, completedID, "withdraw", ""); status != http.StatusConflict {
		t.Errorf("withdraw completed: want 409, got %d", status)
	}

	// The status counts keep the two terminal outcomes apart: both exist
	// as distinct statuses in the org-wide scope.
	if status, counts := callJSON(t, server, http.MethodGet, "/api/tickets/counts?scope=all", "", adminToken); status != http.StatusOK {
		t.Errorf("counts: want 200, got %d", status)
	} else {
		withdrawn, _ := counts["withdrawn"].(float64)
		rejectedCount, _ := counts["rejected"].(float64)
		if withdrawn < 1 || rejectedCount < 1 {
			t.Errorf("counts: want both withdrawn and rejected present, got %v", counts)
		}
	}

	// A rejected ticket can be reworked and resubmitted.
	if status, parsed := ticketAction(t, server, requesterToken, rejectedID, "reopen", ""); status != http.StatusOK || parsed["status"] != "draft" {
		t.Fatalf("reopen rejected: want 200 draft, got %d (%v)", status, parsed["status"])
	}
	if status, _ := ticketAction(t, server, requesterToken, rejectedID, "submit", ""); status != http.StatusOK {
		t.Errorf("resubmit: want 200, got %d", status)
	}
}

func TestTicketScopeSeparation(t *testing.T) {
	server, services := newTicketTestServer(t)
	adminToken := registerAndLogin(t, server, "admin@example.com")
	ownerToken := registerUser(t, server, services, "owner@example.com", "requester")
	otherToken := registerUser(t, server, services, "other@example.com", "requester")
	approverToken := registerUser(t, server, services, "checker@example.com", "approver")
	typeID := createApprovalType(t, server, adminToken, "vacation", "approver")

	ticket := createTicket(t, server, ownerToken, typeID)
	ticketID, _ := ticket["id"].(string)

	// A foreign requester can neither see nor decide the ticket.
	if status, _ := callJSON(t, server, http.MethodGet, "/api/tickets/"+ticketID, "", otherToken); status != http.StatusForbidden {
		t.Errorf("foreign get: want 403, got %d", status)
	}
	if status, _ := ticketAction(t, server, otherToken, ticketID, "approve", ""); status != http.StatusForbidden {
		t.Errorf("foreign approve: want 403, got %d", status)
	}

	// scope=my shows only own tickets; scope=all needs read_all.
	if status, list := callJSON(t, server, http.MethodGet, "/api/tickets", "", ownerToken); status != http.StatusOK || dataLen(list) != 1 {
		t.Errorf("my list: want 1 ticket, got %d (%v)", dataLen(list), list)
	}
	if _, list := callJSON(t, server, http.MethodGet, "/api/tickets", "", otherToken); dataLen(list) != 0 {
		t.Errorf("other list: want empty, got %d", dataLen(list))
	}
	if status, _ := callJSON(t, server, http.MethodGet, "/api/tickets?scope=all", "", ownerToken); status != http.StatusForbidden {
		t.Errorf("scope=all without read_all: want 403, got %d", status)
	}
	if status, list := callJSON(t, server, http.MethodGet, "/api/tickets?scope=all", "", adminToken); status != http.StatusOK || dataLen(list) != 1 {
		t.Errorf("admin scope=all: want 1 ticket, got %d", dataLen(list))
	}

	// The approver sees the ticket only after submission.
	if status, _ := callJSON(t, server, http.MethodGet, "/api/tickets/"+ticketID, "", approverToken); status != http.StatusForbidden {
		t.Errorf("approver get before submit: want 403, got %d", status)
	}
	if status, _ := ticketAction(t, server, ownerToken, ticketID, "submit", ""); status != http.StatusOK {
		t.Fatalf("submit: want 200, got %d", status)
	}
	if status, _ := callJSON(t, server, http.MethodGet, "/api/tickets/"+ticketID, "", approverToken); status != http.StatusOK {
		t.Errorf("approver get after submit: want 200, got %d", status)
	}

	// Comments: visible to the participants, and anyone who can see the
	// ticket can add one.
	if status, _ := callJSON(t, server, http.MethodPost, "/api/tickets/"+ticketID+"/comments", `{"body":"Please expedite"}`, ownerToken); status != http.StatusCreated {
		t.Errorf("comment: want 201, got %d", status)
	}
	if status, comments := callJSON(t, server, http.MethodGet, "/api/tickets/"+ticketID+"/comments", "", approverToken); status != http.StatusOK || dataLen(comments) != 1 {
		t.Errorf("comments: want 200 with 1 entry, got %d (%v)", status, dataLen(comments))
	}
}
