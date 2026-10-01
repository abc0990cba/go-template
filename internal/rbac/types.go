package rbac

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Organization is a tenant: a Casbin domain grouping users and scoping
// policies. The reserved domain "*" means the whole platform.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgMember is one membership with the user snapshot the members list
// renders. Role assignments live in the policy engine, not here.
type OrgMember struct {
	OrgID    string    `json:"org_id"`
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	JoinedAt time.Time `json:"joined_at"`
}

// Role is the human-readable metadata of a Casbin role key. System roles
// cannot be renamed away or deleted from the admin UI.
type Role struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
}

// PolicyRule is one stored policy (ptype "p": subject, domain, object,
// action) or grouping rule (ptype "g": user, role, domain) as shown by the
// raw policy editor.
type PolicyRule struct {
	PType   string `json:"ptype"`
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object,omitempty"`
	Action  string `json:"action,omitempty"`
}

// OrgsStore manages organizations (Casbin domains) and their memberships.
type OrgsStore interface {
	CreateOrg(ctx context.Context, org Organization) error
	GetOrg(ctx context.Context, id string) (Organization, error)
	UpdateOrg(ctx context.Context, org Organization) error
	DeleteOrg(ctx context.Context, id string) error
	ListOrgs(ctx context.Context, opts kernel.ListOptions) ([]Organization, string, error)
	ListOrgsForUser(ctx context.Context, userID string) ([]Organization, error)
	AddOrgMember(ctx context.Context, member OrgMember) error
	RemoveOrgMember(ctx context.Context, orgID, userID string) error
	IsOrgMember(ctx context.Context, orgID, userID string) (bool, error)
	ListOrgMembers(ctx context.Context, orgID string) ([]OrgMember, error)
}

// RolesStore persists role metadata — the display half of RBAC. The
// permissions themselves are enforced by the policy engine.
type RolesStore interface {
	CreateRole(ctx context.Context, role Role) error
	GetRole(ctx context.Context, key string) (Role, error)
	UpdateRole(ctx context.Context, role Role) error
	DeleteRole(ctx context.Context, key string) error
	ListRoles(ctx context.Context) ([]Role, error)
}

// RuleStore persists Casbin policy and grouping rules; the Casbin adapter in
// internal/platform/rbac builds on it. Implementations keep the same rule
// surface as the casbin_rule table: ptype plus up to four values.
type RuleStore interface {
	ListPolicyRules(ctx context.Context, ptype string) ([]PolicyRule, error)
	InsertPolicyRule(ctx context.Context, rule PolicyRule) error
	DeletePolicyRule(ctx context.Context, rule PolicyRule) error
	DeleteFilteredPolicyRules(ctx context.Context, ptype, subject, domain string) error
	DeletePolicyRulesByType(ctx context.Context, ptype string) error
}
