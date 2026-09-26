package repository

import (
	"context"

	"github.com/your-org/go-template/internal/model"
)

// RuleStore persists Casbin policy and grouping rules; the RBAC adapter in
// internal/platform/rbac builds on it. Implementations keep the same rule
// surface as the casbin_rule table: ptype plus up to four values.
type RuleStore interface {
	ListPolicyRules(ctx context.Context, ptype string) ([]model.PolicyRule, error)
	InsertPolicyRule(ctx context.Context, rule model.PolicyRule) error
	DeletePolicyRule(ctx context.Context, rule model.PolicyRule) error
	DeleteFilteredPolicyRules(ctx context.Context, ptype, subject, domain string) error
	DeletePolicyRulesByType(ctx context.Context, ptype string) error
}

// Compile-time proof the memory fake stays interchangeable with the
// PostgreSQL adapter for policy persistence.
var _ RuleStore = (*MemoryStore)(nil)
