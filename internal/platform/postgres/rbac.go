// RBAC and organization persistence: role metadata, Casbin policy rules,
// and organization memberships. The Casbin adapter in internal/platform/rbac
// builds directly on the RuleStore methods here.
package postgres

import (
	"context"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

var (
	_ service.OrgsRepo  = (*Store)(nil)
	_ service.RolesRepo = (*Store)(nil)
	_ repository.RuleStore = (*Store)(nil)
)

// CreateOrg stores the organization (idempotent on id).
func (s *Store) CreateOrg(ctx context.Context, org model.Organization) error {
	return mapErr(s.q.CreateOrg(ctx, db.CreateOrgParams{
		ID: org.ID, Name: org.Name, Slug: org.Slug, CreatedAt: org.CreatedAt, UpdatedAt: org.UpdatedAt,
	}))
}

// GetOrg returns the organization by id.
func (s *Store) GetOrg(ctx context.Context, id string) (model.Organization, error) {
	row, err := s.q.GetOrg(ctx, id)
	if err != nil {
		return model.Organization{}, mapErr(err)
	}
	return model.Organization{ID: row.ID, Name: row.Name, Slug: row.Slug, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

// UpdateOrg rewrites name and slug and refreshes updated_at.
func (s *Store) UpdateOrg(ctx context.Context, org model.Organization) error {
	err := s.q.UpdateOrg(ctx, db.UpdateOrgParams{ID: org.ID, Name: org.Name, Slug: org.Slug})
	return mapErr(err)
}

// DeleteOrg removes the organization; memberships cascade.
func (s *Store) DeleteOrg(ctx context.Context, id string) error {
	n, err := s.q.DeleteOrg(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// ListOrgs pages organizations newest first.
func (s *Store) ListOrgs(ctx context.Context, opts repository.ListOptions) ([]model.Organization, string, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListOrgsOffset(ctx, db.ListOrgsOffsetParams{Query: opts.Query, Off: off, Lim: limit + 1})
	if err != nil {
		return nil, "", mapErr(err)
	}
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	orgs := make([]model.Organization, 0, len(rows))
	for _, row := range rows {
		orgs = append(orgs, model.Organization{ID: row.ID, Name: row.Name, Slug: row.Slug, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return orgs, next, nil
}

// ListOrgsForUser returns the organizations the user belongs to, name order.
func (s *Store) ListOrgsForUser(ctx context.Context, userID string) ([]model.Organization, error) {
	rows, err := s.q.ListOrgsForUser(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	orgs := make([]model.Organization, 0, len(rows))
	for _, row := range rows {
		orgs = append(orgs, model.Organization{ID: row.ID, Name: row.Name, Slug: row.Slug, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return orgs, nil
}

// AddOrgMember records the membership (idempotent on the pair).
func (s *Store) AddOrgMember(ctx context.Context, member model.OrgMember) error {
	err := s.q.AddOrgMember(ctx, db.AddOrgMemberParams{OrgID: member.OrgID, UserID: member.UserID, JoinedAt: member.JoinedAt})
	return mapErr(err)
}

// RemoveOrgMember detaches the user from the organization.
func (s *Store) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	n, err := s.q.RemoveOrgMember(ctx, db.RemoveOrgMemberParams{OrgID: orgID, UserID: userID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// IsOrgMember reports membership.
func (s *Store) IsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	member, err := s.q.IsOrgMember(ctx, db.IsOrgMemberParams{OrgID: orgID, UserID: userID})
	if err != nil {
		return false, mapErr(err)
	}
	return member, nil
}

// ListOrgMembers returns the members with their user snapshot.
func (s *Store) ListOrgMembers(ctx context.Context, orgID string) ([]model.OrgMember, error) {
	rows, err := s.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, mapErr(err)
	}
	members := make([]model.OrgMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, model.OrgMember{
			OrgID: row.OrgID, UserID: row.UserID, Email: row.Email, Name: row.Name, JoinedAt: row.JoinedAt,
		})
	}
	return members, nil
}

// CreateRole stores role metadata (idempotent on key — re-seeding a system
// role must not fail).
func (s *Store) CreateRole(ctx context.Context, role model.Role) error {
	return mapErr(s.q.CreateRole(ctx, db.CreateRoleParams{
		Key: role.Key, Name: role.Name, Description: role.Description, IsSystem: role.IsSystem,
	}))
}

// GetRole returns role metadata by key.
func (s *Store) GetRole(ctx context.Context, key string) (model.Role, error) {
	row, err := s.q.GetRole(ctx, key)
	if err != nil {
		return model.Role{}, mapErr(err)
	}
	return model.Role{Key: row.Key, Name: row.Name, Description: row.Description, IsSystem: row.IsSystem, CreatedAt: row.CreatedAt}, nil
}

// UpdateRole rewrites display metadata; keys and the system flag are
// immutable.
func (s *Store) UpdateRole(ctx context.Context, role model.Role) error {
	return mapErr(s.q.UpdateRole(ctx, db.UpdateRoleParams{Key: role.Key, Name: role.Name, Description: role.Description}))
}

// DeleteRole removes role metadata.
func (s *Store) DeleteRole(ctx context.Context, key string) error {
	n, err := s.q.DeleteRole(ctx, key)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// ListRoles returns every role, creation order.
func (s *Store) ListRoles(ctx context.Context) ([]model.Role, error) {
	rows, err := s.q.ListRoles(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	roles := make([]model.Role, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, model.Role{Key: row.Key, Name: row.Name, Description: row.Description, IsSystem: row.IsSystem, CreatedAt: row.CreatedAt})
	}
	return roles, nil
}

// ListPolicyRules returns stored rules of one ptype in insertion order.
func (s *Store) ListPolicyRules(ctx context.Context, ptype string) ([]model.PolicyRule, error) {
	rows, err := s.q.ListPolicyRules(ctx, ptype)
	if err != nil {
		return nil, mapErr(err)
	}
	rules := make([]model.PolicyRule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, model.PolicyRule{PType: row.Ptype, Subject: row.V0, Domain: row.V1, Object: row.V2, Action: row.V3})
	}
	return rules, nil
}

// InsertPolicyRule appends one rule.
func (s *Store) InsertPolicyRule(ctx context.Context, rule model.PolicyRule) error {
	return mapErr(s.q.InsertPolicyRule(ctx, db.InsertPolicyRuleParams{
		Ptype: rule.PType, V0: rule.Subject, V1: rule.Domain, V2: rule.Object, V3: rule.Action,
	}))
}

// DeletePolicyRule removes the exact rule; a missing rule is not an error
// (concurrent enforcers may race the HasPolicy check).
func (s *Store) DeletePolicyRule(ctx context.Context, rule model.PolicyRule) error {
	_, err := s.q.DeletePolicyRule(ctx, db.DeletePolicyRuleParams{
		Ptype: rule.PType, V0: rule.Subject, V1: rule.Domain, V2: rule.Object, V3: rule.Action,
	})
	return mapErr(err)
}

// DeleteFilteredPolicyRules removes every rule of the ptype with the given
// subject and domain — replacing a role's permission set in one domain.
func (s *Store) DeleteFilteredPolicyRules(ctx context.Context, ptype, subject, domain string) error {
	_, err := s.q.DeletePolicyRulesByPrefix(ctx, db.DeletePolicyRulesByPrefixParams{Ptype: ptype, V0: subject, V1: domain})
	return mapErr(err)
}

// DeletePolicyRulesByType removes every rule of the ptype (adapter
// SavePolicy rebuild).
func (s *Store) DeletePolicyRulesByType(ctx context.Context, ptype string) error {
	_, err := s.q.DeleteAllPolicyRules(ctx, ptype)
	return mapErr(err)
}

// DeleteDemoGroupRules drops demo role assignments (demo reset).
func (s *Store) DeleteDemoGroupRules(ctx context.Context) error {
	_, err := s.q.DeleteDemoGroupRules(ctx)
	return mapErr(err)
}

// DeleteDemoOrgRules drops demo org-scoped policies (demo reset).
func (s *Store) DeleteDemoOrgRules(ctx context.Context) error {
	_, err := s.q.DeleteDemoOrgRules(ctx)
	return mapErr(err)
}
