// File metadata persistence for the in-memory store: an append-only slice
// with soft-delete tombstones (the blobs live in the file store adapter).
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/kernel"
)

// --- File metadata ----------------------------------------------------------

// Compile-time proof the in-memory fake stays interchangeable with any
// adapter, including the PostgreSQL one.
var _ files.Store = (*MemoryStore)(nil)

func (s *MemoryStore) InsertFile(_ context.Context, file files.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, file)
	return nil
}

func (s *MemoryStore) GetFile(_ context.Context, id string) (files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, file := range s.files {
		if file.ID == id && file.DeletedAt == nil {
			return file, nil
		}
	}
	return files.File{}, fmt.Errorf("%w: file not found", kernel.ErrNotFound)
}

func (s *MemoryStore) ListFilesByRef(_ context.Context, refType, refID string) ([]files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []files.File
	for _, file := range s.files {
		if file.RefType == refType && file.RefID == refID && file.DeletedAt == nil {
			out = append(out, file)
		}
	}
	return out, nil
}

func (s *MemoryStore) ListFilesByOwner(_ context.Context, ownerID string, limit, offset int) ([]files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var mine []files.File
	for _, file := range s.files {
		if file.OwnerID == ownerID && file.DeletedAt == nil {
			mine = append(mine, file)
		}
	}
	if offset >= len(mine) {
		return []files.File{}, nil
	}
	end := offset + limit
	if end > len(mine) {
		end = len(mine)
	}
	return mine[offset:end], nil
}

func (s *MemoryStore) SoftDeleteFile(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, file := range s.files {
		if file.ID == id {
			now := time.Now()
			s.files[i].DeletedAt = &now
			return nil
		}
	}
	return fmt.Errorf("%w: file not found", kernel.ErrNotFound)
}

func (s *MemoryStore) ListExportFilesOlderThan(_ context.Context, olderThan time.Time) ([]files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []files.File
	for _, file := range s.files {
		if file.RefType == "export" && file.DeletedAt == nil && file.CreatedAt.Before(olderThan) {
			out = append(out, file)
		}
	}
	return out, nil
}
