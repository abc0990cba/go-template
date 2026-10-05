package files

// Resumable uploads: large attachments go straight to the file store in
// parts, so the API process never buffers them. The store owns the session
// state (S3 multipart upload or the in-memory dev counterpart); this layer
// owns permission checks, key naming, and the metadata row created at
// completion — exactly the guarantees the single-shot upload gives.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/filestore"
)

// Multipart constraints: at least two parts (a single-part upload should
// have been a PUT), at most S3's own limit, and sessions left unattended
// longer than the stale budget are swept by the worker.
const (
	MinParts            = 2
	MaxParts            = 10000
	PartURLTTL          = time.Hour
	StaleMultipartAfter = 24 * time.Hour
)

// MultipartSession is the initiate response: everything a client needs to
// upload the parts and finish the session.
type MultipartSession struct {
	Key      string   `json:"key"`
	UploadID string   `json:"upload_id"`
	PartURLs []string `json:"part_urls"`
	// Streaming is true when the store cannot presign part URLs (dev mode):
	// the client PUTs parts to /files/multipart/parts instead.
	Streaming bool `json:"streaming"`
}

// multipartStorer reports whether the active blob store supports sessions.
func (s *Service) multipartStorer() (filestore.MultipartStorer, bool) {
	storer, ok := s.blobs.(filestore.MultipartStorer)
	return storer, ok
}

// MultipartsEnabled reports whether resumable uploads are available.
func (s *Service) MultipartsEnabled() bool {
	_, ok := s.multipartStorer()
	return ok
}

// InitiateMultipart opens a session. Ticket attachments are visibility
// checked exactly like single-shot uploads, so a caller cannot attach to
// (or stage bytes for) a ticket they cannot see.
func (s *Service) InitiateMultipart(ctx context.Context, ownerID, orgID, filename, contentType string, partCount int, refType, refID string) (MultipartSession, error) {
	if strings.TrimSpace(filename) == "" {
		return MultipartSession{}, kernel.Invalid("filename is required")
	}
	if partCount < MinParts || partCount > MaxParts {
		return MultipartSession{}, kernel.Invalid(fmt.Sprintf("parts must be between %d and %d", MinParts, MaxParts))
	}
	if refType == RefTicket {
		if err := s.checkTicketAttachment(ctx, ownerID, orgID, refID); err != nil {
			return MultipartSession{}, err
		}
	} else if refType != "" {
		return MultipartSession{}, kernel.Invalid("unknown attachment type " + refType)
	}
	storer, ok := s.multipartStorer()
	if !ok {
		return MultipartSession{}, errors.New("resumable uploads are not available on this file store")
	}
	key := kernel.NewID() + "/" + sanitizeFilename(filename)
	uploadID, err := storer.CreateMultipart(ctx, key, contentType)
	if err != nil {
		return MultipartSession{}, fmt.Errorf("open multipart session: %w", err)
	}
	session := MultipartSession{Key: key, UploadID: uploadID, Streaming: true}
	for part := int32(1); part <= int32(partCount); part++ {
		url, err := storer.PresignPart(ctx, key, uploadID, part, PartURLTTL)
		if err != nil {
			// Presigns are optional (dev mode streams instead); one
			// NotSupported means the rest will not come either.
			if errors.Is(err, filestore.ErrNotSupported) {
				session.PartURLs = nil
				return session, nil
			}
			_ = storer.AbortMultipart(ctx, key, uploadID)
			return MultipartSession{}, fmt.Errorf("presign part %d: %w", part, err)
		}
		session.PartURLs = append(session.PartURLs, url)
		session.Streaming = false
	}
	return session, nil
}

