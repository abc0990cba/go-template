package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// PlatformDomain is the reserved Casbin domain covering the whole platform;
// organizations use their id as the domain.
const PlatformDomain = "*"

// ErrForbidden is returned when a role or policy change is not allowed
// (system roles, unknown subjects); handlers map it to HTTP 403/400.
var ErrForbidden = errors.New("forbidden")

// Action names used across the catalog and middleware mapping.
const (
	ActionRead    = "read"
	ActionCreate  = "create"
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionExecute = "execute"
)

// Resource is one protectable object with its meaningful actions; the
// roles page renders the matrix from it.
type Resource struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Actions     []string `json:"actions"`
}

// Resources is the permission catalog: every resource the middleware and
// the roles matrix know about. Extend it together with the routes.
var Resources = []Resource{
	{Key: "users", Description: "User accounts", Actions: []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	{Key: "roles", Description: "Roles and their permissions", Actions: []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	{Key: "policies", Description: "Raw policy rules", Actions: []string{ActionRead, ActionCreate, ActionDelete}},
	{Key: "orgs", Description: "Organizations and members", Actions: []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	{Key: "jobs", Description: "Background jobs", Actions: []string{ActionRead, ActionExecute}},
	{Key: "reports", Description: "Analytics reports", Actions: []string{ActionRead, ActionCreate}},
	{Key: "enrichments", Description: "Enrichment profiles", Actions: []string{ActionRead}},
	{Key: "analytics", Description: "Analytics dashboards", Actions: []string{ActionRead}},
	{Key: "audit", Description: "Audit log", Actions: []string{ActionRead}},
	{Key: "settings", Description: "System settings and feature flags", Actions: []string{ActionRead, ActionUpdate}},
	{Key: "webhooks", Description: "Outgoing webhooks", Actions: []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionExecute}},
	{Key: "api_keys", Description: "Personal API keys", Actions: []string{ActionRead, ActionCreate, ActionDelete}},
	{Key: "impersonation", Description: "Sign in as another user", Actions: []string{ActionExecute}},
	{Key: "tickets", Description: "Tickets and approvals", Actions: []string{ActionRead, "read_all", ActionCreate, ActionUpdate, ActionDelete, "approve"}},
	{Key: "ticket_types", Description: "Ticket types and approval chains", Actions: []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	{Key: "streams", Description: "Event streams: outbox and dead letters", Actions: []string{ActionRead, "replay"}},
	{Key: "admin_console", Description: "Access to the /admin console", Actions: []string{"access"}},
}

// systemRoles are seeded at startup; system roles cannot be deleted.
var systemRoles = []model.Role{
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

// RBACService wraps the Casbin enforcer with role metadata, permission
// management, and the seed used at startup. A nil enforcer denies
// everything, which keeps non-HTTP processes safe.
type RBACService struct {
	permsCache struct {
		cache Cache
		ttl   time.Duration
	}
	enforcer *casbin.SyncedEnforcer
	roles    RolesRepo
	orgs     OrgsRepo
	logger   *slog.Logger
}

// Permission is one object/action pair of the catalog or a role matrix.
type Permission struct {
	Object string `json:"object"`
	Action string `json:"action"`
}

// Catalog returns the resource/action matrix the roles page renders.
func (s *RBACService) Catalog() []Resource { return Resources }

// RoleByKey resolves one role by key (used to validate invitation targets).
func (s *RBACService) RoleByKey(key string) (model.Role, error) {
	return s.roles.GetRole(context.Background(), key)
}

// WithCache attaches the expanded-permissions cache (30s, invalidated on
// every policy mutation).
func (s *RBACService) WithCache(cache Cache) *RBACService {
	s.permsCache.cache = cache
	s.permsCache.ttl = CacheTTLPerms
	return s
}

// Enforce answers whether the subject may perform act on obj in dom.
func (s *RBACService) Enforce(sub, dom, obj, act string) bool {
	if s.enforcer == nil {
		return false
	}
	ok, err := s.enforcer.Enforce(sub, dom, obj, act)
	if err != nil {
		s.log("enforce failed", err, "sub", sub, "dom", dom, "obj", obj, "act", act)
		return false
	}
	return ok
}

// IsAdmin reports whether the subject has full access in the domain — the
// membership bypass used by the domain middleware.
func (s *RBACService) IsAdmin(sub, dom string) bool {
	return s.Enforce(sub, dom, "*", "*")
}

// HasRole reports whether the subject holds the role in the domain. The
// ticket approval guards use it to check the current step's approver role.
func (s *RBACService) HasRole(sub, dom, role string) bool {
	if s.enforcer == nil || sub == "" || role == "" {
		return false
	}
	for _, held := range s.enforcer.GetRolesForUserInDomain(sub, dom) {
		if held == role {
			return true
		}
	}
	return false
}

// UsersWithRole lists every subject holding the role in the domain; the
// ticket service notifies them when a step starts waiting for their role.
func (s *RBACService) UsersWithRole(dom, role string) []string {
	if s.enforcer == nil || role == "" {
		return nil
	}
	users := s.enforcer.GetUsersForRoleInDomain(role, dom)
	return users
}

// ReloadPolicies re-reads every rule from the store into the enforcer.
// Raw-store mutations (the demo reset deletes grouping rules directly)
// leave the cached model stale, and casbin then silently skips re-adding
// rules it still believes exist — a reload re-syncs the two.
func (s *RBACService) ReloadPolicies() error {
	if s.enforcer == nil {
		return nil
	}
	return s.enforcer.LoadPolicy()
}

// RolesForUser lists the role keys the subject holds in the domain; the
// ticket inbox matches the current step's approver role against them.
func (s *RBACService) RolesForUser(sub, dom string) []string {
	if s.enforcer == nil || sub == "" {
		return nil
	}
	roles := s.enforcer.GetRolesForUserInDomain(sub, dom)
	sort.Strings(roles)
	return roles
}

// Roles lists the role metadata.
func (s *RBACService) Roles(ctx context.Context) ([]model.Role, error) {
	return s.roles.ListRoles(ctx)
}

// CreateRole adds a custom role; keys are lowercase slugs.
func (s *RBACService) CreateRole(ctx context.Context, key, name, description string) (model.Role, error) {
	if key == "" || name == "" {
		return model.Role{}, validationError("role key and name are required")
	}
	role := model.Role{Key: key, Name: name, Description: description, CreatedAt: time.Now()}
	if err := s.roles.CreateRole(ctx, role); err != nil {
		return model.Role{}, err
	}
	return role, nil
}

// UpdateRole rewrites display metadata of a non-system role.
func (s *RBACService) UpdateRole(ctx context.Context, key, name, description string) error {
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
func (s *RBACService) DeleteRole(ctx context.Context, key string) error {
	defer s.cacheInvalidateAll()
	role, err := s.roles.GetRole(ctx, key)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return ErrForbidden
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
func (s *RBACService) RolePermissions(key string) ([]model.PolicyRule, error) {
	rules, err := s.enforcer.GetFilteredPolicy(0, key)
	if err != nil {
		return nil, err
	}
	return rulesToPolicyRules("p", rules), nil
}

// SetRolePermissions replaces the role's permission set in one domain.
func (s *RBACService) SetRolePermissions(key, domain string, perms []Permission) error {
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
func (s *RBACService) UserRoles(userID, domain string) ([]model.PolicyRule, error) {
	var rules []model.PolicyRule
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
			rules = append(rules, model.PolicyRule{PType: "g", Subject: userID, Domain: dom, Object: role})
		}
	}
	return rules, nil
}

// AssignRole grants the role to the user in the domain.
func (s *RBACService) AssignRole(userID, key, domain string) error {
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
func (s *RBACService) UnassignRole(userID, key, domain string) error {
	s.cacheInvalidateUser(domain, userID)
	if domain == "" {
		domain = PlatformDomain
	}
	_, err := s.enforcer.DeleteRoleForUserInDomain(userID, key, domain)
	return err
}

// ListPolicies returns the raw rules of one ptype for the policy editor.
func (s *RBACService) ListPolicies(ctx context.Context, ptype string) ([]model.PolicyRule, error) {
	if ptype != "p" && ptype != "g" {
		return nil, validationError("ptype must be p or g")
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
func (s *RBACService) AddPolicyRule(rule model.PolicyRule) error {
	defer s.cacheInvalidateAll()
	if rule.Domain == "" {
		rule.Domain = PlatformDomain
	}
	if rule.Subject == "" {
		return validationError("subject is required")
	}
	if rule.PType == "p" {
		_, err := s.enforcer.AddPolicy(rule.Subject, rule.Domain, rule.Object, rule.Action)
		return err
	}
	_, err := s.enforcer.AddGroupingPolicy(rule.Subject, rule.Object, rule.Domain)
	return err
}

// RemovePolicyRule deletes one raw rule.
func (s *RBACService) RemovePolicyRule(rule model.PolicyRule) error {
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

// EffectivePermissions expands the user's rules in the domain into
// concrete catalog permissions ("obj:act"), so the UI can gate buttons
// with plain string checks.
func (s *RBACService) EffectivePermissions(userID, domain string) []string {
	if s.enforcer == nil || userID == "" {
		return nil
	}
	// The expanded set is pure policy math: cacheable for a bounded window.
	// Every mutation below invalidates the affected keys explicitly, so the
	// TTL is only the cross-replica backstop.
	if perms, ok := s.cacheGet(PermsCacheKey(domain, userID)); ok {
		return perms
	}
	domains := []string{domain, PlatformDomain}
	if domain == "" || domain == PlatformDomain {
		domains = domains[:1]
	}
	allowed := map[string]bool{}
	for _, dom := range domains {
		for _, role := range s.enforcer.GetRolesForUserInDomain(userID, dom) {
			for _, rule := range s.enforcer.GetPermissionsForUserInDomain(role, dom) {
				if len(rule) < 4 {
					continue
				}
				expandPermission(allowed, rule[2], rule[3])
			}
		}
	}
	out := make([]string, 0, len(allowed))
	for perm := range allowed {
		out = append(out, perm)
	}
	sort.Strings(out)
	s.cacheSet(PermsCacheKey(domain, userID), out)
	return out
}

func (s *RBACService) cacheGet(key string) ([]string, bool) {
	if s.permsCache.cache == nil {
		return nil, false
	}
	raw, ok := s.permsCache.cache.Get(context.Background(), key)
	if !ok {
		return nil, false
	}
	var perms []string
	if err := json.Unmarshal(raw, &perms); err != nil {
		return nil, false
	}
	return perms, true
}

func (s *RBACService) cacheSet(key string, perms []string) {
	if s.permsCache.cache == nil {
		return
	}
	if raw, err := json.Marshal(perms); err == nil {
		s.permsCache.cache.Set(context.Background(), key, raw, s.permsCache.ttl)
	}
}

func (s *RBACService) cacheInvalidateUser(domain, userID string) {
	if s.permsCache.cache != nil {
		s.permsCache.cache.Delete(context.Background(), PermsCacheKey(domain, userID), PermsCacheKey(PlatformDomain, userID))
	}
}

func (s *RBACService) cacheInvalidateAll() {
	if s.permsCache.cache != nil {
		s.permsCache.cache.DeletePrefix(context.Background(), CacheKeyPerms)
	}
}

// expandPermission turns one policy rule into concrete "obj:act" entries,
// resolving wildcards against the catalog.
func expandPermission(allowed map[string]bool, objPattern, actPattern string) {
	for _, resource := range Resources {
		if !matchPattern(resource.Key, objPattern) {
			continue
		}
		for _, action := range resource.Actions {
			if matchPattern(action, actPattern) || actPattern == "*" {
				allowed[resource.Key+":"+action] = true
			}
		}
	}
}

// matchPattern is the keyMatch2 subset the catalog needs: exact match,
// trailing "*", or ":*" wildcards.
func matchPattern(value, pattern string) bool {
	if pattern == "*" || value == pattern {
		return true
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return len(value) >= len(pattern)-1 && value[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return false
}

// OrgsForUser lists the organizations the user belongs to.
func (s *RBACService) OrgsForUser(ctx context.Context, userID string) ([]model.Organization, error) {
	if s.orgs == nil {
		return nil, repository.ErrNotFound
	}
	return s.orgs.ListOrgsForUser(ctx, userID)
}

// OrgMember reports membership of the user in the organization.
func (s *RBACService) OrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	if s.orgs == nil {
		return false, repository.ErrNotFound
	}
	if _, err := s.orgs.GetOrg(ctx, orgID); err != nil {
		return false, err
	}
	return s.orgs.IsOrgMember(ctx, orgID, userID)
}

// ListOrgs pages organizations for the admin console.
func (s *RBACService) ListOrgs(ctx context.Context, opts repository.ListOptions) ([]model.Organization, string, error) {
	return s.orgs.ListOrgs(ctx, opts)
}

// CreateOrg adds an organization and, when requested, puts the creator in it.
func (s *RBACService) CreateOrg(ctx context.Context, name, slug string) (model.Organization, error) {
	if name == "" {
		return model.Organization{}, validationError("organization name is required")
	}
	if slug == "" {
		slug = slugify(name)
	}
	if !validSlug(slug) {
		return model.Organization{}, validationError("slug must be lowercase letters, digits, and dashes")
	}
	now := time.Now()
	org := model.Organization{ID: newID(), Name: name, Slug: slug, CreatedAt: now, UpdatedAt: now}
	if err := s.orgs.CreateOrg(ctx, org); err != nil {
		return model.Organization{}, err
	}
	return org, nil
}

// GetOrg returns one organization.
func (s *RBACService) GetOrg(ctx context.Context, id string) (model.Organization, error) {
	return s.orgs.GetOrg(ctx, id)
}

// UpdateOrg rewrites the organization.
func (s *RBACService) UpdateOrg(ctx context.Context, id, name, slug string) error {
	org, err := s.orgs.GetOrg(ctx, id)
	if err != nil {
		return err
	}
	if name != "" {
		org.Name = name
	}
	if slug != "" {
		if !validSlug(slug) {
			return validationError("slug must be lowercase letters, digits, and dashes")
		}
		org.Slug = slug
	}
	return s.orgs.UpdateOrg(ctx, org)
}

// DeleteOrg removes the organization.
func (s *RBACService) DeleteOrg(ctx context.Context, id string) error {
	return s.orgs.DeleteOrg(ctx, id)
}

// OrgMembers lists an organization's members with their user snapshot.
func (s *RBACService) OrgMembers(ctx context.Context, orgID string) ([]model.OrgMember, error) {
	return s.orgs.ListOrgMembers(ctx, orgID)
}

// AddOrgMember attaches the user to the organization.
func (s *RBACService) AddOrgMember(ctx context.Context, orgID, userID string) error {
	return s.orgs.AddOrgMember(ctx, model.OrgMember{OrgID: orgID, UserID: userID, JoinedAt: time.Now()})
}

// RemoveOrgMember detaches the user from the organization; their role
// assignments in the domain are revoked too.
func (s *RBACService) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	for _, role := range s.enforcer.GetRolesForUserInDomain(userID, orgID) {
		_, _ = s.enforcer.DeleteRoleForUserInDomain(userID, role, orgID)
	}
	return s.orgs.RemoveOrgMember(ctx, orgID, userID)
}

// validSlug accepts lowercase letters, digits, and dashes.
func validSlug(slug string) bool {
	if slug == "" {
		return false
	}
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// slugify lowercases and dashes a name for a default slug.
func slugify(name string) string {
	var b []rune
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b = append(b, r+('a'-'A'))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b = append(b, r)
		default:
			b = append(b, '-')
		}
	}
	return strings.Trim(string(b), "-")
}

// EnsureSystemRoles seeds the role metadata and default policies,
// idempotently; safe on every startup.
func (s *RBACService) EnsureSystemRoles(ctx context.Context) error {
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

// rulesToPolicyRules converts raw Casbin rule arrays into domain structs.
// Grouping rules are [user, role, domain]; policies [sub, dom, obj, act].
func rulesToPolicyRules(ptype string, rules [][]string) []model.PolicyRule {
	out := make([]model.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		switch {
		case ptype == "g" && len(rule) >= 3:
			out = append(out, model.PolicyRule{PType: ptype, Subject: rule[0], Object: rule[1], Domain: rule[2]})
		case len(rule) >= 4:
			out = append(out, model.PolicyRule{PType: ptype, Subject: rule[0], Domain: rule[1], Object: rule[2], Action: rule[3]})
		}
	}
	return out
}

func (s *RBACService) log(msg string, err error, args ...any) {
	if s.logger == nil {
		return
	}
	if err != nil {
		s.logger.Error(msg, append(args, "err", err)...)
		return
	}
	s.logger.Info(msg, args...)
}
