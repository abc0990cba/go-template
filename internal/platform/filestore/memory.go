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
	mu    sync.RWMutex
	blobs map[string][]byte
}

// NewMemory builds the in-process store.
func NewMemory() *Memory {
	return &Memory{blobs: map[string][]byte{}}
}

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

func (m *Memory) Open(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.blobs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blobs, key)
	return nil
}

func (m *Memory) Presign(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotSupported
}
