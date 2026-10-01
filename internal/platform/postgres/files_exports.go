package postgres

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/files"
)

func (s *Store) ListExportFilesOlderThan(ctx context.Context, olderThan time.Time) ([]files.File, error) {
	rows, err := s.q.ListExportFilesOlderThan(ctx, olderThan)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]files.File, 0, len(rows))
	for _, row := range rows {
		out = append(out, fileFromRow(row))
	}
	return out, nil
}
