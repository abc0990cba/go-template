// Organization, membership, role metadata, and Casbin policy rule
// persistence for the memory fake. The Casbin adapter in
// internal/platform/rbac builds directly on the rule methods here.
package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/rbac"
)

// Compile-time proof the memory fake stays interchangeable with the
// PostgreSQL adapter for policy persistence.
var _ rbac.RuleStore = (*MemoryStore)(nil)

// CreateOrg stores an organization. Mirroring the Postgres constraints:
// slugs conflict (UNIQUE), while a duplicate id is a silent no-op that
// keeps the original (ON CONFLICT (id) DO NOTHING).
func (s *MemoryStore) CreateOrg(ctx context.Context, org rbac.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[org.ID]; ok {
		return nil
	}
	for _, existing := range s.orgs {
		if existing.Slug == org.Slug {
			return kernel.ErrConflict
		}
	}
	if org.CreatedAt.IsZero() {
		org.CreatedAt = time.Now()
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = org.CreatedAt
	}
	s.orgs[org.ID] = org
	return nil
}

// GetOrg returns an organization by id.
func (s *MemoryStore) GetOrg(ctx context.Context, id string) (rbac.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	org, ok := s.orgs[id]
	if !ok {
		return rbac.Organization{}, kernel.ErrNotFound
	}
	return org, nil
}

// UpdateOrg overwrites name and slug, refreshing UpdatedAt.
func (s *MemoryStore) UpdateOrg(ctx context.Context, org rbac.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[org.ID]; !ok {
		return kernel.ErrNotFound
	}
	for id, existing := range s.orgs {
		if id != org.ID && existing.Slug == org.Slug {
			return kernel.ErrConflict
		}
	}
	org.UpdatedAt = time.Now()
	s.orgs[org.ID] = org
	return nil
}

// DeleteOrg removes the organization; memberships cascade.
func (s *MemoryStore) DeleteOrg(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[id]; !ok {
		return kernel.ErrNotFound
	}
	delete(s.orgs, id)
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if member.OrgID != id {
			members = append(members, member)
		}
	}
	s.orgMembers = members
	return nil
}

// ListOrgs returns a page of organizations newest first.
func (s *MemoryStore) ListOrgs(ctx context.Context, opts kernel.ListOptions) ([]rbac.Organization, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	orgs := make([]rbac.Organization, 0, len(s.orgs))
	query := strings.ToLower(opts.Query)
	for _, org := range s.orgs {
		if query != "" && !strings.Contains(strings.ToLower(org.Name+" "+org.Slug), query) {
			continue
		}
		orgs = append(orgs, org)
	}
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].CreatedAt.Equal(orgs[j].CreatedAt) {
			return orgs[i].ID < orgs[j].ID
		}
		return orgs[i].CreatedAt.After(orgs[j].CreatedAt)
	})
	rows, next := pageSlice(orgs, func(org rbac.Organization) string { return org.ID }, opts)
	return rows, next, nil
}

// ListOrgsForUser returns the organizations the user belongs to, name order.
func (s *MemoryStore) ListOrgsForUser(ctx context.Context, userID string) ([]rbac.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var orgs []rbac.Organization
	for _, member := range s.orgMembers {
		if member.UserID != userID {
			continue
		}
		if org, ok := s.orgs[member.OrgID]; ok {
			orgs = append(orgs, org)
		}
	}
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].Name < orgs[j].Name })
	return orgs, nil
}

// AddOrgMember records a membership; duplicates are a no-op and unknown
// organizations or users are kernel.ErrNotFound.
func (s *MemoryStore) AddOrgMember(ctx context.Context, member rbac.OrgMember) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[member.OrgID]; !ok {
		return kernel.ErrNotFound
	}
	if _, ok := s.users[member.UserID]; !ok {
		return kernel.ErrNotFound
	}
	for _, existing := range s.orgMembers {
		if existing.OrgID == member.OrgID && existing.UserID == member.UserID {
			return nil
		}
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	s.orgMembers = append(s.orgMembers, member)
	return nil
}

// RemoveOrgMember detaches the user from the organization.
func (s *MemoryStore) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if member.OrgID == orgID && member.UserID == userID {
			continue
		}
		members = append(members, member)
	}
	s.orgMembers = members
	return nil
}

