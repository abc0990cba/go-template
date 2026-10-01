// Package export builds a user's GDPR data bundle (art. 20 data
// portability): one ZIP of JSON documents covering every entity the account
// owns or is the subject of, stored as a file the user alone can download.
package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

// RefType marks files that carry a user's GDPR data bundle.
const RefType = "export"

// Retention bounds how long a finished bundle stays downloadable; the
// worker prunes older ones.
const Retention = 7 * 24 * time.Hour

// The jobs feature's worker handler calls the bundle builder through the
// narrow jobs.Exporter interface; prove the structural fit.
var _ jobs.Exporter = (*Service)(nil)

// Users reads the export subject's profile.
type Users interface {
	GetUser(ctx context.Context, id string) (users.User, error)
}

// Auth reads the subject's linked identities and sessions.
type Auth interface {
	ListIdentities(ctx context.Context, userID string) ([]auth.Identity, error)
	ListSessions(ctx context.Context, userID string) ([]auth.SessionInfo, error)
}

// Tickets lists the tickets the subject filed.
type Tickets interface {
	ListTickets(ctx context.Context, opts kernel.TicketOptions) ([]tickets.Ticket, string, error)
}

// Notifications reads the subject's in-app notifications.
type Notifications interface {
	ListNotifications(ctx context.Context, userID string, limit, offset int) ([]notifications.Notification, error)
}

// AuditTrail reads audit entries about the subject.
type AuditTrail interface {
	ListAudit(ctx context.Context, opts kernel.AuditOptions) ([]audit.Entry, error)
}

// FileMeta stores and lists the bundle file records.
type FileMeta interface {
	InsertFile(ctx context.Context, file files.File) error
	ListFilesByRef(ctx context.Context, refType, refID string) ([]files.File, error)
	ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]files.File, error)
	SoftDeleteFile(ctx context.Context, id string) error
	ListExportFilesOlderThan(ctx context.Context, olderThan time.Time) ([]files.File, error)
}

// Service builds a user's data bundle (GDPR art. 20 data
// portability): one ZIP of JSON documents covering every entity the account
// owns or is the subject of, stored as a file the user alone can download.
type Service struct {
	users         Users
	auth          Auth
	tickets       Tickets
	notifications Notifications
	audit         AuditTrail
	files         FileMeta
	blobs         files.FileStore
}

// NewService wires the bundle builder; every repo may be nil where the
// entity is unavailable (degraded modes), and those sections come out empty.
func NewService(users Users, auth Auth, tickets Tickets, notifications Notifications, audit AuditTrail, files FileMeta, blobs files.FileStore) *Service {
	return &Service{
		users: users, auth: auth, tickets: tickets, notifications: notifications,
		audit: audit, files: files, blobs: blobs,
	}
}

// Generate collects, packs, and stores the bundle; the returned file is
// owned by the user (owner-only downloads).
func (s *Service) Generate(ctx context.Context, userID string) (files.File, error) {
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return files.File{}, err
	}

	bundle := map[string]any{
		"generated_at": time.Now().UTC(),
		"profile":      user,
	}
	if s.auth != nil {
		if identities, err := s.auth.ListIdentities(ctx, userID); err == nil {
			bundle["identities"] = identities
		}
		if sessions, err := s.auth.ListSessions(ctx, userID); err == nil {
			bundle["sessions"] = sessions
		}
	}
	if s.tickets != nil {
		if tickets, _, err := s.tickets.ListTickets(ctx, kernel.TicketOptions{
			ListOptions: kernel.ListOptions{Limit: 500, Mode: "offset"},
			CreatedBy:   userID,
		}); err == nil {
			bundle["tickets"] = tickets
		}
	}
	if s.notifications != nil {
		if notifications, err := s.notifications.ListNotifications(ctx, userID, 1000, 0); err == nil {
			bundle["notifications"] = notifications
		}
	}
	if s.audit != nil {
		if entries, err := s.audit.ListAudit(ctx, kernel.AuditOptions{
			ListOptions: kernel.ListOptions{Limit: 1000, Mode: "offset"},
			Subject:     userID,
		}); err == nil {
			bundle["audit_entries_about_me"] = entries
		}
	}
	if s.files != nil {
		if files, err := s.files.ListFilesByOwner(ctx, userID, 1000, 0); err == nil {
			bundle["files"] = files
		}
	}

	zipBytes, err := marshalBundle(bundle)
	if err != nil {
		return files.File{}, err
	}

	key := "exports/" + userID + "/" + kernel.NewID() + ".zip"
	file := files.File{
		ID:          kernel.NewID(),
		OwnerID:     userID,
		Key:         key,
		Filename:    fmt.Sprintf("my-data-%s.zip", time.Now().UTC().Format("20060102-150405")),
		ContentType: "application/zip",
		SizeBytes:   int64(len(zipBytes)),
		RefType:     RefType,
		RefID:       userID,
		CreatedAt:   time.Now(),
	}
	if err := s.blobs.Put(ctx, key, bytes.NewReader(zipBytes), file.SizeBytes, file.ContentType); err != nil {
		return files.File{}, fmt.Errorf("store export: %w", err)
	}
	if s.files != nil {
		if err := s.files.InsertFile(ctx, file); err != nil {
			_ = s.blobs.Delete(ctx, key)
			return files.File{}, err
		}
	}
	return file, nil
}

// Latest resolves the user's most recent finished export file, if any.
func (s *Service) Latest(ctx context.Context, userID string) (files.File, error) {
	if s.files == nil {
		return files.File{}, kernel.ErrNotFound
	}
	bundles, err := s.files.ListFilesByRef(ctx, RefType, userID)
	if err != nil || len(bundles) == 0 {
		return files.File{}, kernel.ErrNotFound
	}
	latest := bundles[0]
	for _, file := range bundles[1:] {
		if file.CreatedAt.After(latest.CreatedAt) {
			latest = file
		}
	}
	if time.Since(latest.CreatedAt) > Retention {
		return files.File{}, kernel.ErrNotFound
	}
	return latest, nil
}

// Open streams the bundle after the ownership check (defense in depth: the
// files service already enforces it).
func (s *Service) Open(ctx context.Context, userID string) (files.File, io.ReadCloser, error) {
	file, err := s.Latest(ctx, userID)
	if err != nil {
		return files.File{}, nil, err
	}
	if file.OwnerID != userID {
		return files.File{}, nil, kernel.ErrNotFound
	}
	stream, err := s.blobs.Open(ctx, file.Key)
	if err != nil {
		return files.File{}, nil, err
	}
	return file, stream, nil
}

// Prune removes expired export bundles (metadata + blob); the worker calls
// it on a schedule.
func (s *Service) Prune(ctx context.Context) (int, error) {
	if s.files == nil || s.blobs == nil {
		return 0, nil
	}
	old, err := s.files.ListExportFilesOlderThan(ctx, time.Now().Add(-Retention))
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, file := range old {
		if err := s.files.SoftDeleteFile(ctx, file.ID); err == nil {
			_ = s.blobs.Delete(ctx, file.Key)
			removed++
		}
	}
	return removed, nil
}

// marshalBundle writes one JSON document per entity into a ZIP.
func marshalBundle(bundle map[string]any) ([]byte, error) {
	buf := &bytes.Buffer{}
	archive := zip.NewWriter(buf)
	// Deterministic order keeps the artifact stable across runs.
	for _, name := range []string{"profile", "identities", "sessions", "tickets", "notifications", "audit_entries_about_me", "files"} {
		value, ok := bundle[name]
		if !ok {
			continue
		}
		writer, err := archive.Create(name + ".json")
		if err != nil {
			return nil, err
		}
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
