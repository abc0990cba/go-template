package httpapp

// The resumable-upload HTTP flow over the in-memory file store: status,
// initiate, streamed part, complete, then the file is listed and readable
// like any single-shot upload.

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestResumableUploadFlow(t *testing.T) {
	server := newTestServer(t)
	user := registerAndLogin(t, server, "multipart-user@example.com")

	// The capability is advertised with its dev-mode shape.
	status, capability := callJSON(t, server, http.MethodGet, "/api/files/multipart", "", user)
	if status != http.StatusOK || capability["enabled"] != true || capability["streaming"] != true {
		t.Fatalf("capability: %d %v", status, capability)
	}

	// Initiate a two-part session.
	status, session := callJSON(t, server, http.MethodPost, "/api/files/multipart",
		`{"filename":"report.bin","content_type":"application/octet-stream","parts":2}`, user)
	if status != http.StatusCreated {
		t.Fatalf("initiate: want 201, got %d (%v)", status, session)
	}
	key, _ := session["key"].(string)
	uploadID, _ := session["upload_id"].(string)
	if key == "" || uploadID == "" {
		t.Fatalf("session: %v", session)
	}

	// Stream both parts; each answers with its part ETag, which completion
	// echoes back (S3-mode clients get the same header on their presigned
	// part PUTs).
	etags := map[int]string{}
	for part, body := range map[int]string{1: "hello ", 2: "world"} {
		status, headers, raw := rawRequest(t, server, http.MethodPost,
			"/api/files/multipart/parts?upload_id="+uploadID+"&part_number="+itoa(part), body, user, nil)
		if status != http.StatusNoContent {
			t.Fatalf("part %d: want 204, got %d (%s)", part, status, raw)
		}
		etags[part] = headers.Get("ETag")
		if etags[part] == "" {
			t.Fatalf("part %d: missing ETag header", part)
		}
	}

	// Complete: the metadata row appears with the storage-derived size.
	status, file := callJSON(t, server, http.MethodPost, "/api/files/multipart/complete",
		`{"filename":"report.bin","content_type":"application/octet-stream","key":"`+key+
			`","upload_id":"`+uploadID+`","parts":[`+
			`{"part_number":1,"etag":`+strconv.Quote(etags[1])+`},`+
			`{"part_number":2,"etag":`+strconv.Quote(etags[2])+`}]}`, user)
	if status != http.StatusCreated {
		t.Fatalf("complete: want 201, got %d (%v)", status, file)
	}
	fileID, _ := file["id"].(string)
	if size, _ := file["size_bytes"].(float64); size != 11 {
		t.Fatalf("size: want 11, got %v", file["size_bytes"])
	}

	// The completed file downloads like any other attachment.
	status, _, raw := rawRequest(t, server, http.MethodGet, "/api/files/"+fileID+"/content", "", user, nil)
	if status != http.StatusOK || strings.TrimSpace(raw) != "hello world" {
		t.Fatalf("download: %d %q", status, raw)
	}

	// Aborting an unknown session answers 404 problem+json; aborting a
	// live one succeeds.
	status, _ = callJSON(t, server, http.MethodPost, "/api/files/multipart/abort",
		`{"key":"nope","upload_id":"nope"}`, user)
	if status != http.StatusNotFound {
		t.Fatalf("abort unknown: want 404, got %d", status)
	}
	status, session2 := callJSON(t, server, http.MethodPost, "/api/files/multipart",
		`{"filename":"scratch.bin","content_type":"text/plain","parts":2}`, user)
	if status != http.StatusCreated {
		t.Fatalf("initiate 2: %d (%v)", status, session2)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/files/multipart/abort",
		`{"key":"`+session2["key"].(string)+`","upload_id":"`+session2["upload_id"].(string)+`"}`, user)
	if status != http.StatusNoContent {
		t.Fatalf("abort: want 204, got %d", status)
	}
}

// itoa is a tiny int formatter for query strings in this test.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
