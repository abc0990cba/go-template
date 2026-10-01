// Package files owns the upload/download lifecycle of file attachments:
// bytes go to the FileStore adapter (S3/MinIO in production, in-process in
// dev), metadata to the Store. Attachments carry their entity reference and
// are permission-checked against it (ticket attachments reuse the ticket
// visibility rules).
package files

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

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/rbac"
)

// Ref types an attachment can point at, plus the direct-download URL budget.
const (
	// RefTicket marks an attachment hung off a ticket.
	RefTicket = "ticket"
	// PresignTTL bounds direct-download URLs handed out by the S3 adapter.
	PresignTTL = 15 * time.Minute
)

// File is one uploaded blob's metadata; the bytes live in the file store.
// RefType/RefId attach it to an entity (e.g. "ticket" + ticket id).
type File struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	OrgID       string     `json:"org_id,omitempty"`
	Key         string     `json:"-"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	Checksum    string     `json:"checksum_sha256,omitempty"`
	RefType     string     `json:"ref_type,omitempty"`
	RefID       string     `json:"ref_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// Store persists file metadata (the blobs live in a FileStore).
type Store interface {
	InsertFile(ctx context.Context, file File) error
	GetFile(ctx context.Context, id string) (File, error)
	ListFilesByRef(ctx context.Context, refType, refID string) ([]File, error)
	ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]File, error)
	SoftDeleteFile(ctx context.Context, id string) error
	ListExportFilesOlderThan(ctx context.Context, olderThan time.Time) ([]File, error)
}

// FileStore keeps the blobs behind Store metadata.
type FileStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Presign(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// TicketView is the slice of the tickets feature the attachment checks and
// the upload handler need: whether a caller may see a ticket (plus its org),
// and the timeline entry an uploaded attachment appends. The tickets service
// satisfies it structurally.
type TicketView interface {
	// VisibleTicketOrg reports the ticket's organization when actorID may
	// see it in domain; an error otherwise.
	VisibleTicketOrg(ctx context.Context, actorID, domain, ticketID string) (string, error)
	RecordAttachment(ctx context.Context, actorID, ticketID, filename string)
}

// Service runs the upload/download lifecycle. Attachments carry their
// entity reference and are permission-checked against it.
type Service struct {
	store   Store
	blobs   FileStore
	tickets TicketView
	logger  *slog.Logger
	maxSize int64
}

// NewService wires the service; maxSize clamps uploads (0 = 10 MiB).
func NewService(store Store, blobs FileStore, tickets TicketView, maxSize int64, logger *slog.Logger) *Service {
	if maxSize <= 0 {
		maxSize = 10 << 20
	}
	return &Service{store: store, blobs: blobs, tickets: tickets, maxSize: maxSize, logger: logger}
}

// MaxSize exposes the upload cap for the HTTP body-limit coordination.
func (s *Service) MaxSize() int64 { return s.maxSize }

// UploadResult carries the stored metadata plus whether the client can fetch
// a presigned direct URL (S3 mode) or must stream through the API.
type UploadResult struct {
	File     File
	Presigns bool
}

// Upload stores a blob. refType/refId attach it to an entity; ticket
// attachments are validated for visibility here so an attacker cannot
// attach to (or read via) a ticket they cannot see.
func (s *Service) Upload(ctx context.Context, ownerID, orgID, filename, contentType string, size int64, r io.Reader, refType, refID string) (UploadResult, error) {
	if strings.TrimSpace(filename) == "" {
		return UploadResult{}, kernel.Invalid("filename is required")
	}
	if size > s.maxSize {
		return UploadResult{}, kernel.Invalid(fmt.Sprintf("file exceeds the %d MiB limit", s.maxSize>>20))
	}
	if refType == RefTicket {
		if err := s.checkTicketAttachment(ctx, ownerID, orgID, refID); err != nil {
			return UploadResult{}, err
		}
	} else if refType != "" {
		return UploadResult{}, kernel.Invalid("unknown attachment type " + refType)
	}
	key := kernel.NewID() + "/" + sanitizeFilename(filename)
	hash := sha256.New()
	limited := io.TeeReader(io.LimitReader(r, s.maxSize+1), hash)
	if err := s.blobs.Put(ctx, key, limited, size, contentType); err != nil {
		return UploadResult{}, fmt.Errorf("store blob: %w", err)
	}
	file := File{
		ID:          kernel.NewID(),
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
func (s *Service) Download(ctx context.Context, callerID, orgID, fileID string) (File, io.ReadCloser, error) {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return File{}, nil, err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return File{}, nil, err
	}
	stream, err := s.blobs.Open(ctx, file.Key)
	if err != nil {
		return File{}, nil, err
	}
	return file, stream, nil
}

// Presign hands out a direct URL when the store supports it.
func (s *Service) Presign(ctx context.Context, callerID, orgID, fileID string) (string, time.Duration, error) {
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
func (s *Service) GetForActor(ctx context.Context, callerID, orgID, fileID string) (File, error) {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return File{}, err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return File{}, err
	}
	return file, nil
}

// ListByRef lists one entity's attachments (ticket detail page).
func (s *Service) ListByRef(ctx context.Context, callerID, orgID, refType, refID string) ([]File, error) {
	if refType == RefTicket {
		if err := s.checkTicketAttachment(ctx, callerID, orgID, refID); err != nil {
			return nil, err
		}
	}
	files, err := s.store.ListFilesByRef(ctx, refType, refID)
	if files == nil {
		files = []File{}
	}
	return files, err
}

// Delete soft-deletes the metadata and removes the blob; only the owner or
// an admin with files:delete permission (checked by the handler) gets here.
func (s *Service) Delete(ctx context.Context, callerID, orgID, fileID string) error {
	file, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	if err := s.canRead(ctx, callerID, orgID, file); err != nil {
		return err
	}
	if file.OwnerID != callerID {
		return kernel.ErrNotFound // hide even the existence from non-owners
	}
	if err := s.store.SoftDeleteFile(ctx, fileID); err != nil {
		return err
	}
	return s.blobs.Delete(ctx, file.Key)
}

// PresignsEnabled reports whether direct URLs are available (S3 mode).
func (s *Service) PresignsEnabled() bool {
	_, err := s.blobs.Presign(context.Background(), "probe", time.Second)
	return !errors.Is(err, filestore.ErrNotSupported)
}

// canRead applies attachment-scoped visibility: ticket attachments follow
// the ticket's canView rules, everything else is owner/admin territory
// (the handler adds the files:read permission guard on top).
func (s *Service) canRead(ctx context.Context, callerID, orgID string, file File) error {
	if file.RefType == RefTicket {
		return s.checkTicketAttachment(ctx, callerID, orgID, file.RefID)
	}
	// Private files (plain uploads, export bundles) are owner-only: no
	// permission widens this, so foreign ids probe nothing.
	if file.OwnerID != callerID {
		return fmt.Errorf("%w: file not found", kernel.ErrNotFound)
	}
	return nil
}

// checkTicketAttachment verifies the caller can see the ticket the file
// hangs off, and that the ticket lives in the active domain.
func (s *Service) checkTicketAttachment(ctx context.Context, callerID, orgID, ticketID string) error {
	if s.tickets == nil {
		return kernel.Invalid("attachments are not available")
	}
	ticketOrg, err := s.tickets.VisibleTicketOrg(ctx, callerID, orgID, ticketID)
	if err != nil {
		return fmt.Errorf("%w: ticket not found or not visible", kernel.ErrNotFound)
	}
	if orgID != "" && orgID != rbac.PlatformDomain && ticketOrg != orgID {
		return kernel.ErrNotFound
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
