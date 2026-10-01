// Package rbac owns roles, policies, and organizations: the Casbin-backed
// permission engine, its admin HTTP surface, and the seeding of the system
// roles. A nil enforcer denies everything, which keeps non-HTTP processes
// safe.
package rbac

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"time"

	"github.com/casbin/casbin/v2"

	"github.com/your-org/go-template/internal/cache"
)

// PlatformDomain is the reserved Casbin domain covering the whole platform;
// organizations use their id as the domain.
const PlatformDomain = "*"

// ErrForbidden is returned when a role or policy change is not allowed
// (system roles, unknown subjects); handlers map it to HTTP 403/400.
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

// Service wraps the Casbin enforcer with role metadata, permission
// management, and the seed used at startup. A nil enforcer denies
// everything, which keeps non-HTTP processes safe.
type Service struct {
	permsCache struct {
		cache cache.Cache
		ttl   time.Duration
	}
	enforcer *casbin.SyncedEnforcer
	roles    RolesStore
	orgs     OrgsStore
	logger   *slog.Logger
}

// NewService wires the policy engine against its stores. The enforcer may
// be nil (non-HTTP processes): every check then denies.
func NewService(enforcer *casbin.SyncedEnforcer, roles RolesStore, orgs OrgsStore, logger *slog.Logger) *Service {
	return &Service{enforcer: enforcer, roles: roles, orgs: orgs, logger: logger}
}

// Permission is one object/action pair of the catalog or a role matrix.
type Permission struct {
	Object string `json:"object"`
	Action string `json:"action"`
}

// Catalog returns the resource/action matrix the roles page renders.
func (s *Service) Catalog() []Resource { return Resources }

// RoleByKey resolves one role by key (used to validate invitation targets).
func (s *Service) RoleByKey(key string) (Role, error) {
	return s.roles.GetRole(context.Background(), key)
}

// WithCache attaches the expanded-permissions cache (30s, invalidated on
// every policy mutation).
func (s *Service) WithCache(c cache.Cache) *Service {
	s.permsCache.cache = c
	s.permsCache.ttl = cache.CacheTTLPerms
	return s
}

// Enforce answers whether the subject may perform act on obj in dom.
func (s *Service) Enforce(sub, dom, obj, act string) bool {
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
func (s *Service) IsAdmin(sub, dom string) bool {
	return s.Enforce(sub, dom, "*", "*")
}

// HasRole reports whether the subject holds the role in the domain. The
// ticket approval guards use it to check the current step's approver role.
func (s *Service) HasRole(sub, dom, role string) bool {
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
func (s *Service) UsersWithRole(dom, role string) []string {
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
func (s *Service) ReloadPolicies() error {
	if s.enforcer == nil {
		return nil
	}
	return s.enforcer.LoadPolicy()
}

// RolesForUser lists the role keys the subject holds in the domain; the
// ticket inbox matches the current step's approver role against them.
func (s *Service) RolesForUser(sub, dom string) []string {
	if s.enforcer == nil || sub == "" {
		return nil
	}
	roles := s.enforcer.GetRolesForUserInDomain(sub, dom)
	sort.Strings(roles)
	return roles
}

// EffectivePermissions expands the user's rules in the domain into
// concrete catalog permissions ("obj:act"), so the UI can gate buttons
// with plain string checks.
func (s *Service) EffectivePermissions(userID, domain string) []string {
	if s.enforcer == nil || userID == "" {
		return nil
	}
	// The expanded set is pure policy math: cacheable for a bounded window.
	// Every mutation below invalidates the affected keys explicitly, so the
	// TTL is only the cross-replica backstop.
	if perms, ok := s.cacheGet(cache.PermsCacheKey(domain, userID)); ok {
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
	s.cacheSet(cache.PermsCacheKey(domain, userID), out)
	return out
}

func (s *Service) cacheGet(key string) ([]string, bool) {
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

func (s *Service) cacheSet(key string, perms []string) {
	if s.permsCache.cache == nil {
		return
	}
	if raw, err := json.Marshal(perms); err == nil {
		s.permsCache.cache.Set(context.Background(), key, raw, s.permsCache.ttl)
	}
}

func (s *Service) cacheInvalidateUser(domain, userID string) {
	if s.permsCache.cache != nil {
		s.permsCache.cache.Delete(context.Background(), cache.PermsCacheKey(domain, userID), cache.PermsCacheKey(PlatformDomain, userID))
	}
}

func (s *Service) cacheInvalidateAll() {
	if s.permsCache.cache != nil {
		s.permsCache.cache.DeletePrefix(context.Background(), cache.CacheKeyPerms)
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

func (s *Service) log(msg string, err error, args ...any) {
	if s.logger == nil {
		return
	}
	if err != nil {
		s.logger.Error(msg, append(args, "err", err)...)
		return
	}
	s.logger.Info(msg, args...)
}
