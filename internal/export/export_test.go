package export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newExportService(t *testing.T) (*Service, *repository.MemoryStore, *filestore.Memory) {
	t.Helper()
	store := repository.NewMemoryStore()
	blobs := filestore.NewMemory()
	svc := NewService(store, store, store, store, store, store, blobs)
	return svc, store, blobs
}

// registerUser inserts a user (and its password identity) directly; the
// bundle builder only reads the profile back.
func registerUser(t *testing.T, store *repository.MemoryStore, email, name string) users.User {
	t.Helper()
	user := users.User{ID: kernel.NewID(), Email: email, Name: name, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateUser(context.Background(), user, "password-hash"); err != nil {
		t.Fatalf("register: %v", err)
	}
	return user
}

// The test helpers share one blob store so ownership checks hit real bytes.

func TestExportBundleRoundtrip(t *testing.T) {
	svc, store, _ := newExportService(t)
	ctx := context.Background()
	user := registerUser(t, store, "export-me@example.com", "Exporter")

	// A ticket so the bundle has content.
	typ := tickets.TicketType{ID: kernel.NewID(), Key: "exp", Name: "Export", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateTicketType(ctx, typ); err != nil {
		t.Fatalf("type: %v", err)
	}
	if _, err := store.CreateTicket(ctx, tickets.Ticket{
		ID: kernel.NewID(), TypeID: typ.ID, Title: "Exported audit request", CreatedBy: user.ID,
	}); err != nil {
		t.Fatalf("ticket: %v", err)
	}

	file, err := svc.Generate(ctx, user.ID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if file.RefType != RefType || file.OwnerID != user.ID || file.SizeBytes == 0 {
		t.Fatalf("export metadata mismatch: %+v", file)
	}

	// The bundle is a ZIP of JSON documents containing the ticket.
	download, stream, err := svc.Open(ctx, user.ID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if download.ID != file.ID {
		t.Fatal("latest must resolve the freshly generated bundle")
	}
	data, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range archive.File {
		names[f.Name] = true
	}
	for _, expected := range []string{"profile.json", "tickets.json", "identities.json"} {
		if !names[expected] {
			t.Fatalf("bundle missing %s, got %v", expected, names)
		}
	}
}

func TestExportOwnershipAndPruning(t *testing.T) {
	svc, store, blobs := newExportService(t)
	ctx := context.Background()
	owner := registerUser(t, store, "export-owner@example.com", "Owner")
	stranger := registerUser(t, store, "export-stranger@example.com", "Stranger")

	if _, err := svc.Generate(ctx, owner.ID); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// A stranger has no bundle of their own.
	if _, _, err := svc.Open(ctx, stranger.ID); err == nil {
		t.Fatal("stranger must not resolve someone else's export")
	}
	// Owner-only visibility is also enforced at the files layer.
	latest, err := svc.Latest(ctx, owner.ID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	fileSvc := files.NewService(store, blobs, nil, 10<<20, discardLogger())
	if _, _, err := fileSvc.Download(ctx, stranger.ID, "*", latest.ID); err == nil {
		t.Fatal("foreign download of a private file must fail")
	}
	if _, _, err := fileSvc.Download(ctx, owner.ID, "*", latest.ID); err != nil {
		t.Fatalf("owner download: %v", err)
	}

	// Pruning only removes bundles past the retention window.
	removed, err := svc.Prune(ctx)
	if err != nil || removed != 0 {
		t.Fatalf("fresh bundle must survive pruning: %d %v", removed, err)
	}
}
