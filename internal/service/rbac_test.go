package service

import (
	"context"
	"testing"
	"time"

	casbinmodel "github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/persist"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// newEnforcerForTest mirrors platform/rbac.NewEnforcer without importing it
// (which would cycle back into this package).
func newEnforcerForTest(t *testing.T, store repository.RuleStore) *casbin.SyncedEnforcer {
	t.Helper()
	m, err := casbinmodel.NewModelFromString(rbacModelTextForTest)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	e, err := casbin.NewSyncedEnforcer(m, testAdapter{store: store})
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}
	if err := e.LoadPolicy(); err != nil {
		t.Fatalf("load policy: %v", err)
	}
	return e
}

// rbacModelTextForTest duplicates the production model; it is asserted
// against the real one in TestModelTextMatchesProduction.
const rbacModelTextForTest = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = (g(r.sub, p.sub, r.dom) || g(r.sub, p.sub, "*")) && (r.dom == p.dom || p.dom == "*") && keyMatch2(r.obj, p.obj) && (keyMatch2(r.act, p.act) || p.act == "*")
`

// newRBACService builds an RBACService over a memory-backed policy store
// with the system roles seeded.
func newRBACService(t *testing.T) (*RBACService, *repository.MemoryStore) {
	t.Helper()
	store := repository.NewMemoryStore()
	enforcer := newEnforcerForTest(t, store)
	svc := &RBACService{enforcer: enforcer, roles: store, orgs: store}
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
		if err := svc.AssignRole(sub, tc.role, PlatformDomain); err != nil {
			t.Fatalf("assign %s: %v", tc.role, err)
		}
		if got := svc.Enforce(sub, PlatformDomain, tc.obj, tc.act); got != tc.want {
			t.Errorf("case %d role %s %s:%s = %v, want %v", i, tc.role, tc.obj, tc.act, got, tc.want)
		}
	}
}

func TestDomainScopedRolesAndPlatformWildcard(t *testing.T) {
	svc, _ := newRBACService(t)
	if _, err := svc.CreateRole(t.Context(), "auditor", "Auditor", "reads audit"); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := svc.SetRolePermissions("auditor", "org-1", []Permission{{Object: "audit", Action: "read"}}); err != nil {
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
	if err := svc.AssignRole("u3", "manager", PlatformDomain); err != nil {
		t.Fatalf("assign platform role: %v", err)
	}
	if !svc.Enforce("u3", "org-9", "jobs", "read") {
		t.Error("platform role should apply in any domain")
	}
}

func TestEffectivePermissionsExpandWildcards(t *testing.T) {
	svc, _ := newRBACService(t)
	if err := svc.AssignRole("u1", "superadmin", PlatformDomain); err != nil {
		t.Fatalf("assign: %v", err)
	}
	perms := svc.EffectivePermissions("u1", PlatformDomain)
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
	if got := svc.EffectivePermissions("nobody", PlatformDomain); len(got) != 0 {
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
	if err := svc.SetRolePermissions("temp", PlatformDomain, []Permission{{Object: "jobs", Action: "read"}}); err != nil {
		t.Fatalf("set permissions: %v", err)
	}
	if err := svc.AssignRole("u4", "temp", PlatformDomain); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !svc.Enforce("u4", PlatformDomain, "jobs", "read") {
		t.Fatal("temp role should grant jobs:read")
	}
	if err := svc.DeleteRole(t.Context(), "temp"); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	if svc.Enforce("u4", PlatformDomain, "jobs", "read") {
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
	if err := store.CreateUser(ctx, model.User{ID: "u9", Email: "u9@example.com", CreatedAt: now, UpdatedAt: now}, ""); err != nil {
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


// testAdapter is a minimal Casbin persist.Adapter over the memory store,
// mirroring the production adapter in internal/platform/rbac without
// importing it (which would cycle back into this package).
type testAdapter struct {
	store repository.RuleStore
}

func (a testAdapter) LoadPolicy(m casbinmodel.Model) error {
	for _, ptype := range []string{"p", "g"} {
		rules, err := a.store.ListPolicyRules(context.Background(), ptype)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			values := []string{rule.Subject, rule.Domain, rule.Object, rule.Action}
			for len(values) > 0 && values[len(values)-1] == "" {
				values = values[:len(values)-1]
			}
			if err := persist.LoadPolicyArray(values, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a testAdapter) SavePolicy(m casbinmodel.Model) error { return nil }

func (a testAdapter) AddPolicy(_ string, ptype string, rule []string) error {
	return a.store.InsertPolicyRule(context.Background(), policyRuleOf(ptype, rule))
}

func (a testAdapter) RemovePolicy(_ string, ptype string, rule []string) error {
	return a.store.DeletePolicyRule(context.Background(), policyRuleOf(ptype, rule))
}

func (a testAdapter) RemoveFilteredPolicy(_ string, _ string, _ int, _ ...string) error { return nil }

func policyRuleOf(ptype string, rule []string) model.PolicyRule {
	padded := make([]string, 4)
	copy(padded, rule)
	return model.PolicyRule{PType: ptype, Subject: padded[0], Domain: padded[1], Object: padded[2], Action: padded[3]}
}
