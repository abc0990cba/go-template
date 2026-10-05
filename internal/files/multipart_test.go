package files

// The resumable-upload flow over the in-memory file store: initiate,
// stream parts, complete into a downloadable file, abort, and the stale
// sweep. S3 uses the same HTTP surface with presigned part URLs; the store
// contract is exercised by the filestore integration tests.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/filestore"
)

type fakeTickets struct{ visible bool }

func (f fakeTickets) VisibleTicketOrg(context.Context, string, string, string) (string, error) {
	if !f.visible {
		return "", kernel.ErrForbidden
	}
	return "org-1", nil
}
func (f fakeTickets) RecordAttachment(context.Context, string, string, string) {}

// memFiles is the metadata store fake (repository imports files, so the
// package's own tests use a local fake rather than the memory repository).
type memFiles struct {
	files map[string]File
}

func newMemFiles() *memFiles { return &memFiles{files: map[string]File{}} }

func (m *memFiles) InsertFile(_ context.Context, file File) error {
	m.files[file.ID] = file
	return nil
}
func (m *memFiles) GetFile(_ context.Context, id string) (File, error) {
	file, ok := m.files[id]
	if !ok {
		return File{}, kernel.ErrNotFound
	}
	return file, nil
}
func (m *memFiles) ListFilesByRef(context.Context, string, string) ([]File, error) {
	return nil, nil
}
func (m *memFiles) ListFilesByOwner(context.Context, string, int, int) ([]File, error) {
	return nil, nil
}
func (m *memFiles) SoftDeleteFile(context.Context, string) error { return nil }
func (m *memFiles) ListExportFilesOlderThan(context.Context, time.Time) ([]File, error) {
	return nil, nil
}

