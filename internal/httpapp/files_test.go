package httpapp

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// uploadFile posts a multipart upload and decodes the created metadata.
func uploadFile(t *testing.T, server *httptest.Server, token, filename, content, refType, refID string) (int, map[string]any) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if refType != "" {
		if err := writer.WriteField("ref_type", refType); err != nil {
			t.Fatalf("ref_type field: %v", err)
		}
		if err := writer.WriteField("ref_id", refID); err != nil {
			t.Fatalf("ref_id field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/files", body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil && err.Error() != "EOF" {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, parsed
}

func TestFileUploadDownloadRoundtrip(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "files-owner@example.com")

	// Plain upload (no attachment target).
	status, body := uploadFile(t, server, token, "notes.txt", "hello files", "", "")
	if status != http.StatusCreated {
		t.Fatalf("upload: want 201, got %d (%v)", status, body)
	}
	fileID, _ := body["id"].(string)
	if fileID == "" {
		t.Fatalf("upload must return metadata, got %v", body)
	}
	if checksum, _ := body["checksum_sha256"].(string); checksum == "" {
		t.Fatal("upload must carry the sha256 checksum")
	}

	// Metadata + byte-exact download.
	status, got := callJSON(t, server, http.MethodGet, "/api/files/"+fileID, "", token)
	if status != http.StatusOK || got["filename"] != "notes.txt" {
		t.Fatalf("metadata: %d %v", status, got)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/files/"+fileID+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download: got %d", resp.StatusCode)
	}
	downloaded := new(bytes.Buffer)
	if _, err := downloaded.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	if downloaded.String() != "hello files" {
		t.Fatalf("roundtrip mismatch: %q", downloaded.String())
	}
	if disp := resp.Header.Get("Content-Disposition"); disp == "" {
		t.Fatal("download must carry Content-Disposition")
	}

	// Delete; the file disappears.
	status, _ = callJSON(t, server, http.MethodDelete, "/api/files/"+fileID, "", token)
	if status != http.StatusNoContent {
		t.Fatalf("delete: got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/files/"+fileID, "", token)
	if status != http.StatusNotFound {
		t.Fatalf("deleted file: want 404, got %d", status)
	}
}

func TestTicketAttachmentVisibility(t *testing.T) {
	server := newTestServer(t)
	admin := registerAndLogin(t, server, "attach-admin@example.com")
	outsider := registerAndLogin(t, server, "attach-outsider@example.com")

	// Ticket type + ticket filed by the admin.
	status, typ := callJSON(t, server, http.MethodPost, "/api/ticket-types",
		`{"org_id":"","key":"attach","name":"Attachments","steps":[{"name":"Review","approver_role":"manager"}]}`, admin)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("ticket type: %d (%v)", status, typ)
	}
	typeID, _ := typ["id"].(string)
	status, ticket := callJSON(t, server, http.MethodPost, "/api/tickets",
		`{"type_id":"`+typeID+`","title":"Attachment target","description":"..."}`, admin)
	if status != http.StatusCreated {
		t.Fatalf("ticket: %d (%v)", status, ticket)
	}
	ticketID, _ := ticket["id"].(string)

	// Attach to the ticket.
	status, file := uploadFile(t, server, admin, "evidence.txt", "attached", "ticket", ticketID)
	if status != http.StatusCreated {
		t.Fatalf("attach upload: want 201, got %d (%v)", status, file)
	}

	// The attachments list is hidden from someone who cannot see the ticket.
	status, _ = callJSON(t, server, http.MethodGet, "/api/tickets/"+ticketID+"/attachments", "", outsider)
	if status != http.StatusForbidden && status != http.StatusNotFound {
		t.Fatalf("outsider attachment list: want 403/404, got %d", status)
	}

	// …and the direct download is equally hidden, while the admin passes.
	// /content returns raw bytes, so read statuses without decoding JSON.
	fileID, _ := file["id"].(string)
	hidden := getStatus(t, server, http.MethodGet, "/api/files/"+fileID+"/content", outsider)
	if hidden != http.StatusForbidden && hidden != http.StatusNotFound {
		t.Fatalf("outsider download: want 403/404, got %d", hidden)
	}
	if got := getStatus(t, server, http.MethodGet, "/api/files/"+fileID+"/content", admin); got != http.StatusOK {
		t.Fatalf("owner download: got %d", got)
	}

	// Attaching to a nonexistent ticket is refused.
	status, _ = uploadFile(t, server, admin, "x.txt", "data", "ticket", "no-such-ticket")
	if status != http.StatusNotFound {
		t.Fatalf("attach to unknown ticket: want 404, got %d", status)
	}
}

// getStatus performs a request and returns only the status code (for
// endpoints whose bodies are not JSON).
func getStatus(t *testing.T, server *httptest.Server, method, path, token string) int {
	t.Helper()
	req, _ := http.NewRequest(method, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
