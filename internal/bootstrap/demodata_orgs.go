package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/rbac"
)

// seedDemoRBAC creates three organizations, their memberships, and the role
// assignments that make the RBAC admin immediately explorable: the first
// demo user is a platform admin, org leads hold manager, the rest viewer.
func seedDemoRBAC(ctx context.Context, services *Container, repos Repos, rng *rand.Rand, now time.Time) error {
	orgNames := []struct{ name, slug string }{
		{"Acme Corp", "acme"}, {"Globex", "globex"}, {"Initech", "initech"},
	}
	for n, named := range orgNames[:demoOrgN] {
		org := rbac.Organization{
			ID:        fmt.Sprintf("%sorg-%d", demoIDPrefix, n+1),
			Name:      named.name,
			Slug:      named.slug,
			CreatedAt: now.Add(-time.Duration(rng.Int63n(int64(demoUserSpan)))),
		}
		if err := repos.Orgs.CreateOrg(ctx, org); err != nil && !errors.Is(err, kernel.ErrConflict) {
			return err
		}
		// Members: the org lead plus a slice of the population.
		lead := demoIDPrefix + "u-" + pad4(10*(n+1))
		if err := repos.Orgs.AddOrgMember(ctx, rbac.OrgMember{OrgID: org.ID, UserID: lead, JoinedAt: now.Add(-720 * time.Hour)}); err != nil {
			return err
		}
		if err := services.RBAC.AssignRole(lead, "manager", org.ID); err != nil {
			return err
		}
		for m := 0; m < 6; m++ {
			member := demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount))
			if err := repos.Orgs.AddOrgMember(ctx, rbac.OrgMember{OrgID: org.ID, UserID: member, JoinedAt: now.Add(-time.Duration(rng.Int63n(int64(400 * time.Hour))))}); err != nil {
				continue // duplicates are fine
			}
			// Members file tickets; every second one can also approve, so
			// the approvals inbox is populated and some users see both
			// interfaces.
			_ = services.RBAC.AssignRole(member, "requester", org.ID)
			if m%2 == 0 {
				_ = services.RBAC.AssignRole(member, "approver", org.ID)
			}
		}
	}
	// Platform-level grants for the loginable head of the dataset. The six
	// named accounts cover every interface: admin, manager, viewer, a pure
	// requester, a pure approver, and a requester+approver hybrid — the
	// quickstart table and the login page walk through them.
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0001", "admin", rbac.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0002", "manager", rbac.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0003", "viewer", rbac.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0004", "requester", rbac.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0005", "approver", rbac.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0006", "requester", rbac.PlatformDomain); err != nil {
		return err
	}
	if err := services.RBAC.AssignRole(demoIDPrefix+"u-0006", "approver", rbac.PlatformDomain); err != nil {
		return err
	}
	return nil
}

// demoOrgContext carries one seeded org's cast: who files tickets, who
// approves them, who manages them.
type demoOrgContext struct {
	id        string
	members   []string
	approvers []string
	managers  []string
}

// decideAs picks a plausible decider for the role inside the org, falling
// back to any member when the cast is thin.
func (c demoOrgContext) decideAs(rng *rand.Rand, role string) string {
	switch role {
	case "manager":
		if len(c.managers) > 0 {
			return pick(rng, c.managers)
		}
	case "approver":
		if len(c.approvers) > 0 {
			return pick(rng, c.approvers)
		}
	}
	return pick(rng, c.members)
}

// demoOrgContexts reloads the seeded organizations with their members and
// role holders, so tickets land in orgs their members can actually see.
func demoOrgContexts(ctx context.Context, services *Container) ([]demoOrgContext, error) {
	orgs, _, err := services.RBAC.ListOrgs(ctx, kernel.ListOptions{Mode: "offset", Limit: 100})
	if err != nil {
		return nil, err
	}
	var out []demoOrgContext
	for _, org := range orgs {
		if !strings.HasPrefix(org.ID, demoIDPrefix) {
			continue
		}
		members, err := services.RBAC.OrgMembers(ctx, org.ID)
		if err != nil {
			return nil, err
		}
		c := demoOrgContext{id: org.ID}
		for _, m := range members {
			c.members = append(c.members, m.UserID)
			for _, role := range services.RBAC.RolesForUser(m.UserID, org.ID) {
				switch role {
				case "approver":
					c.approvers = append(c.approvers, m.UserID)
				case "manager":
					c.managers = append(c.managers, m.UserID)
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}