func newMultipartService(t *testing.T) (*Service, *memFiles, *filestore.Memory) {
	t.Helper()
	store := newMemFiles()
	blobs := filestore.NewMemory()
	svc := NewService(store, blobs, fakeTickets{visible: true}, 10<<20,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, store, blobs
}

func TestMultipartLifecycle(t *testing.T) {
	svc, store, _ := newMultipartService(t)
	ctx := context.Background()

	// Initiate: two parts, ticket attachment.
	session, err := svc.InitiateMultipart(ctx, "owner-1", "org-1", "big.bin", "application/octet-stream", 2, RefTicket, "ticket-1")
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if session.Key == "" || session.UploadID == "" || !session.Streaming {
		t.Fatalf("session: %+v", session)
	}

	// Upload both parts through the streaming path (dev mode).
	if _, err := svc.PutMultipartPart(ctx, session.UploadID, 1, strings.NewReader("hello ")); err != nil {
		t.Fatalf("part 1: %v", err)
	}
	if _, err := svc.PutMultipartPart(ctx, session.UploadID, 2, strings.NewReader("world")); err != nil {
		t.Fatalf("part 2: %v", err)
	}

	// Complete: metadata row appears, size comes from storage.
	file, err := svc.CompleteMultipart(ctx, "owner-1", "org-1", "big.bin", "application/octet-stream",
		session.Key, session.UploadID,
		[]filestore.CompletedPart{{PartNumber: 1}, {PartNumber: 2}}, RefTicket, "ticket-1")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if file.SizeBytes != 11 {
		t.Fatalf("size: want 11, got %d", file.SizeBytes)
	}
	if file.Filename != "big.bin" || file.RefID != "ticket-1" {
		t.Fatalf("metadata: %+v", file)
	}
	// The blob is assembled in order and downloadable.
	_, stream, err := svc.Download(ctx, "owner-1", "org-1", file.ID)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer func() { _ = stream.Close() }()
	got, _ := io.ReadAll(stream)
	if !bytes.Equal(got, []byte("hello world")) {
		t.Fatalf("assembled blob: %q", got)
	}
	if _, err := store.GetFile(ctx, file.ID); err != nil {
		t.Fatalf("metadata row: %v", err)
	}

	// Completing the same session again fails: it is consumed.
	if _, err := svc.CompleteMultipart(ctx, "owner-1", "org-1", "big.bin", "application/octet-stream",
		session.Key, session.UploadID,
		[]filestore.CompletedPart{{PartNumber: 1}, {PartNumber: 2}}, "", ""); err == nil {
		t.Fatal("double complete must fail")
	}
}

func TestMultipartRejectsBadRequests(t *testing.T) {
	svc, _, _ := newMultipartService(t)
	ctx := context.Background()

	if _, err := svc.InitiateMultipart(ctx, "o", "org-1", "f.bin", "text/plain", 1, "", ""); err == nil {
		t.Fatal("one part must be rejected")
	}
	if _, err := svc.InitiateMultipart(ctx, "o", "org-1", "f.bin", "text/plain", 10001, "", ""); err == nil {
		t.Fatal("too many parts must be rejected")
	}
	hidden := NewService(newMemFiles(), filestore.NewMemory(), fakeTickets{visible: false}, 10<<20,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := hidden.InitiateMultipart(ctx, "o", "org-1", "f.bin", "text/plain", 2, RefTicket, "hidden-ticket"); err == nil {
		t.Fatal("invisible ticket must be rejected")
	}
	if _, err := svc.InitiateMultipart(ctx, "o", "org-1", " ", "text/plain", 2, "", ""); err == nil {
		t.Fatal("blank filename must be rejected")
	}
	if _, err := svc.InitiateMultipart(ctx, "o", "org-1", "f.bin", "text/plain", 2, "unknown", "x"); err == nil {
		t.Fatal("unknown ref type must be rejected")
	}

	// Completion with fewer parts than initiated fails validation.
	session, err := svc.InitiateMultipart(ctx, "o", "org-1", "f.bin", "text/plain", 2, "", "")
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if _, err := svc.CompleteMultipart(ctx, "o", "org-1", "f.bin", "text/plain",
		session.Key, session.UploadID, []filestore.CompletedPart{{PartNumber: 1}}, "", ""); err == nil {
		t.Fatal("single-part completion must be rejected")
	}
}

func TestMultipartAbortAndStaleSweep(t *testing.T) {
	svc, _, blobs := newMultipartService(t)
	ctx := context.Background()

	session, err := svc.InitiateMultipart(ctx, "o", "org-1", "gone.bin", "text/plain", 2, "", "")
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	// Explicit abort discards the session.
	if err := svc.AbortMultipart(ctx, session.Key, session.UploadID); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if _, err := svc.PutMultipartPart(ctx, session.UploadID, 1, strings.NewReader("x")); err == nil {
		t.Fatal("aborted session must not accept parts")
	}

	// Fresh sessions survive the sweep; a backdated one does not.
	fresh, err := svc.InitiateMultipart(ctx, "o", "org-1", "fresh.bin", "text/plain", 2, "", "")
	if err != nil {
		t.Fatalf("initiate fresh: %v", err)
	}
	stale, err := svc.InitiateMultipart(ctx, "o", "org-1", "stale.bin", "text/plain", 2, "", "")
	if err != nil {
		t.Fatalf("initiate stale: %v", err)
	}
	clock := time.Unix(1_600_000_000, 0) // genuinely in the past
	blobs.WithClock(func() time.Time { return clock })
	backdated, err := svc.InitiateMultipart(ctx, "o", "org-1", "old.bin", "text/plain", 2, "", "")
	if err != nil {
		t.Fatalf("initiate backdated: %v", err)
	}
	// Back to the real clock: fresh sessions are fresh again.
	blobs.WithClock(time.Now)

	aborted, err := svc.AbortStaleMultiparts(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if aborted != 1 {
		t.Fatalf("sweep: want 1 abort, got %d", aborted)
	}
	if _, err := svc.PutMultipartPart(ctx, backdated.UploadID, 1, strings.NewReader("x")); err == nil {
		t.Fatal("swept session must be gone")
	}
	if _, err := svc.PutMultipartPart(ctx, fresh.UploadID, 1, strings.NewReader("x")); err != nil {
		t.Fatalf("fresh session must survive: %v", err)
	}
	if _, err := svc.PutMultipartPart(ctx, session.UploadID, 1, strings.NewReader("x")); err == nil {
		t.Fatal("explicitly aborted session must stay gone")
	}
	if _, err := svc.PutMultipartPart(ctx, stale.UploadID, 1, strings.NewReader("x")); err != nil {
		t.Fatalf("recent session must survive the sweep: %v", err)
	}
}
