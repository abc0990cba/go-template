// Role and policy administration: the system role seed, the role metadata
// CRUD, the permission matrix editor, and the raw policy rules.

package rbac

import (
	"context"
	"sort"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// systemRoles are seeded at startup; system roles cannot be deleted.
var systemRoles = []Role{
	{Key: "superadmin", Name: "Super Admin", Description: "Unrestricted platform access", IsSystem: true},
	{Key: "admin", Name: "Administrator", Description: "Manages users, organizations, and settings", IsSystem: true},
	{Key: "manager", Name: "Manager", Description: "Operates jobs and reports, edits users, approves tickets", IsSystem: true},
	{Key: "requester", Name: "Requester", Description: "Files, tracks, and completes own tickets", IsSystem: true},
	{Key: "approver", Name: "Approver", Description: "Decides on approval steps of tickets", IsSystem: true},
	{Key: "viewer", Name: "Viewer", Description: "Read-only access", IsSystem: true},
}

// systemPolicies are the default permission sets of the system roles,
// expressed in the platform domain "*" (they apply everywhere unless a
// tenant overrides a custom role).
var systemPolicies = [][]string{
	{"superadmin", PlatformDomain, "*", "*"},

	{"admin", PlatformDomain, "users", "*"},
	{"admin", PlatformDomain, "roles", "*"},
	{"admin", PlatformDomain, "orgs", "*"},
	{"admin", PlatformDomain, "jobs", "*"},
	{"admin", PlatformDomain, "reports", "*"},
	{"admin", PlatformDomain, "enrichments", ActionRead},
	{"admin", PlatformDomain, "analytics", ActionRead},
	{"admin", PlatformDomain, "audit", ActionRead},
	{"admin", PlatformDomain, "settings", "*"},
	{"admin", PlatformDomain, "webhooks", "*"},
	{"admin", PlatformDomain, "api_keys", "*"},
	{"admin", PlatformDomain, "impersonation", ActionExecute},
	{"admin", PlatformDomain, "tickets", "*"},
	{"admin", PlatformDomain, "ticket_types", "*"},
	{"admin", PlatformDomain, "streams", "*"},
	{"admin", PlatformDomain, "emails", "*"},
	{"admin", PlatformDomain, "admin_console", "access"},

	{"manager", PlatformDomain, "users", ActionRead},
	{"manager", PlatformDomain, "users", ActionUpdate},
	{"manager", PlatformDomain, "jobs", ActionRead},
	{"manager", PlatformDomain, "jobs", ActionExecute},
	{"manager", PlatformDomain, "reports", ActionRead},
	{"manager", PlatformDomain, "reports", ActionCreate},
	{"manager", PlatformDomain, "enrichments", ActionRead},
	{"manager", PlatformDomain, "analytics", ActionRead},
	{"manager", PlatformDomain, "tickets", ActionRead},
	{"manager", PlatformDomain, "tickets", "read_all"},
	{"manager", PlatformDomain, "tickets", ActionCreate},
	{"manager", PlatformDomain, "tickets", ActionUpdate},
	{"manager", PlatformDomain, "tickets", ActionDelete},
	{"manager", PlatformDomain, "tickets", "approve"},
	{"manager", PlatformDomain, "ticket_types", ActionRead},
	{"manager", PlatformDomain, "streams", ActionRead},
	{"manager", PlatformDomain, "streams", "replay"},
	{"manager", PlatformDomain, "admin_console", "access"},

	// The two sides of the ticket flow: requesters file and track their own
	// tickets, approvers decide on the steps assigned to their role.
	{"requester", PlatformDomain, "tickets", ActionRead},
	{"requester", PlatformDomain, "tickets", ActionCreate},
	{"requester", PlatformDomain, "tickets", ActionUpdate},
	{"requester", PlatformDomain, "tickets", ActionDelete},
	{"approver", PlatformDomain, "tickets", ActionRead},
	{"approver", PlatformDomain, "tickets", "approve"},

	{"viewer", PlatformDomain, "jobs", ActionRead},
	{"viewer", PlatformDomain, "reports", ActionRead},
	{"viewer", PlatformDomain, "enrichments", ActionRead},
	{"viewer", PlatformDomain, "analytics", ActionRead},
	{"viewer", PlatformDomain, "tickets", ActionRead},
	{"viewer", PlatformDomain, "admin_console", "access"},
}

// EnsureSystemRoles seeds the role metadata and default policies,
// idempotently; safe on every startup.
func (s *Service) EnsureSystemRoles(ctx context.Context) error {
	for _, role := range systemRoles {
		if err := s.roles.CreateRole(ctx, role); err != nil {
			return err
		}
	}
	for _, rule := range systemPolicies {
		if len(rule) != 4 {
			continue
		}
		added, err := s.enforcer.AddPolicy(rule[0], rule[1], rule[2], rule[3])
		if err != nil {
			return err
		}
		if added {
			s.log("seeded system policy", nil, "rule", rule)
		}
	}
	return nil
}

// Roles lists the role metadata.
func (s *Service) Roles(ctx context.Context) ([]Role, error) {
	return s.roles.ListRoles(ctx)
}

// CreateRole adds a custom role; keys are lowercase slugs.
func (s *Service) CreateRole(ctx context.Context, key, name, description string) (Role, error) {
	if key == "" || name == "" {
		return Role{}, kernel.Invalid("role key and name are required")
	}
	role := Role{Key: key, Name: name, Description: description, CreatedAt: time.Now()}
	if err := s.roles.CreateRole(ctx, role); err != nil {
		return Role{}, err
	}
	return role, nil
}

// UpdateRole rewrites display metadata of a non-system role.
func (s *Service) UpdateRole(ctx context.Context, key, name, description string) error {
	role, err := s.roles.GetRole(ctx, key)
	if err != nil {
		return err
	}
	if name != "" {
		role.Name = name
	}
	role.Description = description
	return s.roles.UpdateRole(ctx, role)
}

// DeleteRole removes a custom role together with its policies and
// assignments. System roles are protected.
func (s *Service) DeleteRole(ctx context.Context, key string) error {
	defer s.cacheInvalidateAll()
	role, err := s.roles.GetRole(ctx, key)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return kernel.ErrForbidden
	}
	if _, err := s.enforcer.RemoveFilteredPolicy(0, key); err != nil {
		return err
	}
	if _, err := s.enforcer.RemoveFilteredGroupingPolicy(1, key); err != nil {
		return err
	}
	return s.roles.DeleteRole(ctx, key)
}