// CompleteMultipart assembles the parts and creates the metadata row — the
// completion counterpart of Upload. Size is computed by the store, so the
// client cannot claim a size it did not upload; the checksum is left empty
// (parts are integrity-checked by the store's ETags instead).
func (s *Service) CompleteMultipart(ctx context.Context, ownerID, orgID, filename, contentType, key, uploadID string, parts []filestore.CompletedPart, refType, refID string) (File, error) {
	if len(parts) < MinParts {
		return File{}, kernel.Invalid(fmt.Sprintf("at least %d parts are required", MinParts))
	}
	if refType == RefTicket {
		if err := s.checkTicketAttachment(ctx, ownerID, orgID, refID); err != nil {
			return File{}, err
		}
	} else if refType != "" {
		return File{}, kernel.Invalid("unknown attachment type " + refType)
	}
	storer, ok := s.multipartStorer()
	if !ok {
		return File{}, errors.New("resumable uploads are not available on this file store")
	}
	if err := storer.CompleteMultipart(ctx, key, uploadID, parts); err != nil {
		return File{}, err
	}
	file := File{
		ID:          kernel.NewID(),
		OwnerID:     ownerID,
		OrgID:       orgID,
		Key:         key,
		Filename:    sanitizeFilename(filename),
		ContentType: contentType,
		RefType:     refType,
		RefID:       refID,
		CreatedAt:   time.Now(),
	}
	sizer, ok := s.blobs.(filestore.Sizer)
	if !ok {
		return File{}, errors.New("this file store cannot report object sizes")
	}
	size, err := sizer.Size(ctx, key)
	if err != nil {
		_ = s.blobs.Delete(ctx, key)
		return File{}, err
	}
	file.SizeBytes = size
	if err := s.store.InsertFile(ctx, file); err != nil {
		_ = s.blobs.Delete(ctx, key)
		return File{}, err
	}
	s.logger.Info("multipart upload completed", "file_id", file.ID, "key", key, "size", size, "parts", len(parts), "owner", ownerID)
	return file, nil
}

// AbortMultipart discards a session; any caller who can see the target may
// abort their own abandoned session (uploadID is the capability).
func (s *Service) AbortMultipart(ctx context.Context, key, uploadID string) error {
	storer, ok := s.multipartStorer()
	if !ok {
		return errors.New("resumable uploads are not available on this file store")
	}
	if err := storer.AbortMultipart(ctx, key, uploadID); err != nil {
		// The store's not-found sentinel maps to the kernel's like every
		// other feature; storage failures surface as themselves.
		if errors.Is(err, filestore.ErrNotFound) {
			return fmt.Errorf("%w: upload session", kernel.ErrNotFound)
		}
		return err
	}
	return nil
}

// multipartStreamer is the dev-mode part intake: stores without presigned
// part URLs accept part bytes through the API instead.
type multipartStreamer interface {
	PutMultipartPart(ctx context.Context, uploadID string, partNumber int32, r io.Reader) (string, error)
}

// PutMultipartPart buffers one part in dev mode and returns its checksum
// as the ETag (presigned part URLs are the S3-mode affordance; there the
// client reads the ETag from the presigned PUT's own response headers).
func (s *Service) PutMultipartPart(ctx context.Context, uploadID string, partNumber int32, r io.Reader) (string, error) {
	streamer, ok := s.blobs.(multipartStreamer)
	if !ok {
		return "", errors.New("part streaming is only available on the in-process file store")
	}
	return streamer.PutMultipartPart(ctx, uploadID, partNumber, r)
}

// AbortStaleMultiparts sweeps sessions abandoned past the budget; the
// worker calls it on a schedule. Storage bills uploaded parts until a
// session completes or aborts, so this is a cost-control loop, not hygiene.
func (s *Service) AbortStaleMultiparts(ctx context.Context) (int, error) {
	storer, ok := s.multipartStorer()
	if !ok {
		return 0, nil
	}
	sessions, err := storer.ListMultiparts(ctx)
	if err != nil {
		return 0, err
	}
	aborted := 0
	for _, session := range sessions {
		if time.Since(session.Initiated) < StaleMultipartAfter {
			continue
		}
		if err := storer.AbortMultipart(ctx, session.Key, session.UploadID); err != nil {
			s.logger.Warn("stale multipart abort failed", "key", session.Key, "upload_id", session.UploadID, "err", err)
			continue
		}
		aborted++
	}
	return aborted, nil
}
