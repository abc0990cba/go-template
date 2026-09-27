// Package rbac wires the Casbin policy engine onto the RuleStore port.
// The model is RBAC with domains: organizations are domains, the reserved
// domain "*" is the whole platform, and policies support keyMatch2
// wildcards on object and action (e.g. "users/*", "users:*").
package rbac

import (
	"context"
	"fmt"

	casbinmodel "github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// ModelText is the Casbin model: policies p(sub, dom, obj, act), role
// assignments g(user, role, domain). A policy matches when the subject
// holds the policy's role in the request domain (or was assigned it in
// the "*" domain), the domains match (with "*" as the platform wildcard),
// and object/action match with keyMatch2 wildcards.
const ModelText = `
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

// Compile-time proof that the adapter satisfies Casbin's persistence
// contract.
var _ persist.Adapter = (*Adapter)(nil)

// Adapter persists Casbin policies through the RuleStore port, so the
// engine's autosaved mutations land in PostgreSQL (or the memory fake)
// without the service layer knowing about Casbin's storage interface.
type Adapter struct {
	store repository.RuleStore
}

// NewAdapter builds an adapter over the store.
func NewAdapter(store repository.RuleStore) *Adapter {
	return &Adapter{store: store}
}

// LoadPolicy reads every p and g rule into the Casbin model.
func (a *Adapter) LoadPolicy(m casbinmodel.Model) error {
	ctx := context.Background()
	for _, ptype := range []string{"p", "g"} {
		rules, err := a.store.ListPolicyRules(ctx, ptype)
		if err != nil {
			return fmt.Errorf("rbac load %s rules: %w", ptype, err)
		}
		for _, rule := range rules {
			// Policy lines carry the ptype first, exactly like file lines
			// ("p, sub, dom, obj, act"); trailing empties are trimmed so
			// three-value grouping rules are accepted.
			values := append([]string{ptype}, ruleValues(rule)...)
			for len(values) > 1 && values[len(values)-1] == "" {
				values = values[:len(values)-1]
			}
			if err := persist.LoadPolicyArray(values, m); err != nil {
				return fmt.Errorf("rbac load rule %v: %w", values, err)
			}
		}
	}
	return nil
}

// SavePolicy rewrites the whole policy set; used by bulk operations the
// template does not exercise, kept correct for completeness.
func (a *Adapter) SavePolicy(m casbinmodel.Model) error {
	ctx := context.Background()
	if err := a.store.DeletePolicyRulesByType(ctx, "p"); err != nil {
		return err
	}
	if err := a.store.DeletePolicyRulesByType(ctx, "g"); err != nil {
		return err
	}
	sets := []struct {
		ptype   string
		policy  [][]string
	}{
		{"p", m["p"]["p"].Policy},
		{"g", m["g"]["g"].Policy},
	}
	for _, set := range sets {
		for _, rule := range set.policy {
			if err := a.store.InsertPolicyRule(ctx, ruleFromValues(set.ptype, rule)); err != nil {
				return err
			}
		}
	}
	return nil
}

// AddPolicy persists one rule the enforcer just accepted (autosave).
func (a *Adapter) AddPolicy(sec string, ptype string, rule []string) error {
	stored := ruleFromValues(ptype, rule)
	return a.store.InsertPolicyRule(context.Background(), stored)
}

// RemovePolicy deletes the exact rule.
func (a *Adapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return a.store.DeletePolicyRule(context.Background(), ruleFromValues(ptype, rule))
}

// RemoveFilteredPolicy deletes every rule of the ptype matching the given
// field values starting at fieldIndex. Only subject+domain prefix removal
// is exercised (replacing a role's permission set); other filters remove
// matching rules one by one.
func (a *Adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	ctx := context.Background()
	if fieldIndex == 0 && len(fieldValues) == 2 && ptype == "p" {
		return a.store.DeleteFilteredPolicyRules(ctx, ptype, fieldValues[0], fieldValues[1])
	}
	rules, err := a.store.ListPolicyRules(ctx, ptype)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		values := ruleValues(rule)
		match := true
		for i, want := range fieldValues {
			idx := fieldIndex + i
			if idx >= len(values) || values[idx] != want {
				match = false
				break
			}
		}
		if match {
			if err := a.store.DeletePolicyRule(ctx, rule); err != nil {
				return err
			}
		}
	}
	return nil
}

func ruleFromValues(ptype string, rule []string) model.PolicyRule {
	padded := make([]string, 4)
	copy(padded, rule)
	return model.PolicyRule{
		PType: ptype, Subject: padded[0], Domain: padded[1], Object: padded[2], Action: padded[3],
	}
}

func ruleValues(rule model.PolicyRule) []string {
	return []string{rule.Subject, rule.Domain, rule.Object, rule.Action}
}
