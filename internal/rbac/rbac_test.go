package rbac_test

import (
	"testing"
	"time"

	platformrbac "github.com/your-org/go-template/internal/platform/rbac"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/users"
)

// newRBACService builds a Service over a memory-backed policy store with
// the system roles seeded. The enforcer comes from the production adapter:
// the external test package may import platform/rbac (repository imports
// rbac, so a same-package test using MemoryStore would cycle).
func newRBACService(t *testing.T) (*rbac.Service, *repository.MemoryStore) {
	t.Helper()
	store := repository.NewMemoryStore()
	enforcer, err := platformrbac.NewEnforcer(store)
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}
	svc := rbac.NewService(enforcer, store, store, nil)
	if err := svc.EnsureSystemRoles(t.Context()); err != nil {
		t.Fatalf("seed system roles: %v", err)
	}
	return svc, store
}

func TestSystemRolePoliciesAreEnforced(t *testing.T) {
	svc, _ := newRBACService(t)

	cases := []struct {
		role, obj, act string
		want           bool
	}{
		{"admin", "users", "delete", true},
		{"admin", "policies", "create", false}, // raw editor is superadmin-only
		{"manager", "jobs", "execute", true},
		{"manager", "users", "create", false},
		{"viewer", "analytics", "read", true},
		{"viewer", "users", "update", false},
	}
	for i, tc := range cases {
		sub := "subject-" + tc.role // one user per role so grants never mix
		if err := svc.AssignRole(sub, tc.role, rbac.PlatformDomain); err != nil {
			t.Fatalf("assign %s: %v", tc.role, err)
		}
		if got := svc.Enforce(sub, rbac.PlatformDomain, tc.obj, tc.act); got != tc.want {
			t.Errorf("case %d role %s %s:%s = %v, want %v", i, tc.role, tc.obj, tc.act, got, tc.want)
		}
	}
}

func TestDomainScopedRolesAndPlatformWildcard(t *testing.T) {
	svc, _ := newRBACService(t)
	if _, err := svc.CreateRole(t.Context(), "auditor", "Auditor", "reads audit"); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := svc.SetRolePermissions("auditor", "org-1", []rbac.Permission{{Object: "audit", Action: "read"}}); err != nil {
		t.Fatalf("set permissions: %v", err)
	}
	if err := svc.AssignRole("u2", "auditor", "org-1"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if !svc.Enforce("u2", "org-1", "audit", "read") {
		t.Error("domain grant should apply in its domain")
	}
	if svc.Enforce("u2", "org-2", "audit", "read") {
		t.Error("domain grant must not leak to another domain")
	}

	// A platform-domain assignment applies everywhere.
	if err := svc.AssignRole("u3", "manager", rbac.PlatformDomain); err != nil {
		t.Fatalf("assign platform role: %v", err)
	}
	if !svc.Enforce("u3", "org-9", "jobs", "read") {
		t.Error("platform role should apply in any domain")
	}
}

func TestEffectivePermissionsExpandWildcards(t *testing.T) {
	svc, _ := newRBACService(t)
	if err := svc.AssignRole("u1", "superadmin", rbac.PlatformDomain); err != nil {
		t.Fatalf("assign: %v", err)
	}
	perms := svc.EffectivePermissions("u1", rbac.PlatformDomain)
	if len(perms) == 0 {
		t.Fatal("superadmin should expand to concrete permissions")
	}
	found := false
	for _, p := range perms {
		if p == "users:delete" {
			found = true
		}
	}
	if !found {
		t.Errorf("superadmin permissions missing users:delete, got %v", perms)
	}
	if got := svc.EffectivePermissions("nobody", rbac.PlatformDomain); len(got) != 0 {
		t.Errorf("unknown user should have no permissions, got %v", got)
	}
}

func TestDeleteRoleRemovesRulesAndIsBlockedForSystem(t *testing.T) {
	svc, _ := newRBACService(t)
	if err := svc.DeleteRole(t.Context(), "viewer"); err == nil {
		t.Fatal("deleting a system role must fail")
	}
	if _, err := svc.CreateRole(t.Context(), "temp", "Temp", ""); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := svc.SetRolePermissions("temp", rbac.PlatformDomain, []rbac.Permission{{Object: "jobs", Action: "read"}}); err != nil {
		t.Fatalf("set permissions: %v", err)
	}
	if err := svc.AssignRole("u4", "temp", rbac.PlatformDomain); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !svc.Enforce("u4", rbac.PlatformDomain, "jobs", "read") {
		t.Fatal("temp role should grant jobs:read")
	}
	if err := svc.DeleteRole(t.Context(), "temp"); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	if svc.Enforce("u4", rbac.PlatformDomain, "jobs", "read") {
		t.Error("deleted role must not keep granting permissions")
	}
}

func TestOrgMembershipLifecycle(t *testing.T) {
	svc, store := newRBACService(t)
	ctx := t.Context()
	org, err := svc.CreateOrg(ctx, "Acme", "")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if org.Slug != "acme" {
		t.Errorf("slugify: want acme, got %s", org.Slug)
	}
	now := time.Now()
	if err := store.CreateUser(ctx, users.User{ID: "u9", Email: "u9@example.com", CreatedAt: now, UpdatedAt: now}, ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := svc.AddOrgMember(ctx, org.ID, "u9"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	member, err := svc.OrgMember(ctx, org.ID, "u9")
	if err != nil || !member {
		t.Fatalf("membership check: %v %v", member, err)
	}
	if err := svc.AssignRole("u9", "manager", org.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := svc.RemoveOrgMember(ctx, org.ID, "u9"); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if svc.Enforce("u9", org.ID, "jobs", "read") {
		t.Error("role assignments must be revoked when membership ends")
	}
}

// TestPoliciesSurviveEnforcerRestart reproduces the restart path: a second
// enforcer over the same store must load the persisted rules and enforce
// them (this failed when LoadPolicy omitted the ptype prefix).
func TestPoliciesSurviveEnforcerRestart(t *testing.T) {
	svc, store := newRBACService(t)
	if err := svc.AssignRole("u1", "admin", rbac.PlatformDomain); err != nil {
		t.Fatalf("assign: %v", err)
	}

	enforcer, err := platformrbac.NewEnforcer(store)
	if err != nil {
		t.Fatalf("restarted enforcer: %v", err)
	}
	restarted := rbac.NewService(enforcer, store, store, nil)
	if !restarted.Enforce("u1", rbac.PlatformDomain, "users", "delete") {
		t.Fatal("persisted policies must be enforced after a restart")
	}
	if restarted.Enforce("u1", rbac.PlatformDomain, "policies", "create") {
		t.Fatal("admin must not gain policies:create after restart")
	}
}
