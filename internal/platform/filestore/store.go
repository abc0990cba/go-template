// Package filestore provides the FileStore port adapters: an in-process
// store for development (and S3-less deployments) and an S3-compatible
// client for production (AWS S3, MinIO, or any compatible endpoint).
package filestore

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound reports a missing blob; adapters wrap driver errors into it.
var ErrNotFound = errors.New("blob not found")

// ErrNotSupported marks capability gaps of the active adapter (presigned
// URLs from the in-process store, most notably).
var ErrNotSupported = errors.New("not supported by this file store")

// Store keeps opaque blobs addressable by key. Implementations must be safe
// for concurrent use.
type Store interface {
	// Put writes the blob; size is advisory (used by S3 for buffering).
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Open returns a stream over the blob; callers close it.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	// Presign returns a temporary direct-download URL; ErrNotSupported when
	// the adapter cannot hand out URLs (the in-process store).
	Presign(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// CompletedPart is one uploaded part of a multipart session, identified by
// its 1-based part number and the ETag the storage returned on upload.
type CompletedPart struct {
	PartNumber int32
	ETag       string
}

// MultipartUpload describes one in-flight multipart session.
type MultipartUpload struct {
	Key       string
	UploadID  string
	Initiated time.Time
}

// MultipartStorer is the optional resumable-upload capability of a file
// store: the client uploads parts directly to presigned part URLs and the
// server assembles them on completion. Adapters may not support it (the
// in-process store does, S3 does) — feature code type-asserts and degrades.
type MultipartStorer interface {
	// CreateMultipart opens a session for the destination key.
	CreateMultipart(ctx context.Context, key, contentType string) (uploadID string, err error)
	// PresignPart returns a PUT URL for one part (1-based number).
	PresignPart(ctx context.Context, key, uploadID string, partNumber int32, ttl time.Duration) (string, error)
	// CompleteMultipart assembles the listed parts (in order) into the key.
	CompleteMultipart(ctx context.Context, key, uploadID string, parts []CompletedPart) error
	// AbortMultipart discards a session and its uploaded parts.
	AbortMultipart(ctx context.Context, key, uploadID string) error
	// ListMultiparts returns every in-flight session (stale-session sweeps
	// filter by Initiated themselves).
	ListMultiparts(ctx context.Context) ([]MultipartUpload, error)
}

// Sizer is the optional byte-count capability: multipart completion derives
// the authoritative object size from storage rather than client claims.
type Sizer interface {
	Size(ctx context.Context, key string) (int64, error)
}
