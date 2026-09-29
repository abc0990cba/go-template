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
