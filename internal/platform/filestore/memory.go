package filestore

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)

// Memory keeps blobs in the process; a dev-mode convenience that resets on
// restart, matching the in-memory repository's contract.
type Memory struct {
	mu         sync.RWMutex
	blobs      map[string][]byte
	multiparts map[string]*memoryMultipart
	now        func() time.Time
}

// NewMemory builds the in-process store.
func NewMemory() *Memory {
	return &Memory{blobs: map[string][]byte{}, now: time.Now}
}

// WithClock replaces the wall clock (tests backdate multipart sessions).
func (m *Memory) WithClock(now func() time.Time) *Memory { m.now = now; return m }

// Put writes the blob to memory under the given key.
func (m *Memory) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs[key] = data
	return nil
}

// Open returns a reader over the stored blob.
func (m *Memory) Open(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.blobs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Delete removes the blob; a missing key is not an error.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blobs, key)
	return nil
}

// Presign is unsupported by the in-process store and reports ErrNotSupported.
func (m *Memory) Presign(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotSupported
}

// Size reports the stored blob's byte count.
func (m *Memory) Size(_ context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	blob, ok := m.blobs[key]
	if !ok {
		return 0, ErrNotFound
	}
	return int64(len(blob)), nil
}