// IsOrgMember reports membership.
func (s *MemoryStore) IsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, member := range s.orgMembers {
		if member.OrgID == orgID && member.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

// ListOrgMembers returns the members of an organization with their user
// snapshot, oldest join first.
func (s *MemoryStore) ListOrgMembers(ctx context.Context, orgID string) ([]rbac.OrgMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.orgs[orgID]; !ok {
		return nil, kernel.ErrNotFound
	}
	var members []rbac.OrgMember
	for _, member := range s.orgMembers {
		if member.OrgID != orgID {
			continue
		}
		if user, ok := s.users[member.UserID]; ok {
			member.Email = user.Email
			member.Name = user.Name
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].JoinedAt.Before(members[j].JoinedAt) })
	return members, nil
}

// CreateRole stores role metadata; keys are unique, re-seeding is a no-op.
func (s *MemoryStore) CreateRole(ctx context.Context, role rbac.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[role.Key]; ok {
		return nil
	}
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now()
	}
	s.roles[role.Key] = role
	return nil
}

// GetRole returns role metadata by key.
func (s *MemoryStore) GetRole(ctx context.Context, key string) (rbac.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.roles[key]
	if !ok {
		return rbac.Role{}, kernel.ErrNotFound
	}
	return role, nil
}

// UpdateRole overwrites display metadata; keys and the system flag are
// immutable.
func (s *MemoryStore) UpdateRole(ctx context.Context, role rbac.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.roles[role.Key]
	if !ok {
		return kernel.ErrNotFound
	}
	existing.Name = role.Name
	existing.Description = role.Description
	s.roles[role.Key] = existing
	return nil
}

// DeleteRole removes role metadata.
func (s *MemoryStore) DeleteRole(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[key]; !ok {
		return kernel.ErrNotFound
	}
	delete(s.roles, key)
	return nil
}

// ListRoles returns every role, creation order.
func (s *MemoryStore) ListRoles(ctx context.Context) ([]rbac.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roles := make([]rbac.Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].CreatedAt.Before(roles[j].CreatedAt) })
	return roles, nil
}

// ListPolicyRules returns stored rules of one ptype in insertion order —
// the same order the Casbin adapter loads them in.
func (s *MemoryStore) ListPolicyRules(ctx context.Context, ptype string) ([]rbac.PolicyRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rules []rbac.PolicyRule
	for _, rule := range s.rules {
		if rule.PType == ptype {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// InsertPolicyRule appends one rule; duplicates are a no-op so re-seeding
// stays idempotent.
func (s *MemoryStore) InsertPolicyRule(ctx context.Context, rule rbac.PolicyRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.rules {
		if policyRuleEqual(existing, rule) {
			return nil
		}
	}
	s.ruleSeq++
	s.rules = append(s.rules, rule)
	return nil
}

// DeletePolicyRule removes the exact rule.
func (s *MemoryStore) DeletePolicyRule(ctx context.Context, rule rbac.PolicyRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, existing := range s.rules {
		if policyRuleEqual(existing, rule) {
			continue
		}
		next = append(next, existing)
	}
	s.rules = next
	return nil
}

// DeleteFilteredPolicyRules removes every rule of the ptype whose subject
// and domain match — replacing a role's permission set in one domain.
func (s *MemoryStore) DeleteFilteredPolicyRules(ctx context.Context, ptype, subject, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, existing := range s.rules {
		if existing.PType == ptype && existing.Subject == subject && existing.Domain == domain {
			continue
		}
		next = append(next, existing)
	}
	s.rules = next
	return nil
}

// DeletePolicyRulesByType removes every rule of the ptype (adapter
// SavePolicy rebuild).
func (s *MemoryStore) DeletePolicyRulesByType(ctx context.Context, ptype string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, existing := range s.rules {
		if existing.PType != ptype {
			next = append(next, existing)
		}
	}
	s.rules = next
	return nil
}

// DeleteDemoGroupRules drops demo role assignments (demo reset).
func (s *MemoryStore) DeleteDemoGroupRules(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, rule := range s.rules {
		if rule.PType == "g" && strings.HasPrefix(rule.Subject, "demo-") {
			continue
		}
		next = append(next, rule)
	}
	s.rules = next
	return nil
}

// DeleteDemoOrgRules drops demo org-scoped policies (demo reset).
func (s *MemoryStore) DeleteDemoOrgRules(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, rule := range s.rules {
		if rule.PType == "p" && strings.HasPrefix(rule.Domain, "demo-") {
			continue
		}
		next = append(next, rule)
	}
	s.rules = next
	return nil
}

// policyRuleEqual compares the five significant rule columns.
func policyRuleEqual(a, b rbac.PolicyRule) bool {
	return a.PType == b.PType && a.Subject == b.Subject && a.Domain == b.Domain &&
		a.Object == b.Object && a.Action == b.Action
}
