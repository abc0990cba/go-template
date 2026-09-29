package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// ExportRefType marks files that carry a user's GDPR data bundle.
const ExportRefType = "export"

// ExportRetention bounds how long a finished bundle stays downloadable; the
// worker prunes older ones.
const ExportRetention = 7 * 24 * time.Hour

// ExportService builds a user's data bundle (GDPR art. 20 data
// portability): one ZIP of JSON documents covering every entity the account
// owns or is the subject of, stored as a file the user alone can download.
type ExportService struct {
	users         UsersRepo
	auth          AuthRepo
	tickets       TicketsRepo
	notifications NotificationsRepo
	audit         AuditRepo
	files         FilesRepo
	blobs         FileStore
}

// NewExportService wires the bundle builder; every repo may be nil where the
// entity is unavailable (degraded modes), and those sections come out empty.
func NewExportService(users UsersRepo, auth AuthRepo, tickets TicketsRepo, notifications NotificationsRepo, audit AuditRepo, files FilesRepo, blobs FileStore) *ExportService {
	return &ExportService{
		users: users, auth: auth, tickets: tickets, notifications: notifications,
		audit: audit, files: files, blobs: blobs,
	}
}

// Generate collects, packs, and stores the bundle; the returned file is
// owned by the user (owner-only downloads).
func (s *ExportService) Generate(ctx context.Context, userID string) (model.File, error) {
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return model.File{}, err
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
		if tickets, _, err := s.tickets.ListTickets(ctx, repository.TicketOptions{
			ListOptions: repository.ListOptions{Limit: 500, Mode: "offset"},
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
		if entries, err := s.audit.ListAudit(ctx, repository.AuditOptions{
			ListOptions: repository.ListOptions{Limit: 1000, Mode: "offset"},
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
		return model.File{}, err
	}

	key := "exports/" + userID + "/" + newID() + ".zip"
	file := model.File{
		ID:          newID(),
		OwnerID:     userID,
		Key:         key,
		Filename:    fmt.Sprintf("my-data-%s.zip", time.Now().UTC().Format("20060102-150405")),
		ContentType: "application/zip",
		SizeBytes:   int64(len(zipBytes)),
		RefType:     ExportRefType,
		RefID:       userID,
		CreatedAt:   time.Now(),
	}
	if err := s.blobs.Put(ctx, key, bytes.NewReader(zipBytes), file.SizeBytes, file.ContentType); err != nil {
		return model.File{}, fmt.Errorf("store export: %w", err)
	}
	if s.files != nil {
		if err := s.files.InsertFile(ctx, file); err != nil {
			_ = s.blobs.Delete(ctx, key)
			return model.File{}, err
		}
	}
	return file, nil
}

// Latest resolves the user's most recent finished export file, if any.
func (s *ExportService) Latest(ctx context.Context, userID string) (model.File, error) {
	if s.files == nil {
		return model.File{}, repository.ErrNotFound
	}
	files, err := s.files.ListFilesByRef(ctx, ExportRefType, userID)
	if err != nil || len(files) == 0 {
		return model.File{}, repository.ErrNotFound
	}
	latest := files[0]
	for _, file := range files[1:] {
		if file.CreatedAt.After(latest.CreatedAt) {
			latest = file
		}
	}
	if time.Since(latest.CreatedAt) > ExportRetention {
		return model.File{}, repository.ErrNotFound
	}
	return latest, nil
}

// Open streams the bundle after the ownership check (defense in depth: the
// files service already enforces it).
func (s *ExportService) Open(ctx context.Context, userID string) (model.File, io.ReadCloser, error) {
	file, err := s.Latest(ctx, userID)
	if err != nil {
		return model.File{}, nil, err
	}
	if file.OwnerID != userID {
		return model.File{}, nil, repository.ErrNotFound
	}
	stream, err := s.blobs.Open(ctx, file.Key)
	if err != nil {
		return model.File{}, nil, err
	}
	return file, stream, nil
}

// Prune removes expired export bundles (metadata + blob); the worker calls
// it on a schedule.
func (s *ExportService) Prune(ctx context.Context) (int, error) {
	if s.files == nil || s.blobs == nil {
		return 0, nil
	}
	old, err := s.files.ListExportFilesOlderThan(ctx, time.Now().Add(-ExportRetention))
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
