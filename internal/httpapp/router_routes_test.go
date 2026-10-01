package httpapp

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestUserManagementAndJobEndpoints(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "ada@example.com")
	// The caller's own account, used for identity-sensitive checks below.
	_, body := callJSON(t, server, http.MethodGet, "/api/me", "", token)

	// GET/PUT/DELETE by id, against a managed user (deleting the caller's
	// own account would cascade the session away with it).
	status, managed := callJSON(t, server, http.MethodPost, "/api/users",
		`{"email":"managed@example.com","name":"Managed"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("create managed user: %d %v", status, managed)
	}
	managedID, _ := managed["id"].(string)
	status, body = callJSON(t, server, http.MethodGet, "/api/users/"+managedID, "", token)
	if status != http.StatusOK || body["id"] != managedID {
		t.Errorf("get user: %d %v", status, body)
	}
	status, body = callJSON(t, server, http.MethodPut, "/api/users/"+managedID,
		`{"email":"renamed@example.com","name":"Renamed"}`, token)
	if status != http.StatusOK || body["name"] != "Renamed" {
		t.Errorf("update user: %d %v", status, body)
	}
	status, _ = callJSON(t, server, http.MethodDelete, "/api/users/"+managedID, "", token)
	if status != http.StatusNoContent {
		t.Errorf("delete user: want 204, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/users/"+managedID, "", token)
	if status != http.StatusNotFound {
		t.Errorf("get deleted user: want 404, got %d", status)
	}

	// Jobs: enqueue enrichment and a report; both accepted and listed.
	status, _ = callJSON(t, server, http.MethodPost, "/api/jobs/enrichment",
		`{"count":3}`, token)
	if status != http.StatusAccepted {
		t.Errorf("enqueue enrichment: want 202, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/reports", "{}", token)
	if status != http.StatusAccepted {
		t.Errorf("generate report: want 202, got %d", status)
	}
	status, body = callJSON(t, server, http.MethodGet, "/api/jobs", "", token)
	if status != http.StatusOK {
		t.Fatalf("list jobs: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 2 {
		t.Errorf("jobs: want 2, got %v", body)
	}

	// Missing user yields 404 problem+json.
	status, body = callJSON(t, server, http.MethodGet, "/api/users/nope", "", token)
	if status != http.StatusNotFound {
		t.Errorf("missing user: want 404, got %d", status)
	}
	if body["title"] != "not found" {
		t.Errorf("problem title: want not found, got %v", body["title"])
	}
}

func TestPaginationModesAcrossCollections(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "pager@example.com")

	// A deterministic population: the registering user plus 7 managed ones,
	// created in ascending order.
	ids := make([]string, 0, 8)
	_, me := callJSON(t, server, http.MethodGet, "/api/me", "", token)
	ids = append(ids, me["id"].(string))
	for i := 1; i <= 7; i++ {
		email := "page-" + strings.Repeat("x", 20) + strconv.Itoa(i) + "@example.com"
		status, body := callJSON(t, server, http.MethodPost, "/api/users",
			`{"email":"`+email+`","name":"Page `+strconv.Itoa(i)+`"}`, token)
		if status != http.StatusCreated {
			t.Fatalf("seed user %d: %d", i, status)
		}
		ids = append(ids, body["id"].(string))
	}

	for _, mode := range []string{"offset", "cursor", "keyset"} {
		seen := 0
		after := ""
		pages := 0
		for seen < 8 && pages < 10 {
			path := "/api/users?mode=" + mode + "&limit=3"
			if after != "" {
				path += "&after=" + after
				if mode == "offset" {
					path = "/api/users?mode=offset&limit=3&offset=" + strconv.Itoa(seen)
				}
			}
			status, body := callJSON(t, server, http.MethodGet, path, "", token)
			if status != http.StatusOK {
				t.Fatalf("%s page %d: %d", mode, pages, status)
			}
			data, _ := body["data"].([]any)
			if len(data) == 0 {
				break
			}
			seen += len(data)
			pages++
			if next, _ := body["next_cursor"].(string); next != "" {
				after = next
			} else {
				break
			}
		}
		if seen != 8 {
			t.Errorf("mode %s: want to page through 8 users, saw %d", mode, seen)
		}
	}

	// Job and report listings answer in every mode too.
	for _, path := range []string{"/api/jobs", "/api/reports", "/api/enrichment-profiles"} {
		for _, mode := range []string{"offset", "cursor", "keyset"} {
			status, _ := callJSON(t, server, http.MethodGet, path+"?mode="+mode+"&limit=1", "", token)
			if status != http.StatusOK {
				t.Errorf("%s mode %s: want 200, got %d", path, mode, status)
			}
		}
	}
}

func TestAnalyticsAndSessionEndpoints(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "analytics@example.com")

	status, body := callJSON(t, server, http.MethodGet, "/api/analytics/summary", "", token)
	if status != http.StatusOK || body["total_users"] == nil {
		t.Errorf("summary: %d %v", status, body)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/analytics/timeseries?days=7", "", token)
	if status != http.StatusOK {
		t.Errorf("timeseries: want 200, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/analytics/events?limit=5", "", token)
	if status != http.StatusOK {
		t.Errorf("recent events: want 200, got %d", status)
	}

	status, body = callJSON(t, server, http.MethodGet, "/api/sessions", "", token)
	if status != http.StatusOK {
		t.Fatalf("sessions: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 1 {
		t.Errorf("sessions: want exactly the current session, got %v", body)
	}

	status, body = callJSON(t, server, http.MethodGet, "/api/oauth/providers", "", "")
	if status != http.StatusOK {
		t.Fatalf("oauth providers: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 2 {
		t.Errorf("oauth providers: want 2 entries, got %v", body)
	}
}
