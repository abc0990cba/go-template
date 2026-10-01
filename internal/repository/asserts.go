package repository

// Compile-time proof that the in-memory store stays interchangeable with
// any adapter, including the PostgreSQL one; contract tests run the same
// behavioral suites against both.
import (
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/notifications"
)

var (
	_ kernel.Pinger          = (*MemoryStore)(nil)
	_ audit.Store            = (*MemoryStore)(nil)
	_ notifications.Store    = (*MemoryStore)(nil)
	_ kernel.IdempotencyRepo = (*MemoryStore)(nil)
)
