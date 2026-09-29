package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/repository"
)

// Ref types an attachment can point at.
const (
	FileRefTicket = "ticket"
	// PresignTTL bounds direct-download URLs handed out by the S3 adapter.
	PresignTTL = 15 * time.Minute
)

// FilesService owns the upload/download lifecycle: bytes go to the FileStore
// adapter (S3/MinIO in production, in-process in dev), metadata to the
// FilesRepo. Attachments carry their entity reference and are permission-
// checked against it (ticket attachments reuse the ticket visibility rules).
type FilesService struct {
	store   FilesRepo
	blobs   FileStore
	tickets *TicketsService
	logger  *slog.Logger
	maxSize int64
}

// NewFilesService wires the service; maxSize clamps uploads (0 = 10 MiB).
func NewFilesService(store FilesRepo, blobs FileStore, tickets *TicketsService, maxSize int64, logger *slog.Logger) *FilesService {
	if maxSize <= 0 {
		maxSize = 10 << 20
	}
	return &FilesService{store: store, blobs: blobs, tickets: tickets, maxSize: maxSize, logger: logger}
}

// MaxSize exposes the upload cap for the HTTP body-limit coordination.
func (s *FilesService) MaxSize() int64 { return s.maxSize }

// UploadResult carries the stored metadata plus whether the client can fetch
// a presigned direct URL (S3 mode) or must stream through the API.
type UploadResult struct {
	File     model.File
	Presigns bool
}

// Upload stores a blob. refType/refId attach it to an entity; ticket
// attachments are validated for visibility here so an attacker cannot
// attach to (or read via) a ticket they cannot see.
func (s *FilesService) Upload(ctx context.Context, ownerID, orgID, filename, contentType string, size int64, r io.Reader, refType, refID string) (UploadResult, error) {
	if strings.TrimSpace(filename) == "" {
		return UploadResult{}, validationError("filename is required")
	}
	if size > s.maxSize {
		return UploadResult{}, validationError(fmt.Sprintf("file exceeds the %d MiB limit", s.maxSize>>20))
	}
	if refType == FileRefTicket {
		if err := s.checkTicketAttachment(ctx, ownerID, orgID, refID); err != nil {
			return UploadResult{}, err
		}
	} else if refType != "" {
		return UploadResult{}, validationError("unknown attachment type " + refType)
	}
	key := newID() + "/" + sanitizeFilename(filename)
	hash := sha256.New()
	limited := io.TeeReader(io.LimitReader(r, s.maxSize+1), hash)
	if err := s.blobs.Put(ctx, key, limited, size, contentType); err != nil {
		return UploadResult{}, fmt.Errorf("store blob: %w", err)
	}
	file := model.File{
		ID:          newID(),
		OwnerID:     ownerID,
		OrgID:       orgID,
		Key:         key,
		Filename:    sanitizeFilename(filename),
		ContentType: contentType,
		SizeBytes:   size,
		Checksum:    hex.EncodeToString(hash.Sum(nil)),
		RefType:     refType,
		RefID:       refID,
		CreatedAt:   time.Now(),
	}
	if err := s.store.InsertFile(ctx, file); err != nil {
		// The blob is orphaned; the metadata insert failed, so clean up.
		_ = s.blobs.Delete(ctx, key)
		return UploadResult{}, err
	}
	return UploadResult{File: file, Presigns: s.PresignsEnabled()}, nil
}

// Download opens the blob stream after the metadata + permission checks.
func (s *FilesService) Download(ctx context.Context, callerID, orgID, fileID string) (model.File, io.ReadCloser, error) {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return model.File{}, nil, err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return model.File{}, nil, err
	}
	stream, err := s.blobs.Open(ctx, file.Key)
	if err != nil {
		return model.File{}, nil, err
	}
	return file, stream, nil
}

// Presign hands out a direct URL when the store supports it.
func (s *FilesService) Presign(ctx context.Context, callerID, orgID, fileID string) (string, time.Duration, error) {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return "", 0, err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return "", 0, err
	}
	url, err := s.blobs.Presign(ctx, file.Key, PresignTTL)
	if err != nil {
		if errors.Is(err, filestore.ErrNotSupported) {
			return "", 0, filestore.ErrNotSupported
		}
		return "", 0, err
	}
	return url, PresignTTL, nil
}

// GetForActor returns metadata after the visibility check.
func (s *FilesService) GetForActor(ctx context.Context, callerID, orgID, fileID string) (model.File, error) {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return model.File{}, err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return model.File{}, err
	}
	return file, nil
}

// ListByRef lists one entity's attachments (ticket detail page).
func (s *FilesService) ListByRef(ctx context.Context, callerID, orgID, refType, refID string) ([]model.File, error) {
	if refType == FileRefTicket {
		if err := s.checkTicketAttachment(ctx, callerID, orgID, refID); err != nil {
			return nil, err
		}
	}
	files, err := s.store.ListFilesByRef(ctx, refType, refID)
	if files == nil {
		files = []model.File{}
	}
	return files, err
}

// Delete soft-deletes the metadata and removes the blob; only the owner or
// an admin with files:delete permission (checked by the handler) gets here.
func (s *FilesService) Delete(ctx context.Context, callerID, orgID, fileID string) error {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return err
	}
	if file.OwnerID != callerID {
		return repository.ErrNotFound // hide even the existence from non-owners
	}
	if err := s.store.SoftDeleteFile(ctx, fileID); err != nil {
		return err
	}
	return s.blobs.Delete(ctx, file.Key)
}

// PresignsEnabled reports whether direct URLs are available (S3 mode).
func (s *FilesService) PresignsEnabled() bool {
	_, err := s.blobs.Presign(context.Background(), "probe", time.Second)
	return !errors.Is(err, filestore.ErrNotSupported)
}

// canRead applies attachment-scoped visibility: ticket attachments follow
// the ticket's canView rules, everything else is owner/admin territory
// (the handler adds the files:read permission guard on top).
func (s *FilesService) canRead(ctx context.Context, callerID, orgID string, file model.File) error {
	if file.RefType == FileRefTicket {
		return s.checkTicketAttachment(ctx, callerID, orgID, file.RefID)
	}
	// Private files (plain uploads, export bundles) are owner-only: no
	// permission widens this, so foreign ids probe nothing.
	if file.OwnerID != callerID {
		return fmt.Errorf("%w: file not found", repository.ErrNotFound)
	}
	return nil
}

// checkTicketAttachment verifies the caller can see the ticket the file
// hangs off, and that the ticket lives in the active domain.
func (s *FilesService) checkTicketAttachment(ctx context.Context, callerID, orgID, ticketID string) error {
	if s.tickets == nil {
		return validationError("attachments are not available")
	}
	ticket, _, err := s.tickets.Get(ctx, Actor{ID: callerID, Domain: orgID}, ticketID)
	if err != nil {
		return fmt.Errorf("%w: ticket not found or not visible", repository.ErrNotFound)
	}
	if orgID != "" && orgID != PlatformDomain && ticket.OrgID != orgID {
		return repository.ErrNotFound
	}
	return nil
}

// sanitizeFilename strips path components and control characters; the
// original extension survives for content-type hints on download.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
	}
	name = b.String()
	if name == "" || name == "." || name == ".." {
		name = "upload"
	}
	if len(name) > 200 {
		name = name[len(name)-200:]
	}
	return name
}
