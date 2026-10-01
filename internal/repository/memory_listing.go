package repository

import (
	"github.com/your-org/go-template/internal/kernel"
)

// pageSlice applies the shared pagination strategy to an already sorted
// slice: offset pages by Offset, cursor/keyset resume after the cursor the
// key function returned. Limit is clamped to 1..100 with a 25 default.
func pageSlice[T any](rows []T, key func(T) string, opts kernel.ListOptions) ([]T, string) {
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := 0
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if opts.After != "" {
			for i, row := range rows {
				if key(row) == opts.After {
					start = i + 1
					break
				}
			}
		}
	} else {
		start = max(opts.Offset, 0)
	}
	if start >= len(rows) {
		return nil, ""
	}
	end := min(start+limit, len(rows))
	next := ""
	if end < len(rows) {
		next = key(rows[end-1])
	}
	return rows[start:end], next
}

// tokenPrefix derives the public session id: a short prefix of the opaque
// token, enough to tell sessions apart and useless to resume one.
func tokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}
