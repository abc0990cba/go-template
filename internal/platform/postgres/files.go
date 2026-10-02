package postgres

import (
	"context"
	"math"

	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

// Compile-time proof the PostgreSQL adapter stays interchangeable with the
// in-memory fake.
var _ files.Store = (*Store)(nil)

// InsertFile stores a file's metadata row.
func (s *Store) InsertFile(ctx context.Context, file files.File) error {
	return mapErr(s.q.InsertFile(ctx, db.InsertFileParams{
		ID:             file.ID,
		OwnerID:        file.OwnerID,
		OrgID:          file.OrgID,
		Key:            file.Key,
		Filename:       file.Filename,
		ContentType:    file.ContentType,
		SizeBytes:      file.SizeBytes,
		ChecksumSha256: file.Checksum,
		RefType:        file.RefType,
		RefID:          file.RefID,
	}))
}

// GetFile returns one file by id.
func (s *Store) GetFile(ctx context.Context, id string) (files.File, error) {
	row, err := s.q.GetFile(ctx, id)
	if err != nil {
		return files.File{}, mapErr(err)
	}
	return fileFromRow(row), nil
}

// ListFilesByRef returns the live files attached to a ticket or export.
func (s *Store) ListFilesByRef(ctx context.Context, refType, refID string) ([]files.File, error) {
	rows, err := s.q.ListFilesByRef(ctx, db.ListFilesByRefParams{RefType: refType, RefID: refID})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]files.File, 0, len(rows))
	for _, row := range rows {
		out = append(out, fileFromRow(row))
	}
	return out, nil
}

// ListFilesByOwner pages one owner's files newest first.
func (s *Store) ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]files.File, error) {
	rows, err := s.q.ListFilesByOwner(ctx, db.ListFilesByOwnerParams{
		OwnerID: ownerID,
		Offset:  int32(min(offset, math.MaxInt32)), // #nosec G115 -- clamped to int32 range
		Limit:   int32(min(limit, math.MaxInt32)),  // #nosec G115 -- clamped to int32 range
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]files.File, 0, len(rows))
	for _, row := range rows {
		out = append(out, fileFromRow(row))
	}
	return out, nil
}

// SoftDeleteFile tombstones the file; the blob is reaped separately.
func (s *Store) SoftDeleteFile(ctx context.Context, id string) error {
	return mapErr(s.q.SoftDeleteFile(ctx, id))
}

func fileFromRow(row db.File) files.File {
	return files.File{
		ID:          row.ID,
		OwnerID:     row.OwnerID,
		OrgID:       row.OrgID,
		Key:         row.Key,
		Filename:    row.Filename,
		ContentType: row.ContentType,
		SizeBytes:   row.SizeBytes,
		Checksum:    row.ChecksumSha256,
		RefType:     row.RefType,
		RefID:       row.RefID,
		CreatedAt:   row.CreatedAt,
	}
}