// RolePermissions lists the raw p rules granted to the role across all
// domains.
func (s *Service) RolePermissions(key string) ([]PolicyRule, error) {
	rules, err := s.enforcer.GetFilteredPolicy(0, key)
	if err != nil {
		return nil, err
	}
	return rulesToPolicyRules("p", rules), nil
}

// SetRolePermissions replaces the role's permission set in one domain.
func (s *Service) SetRolePermissions(key, domain string, perms []Permission) error {
	defer s.cacheInvalidateAll()
	if domain == "" {
		domain = PlatformDomain
	}
	if _, err := s.roles.GetRole(context.Background(), key); err != nil {
		return err
	}
	if _, err := s.enforcer.RemoveFilteredPolicy(1, key, domain); err != nil {
		return err
	}
	for _, perm := range perms {
		if _, err := s.enforcer.AddPolicy(key, domain, perm.Object, perm.Action); err != nil {
			return err
		}
	}
	return nil
}

// UserRoles lists the roles of a user in the domain plus the platform
// domain.
func (s *Service) UserRoles(userID, domain string) ([]PolicyRule, error) {
	var rules []PolicyRule
	domains := []string{domain, PlatformDomain}
	if domain == PlatformDomain {
		domains = domains[:1]
	}
	seen := map[string]bool{}
	for _, dom := range domains {
		for _, role := range s.enforcer.GetRolesForUserInDomain(userID, dom) {
			id := role + "\x00" + dom
			if seen[id] {
				continue
			}
			seen[id] = true
			rules = append(rules, PolicyRule{PType: "g", Subject: userID, Domain: dom, Object: role})
		}
	}
	return rules, nil
}

// AssignRole grants the role to the user in the domain.
func (s *Service) AssignRole(userID, key, domain string) error {
	s.cacheInvalidateUser(domain, userID)
	if domain == "" {
		domain = PlatformDomain
	}
	if _, err := s.roles.GetRole(context.Background(), key); err != nil {
		return err
	}
	_, err := s.enforcer.AddRoleForUserInDomain(userID, key, domain)
	return err
}

// UnassignRole revokes the role from the user in the domain.
func (s *Service) UnassignRole(userID, key, domain string) error {
	s.cacheInvalidateUser(domain, userID)
	if domain == "" {
		domain = PlatformDomain
	}
	_, err := s.enforcer.DeleteRoleForUserInDomain(userID, key, domain)
	return err
}

// ListPolicies returns the raw rules of one ptype for the policy editor.
func (s *Service) ListPolicies(_ context.Context, ptype string) ([]PolicyRule, error) {
	if ptype != "p" && ptype != "g" {
		return nil, kernel.Invalid("ptype must be p or g")
	}
	rules, err := s.enforcer.GetPolicy()
	if ptype == "g" {
		rules, err = s.enforcer.GetGroupingPolicy()
	}
	if err != nil {
		return nil, err
	}
	out := rulesToPolicyRules(ptype, rules)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		if out[i].Object != out[j].Object {
			return out[i].Object < out[j].Object
		}
		return out[i].Action < out[j].Action
	})
	return out, nil
}

// AddPolicyRule appends one raw rule (p or g).
func (s *Service) AddPolicyRule(rule PolicyRule) error {
	defer s.cacheInvalidateAll()
	if rule.Domain == "" {
		rule.Domain = PlatformDomain
	}
	if rule.Subject == "" {
		return kernel.Invalid("subject is required")
	}
	if rule.PType == "p" {
		_, err := s.enforcer.AddPolicy(rule.Subject, rule.Domain, rule.Object, rule.Action)
		return err
	}
	_, err := s.enforcer.AddGroupingPolicy(rule.Subject, rule.Object, rule.Domain)
	return err
}

// RemovePolicyRule deletes one raw rule.
func (s *Service) RemovePolicyRule(rule PolicyRule) error {
	defer s.cacheInvalidateAll()
	if rule.Domain == "" {
		rule.Domain = PlatformDomain
	}
	if rule.PType == "p" {
		_, err := s.enforcer.RemovePolicy(rule.Subject, rule.Domain, rule.Object, rule.Action)
		return err
	}
	_, err := s.enforcer.RemoveGroupingPolicy(rule.Subject, rule.Object, rule.Domain)
	return err
}

// rulesToPolicyRules converts raw Casbin rule arrays into domain structs.
// Grouping rules are [user, role, domain]; policies [sub, dom, obj, act].
func rulesToPolicyRules(ptype string, rules [][]string) []PolicyRule {
	out := make([]PolicyRule, 0, len(rules))
	for _, rule := range rules {
		switch {
		case ptype == "g" && len(rule) >= 3:
			out = append(out, PolicyRule{PType: ptype, Subject: rule[0], Object: rule[1], Domain: rule[2]})
		case len(rule) >= 4:
			out = append(out, PolicyRule{PType: ptype, Subject: rule[0], Domain: rule[1], Object: rule[2], Action: rule[3]})
		}
	}
	return out
}
