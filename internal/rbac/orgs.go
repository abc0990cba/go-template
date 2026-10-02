// Organization administration: CRUD over the Casbin domains and their
// memberships.

package rbac

import (
	"context"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// OrgsForUser lists the organizations the user belongs to.
func (s *Service) OrgsForUser(ctx context.Context, userID string) ([]Organization, error) {
	if s.orgs == nil {
		return nil, kernel.ErrNotFound
	}
	return s.orgs.ListOrgsForUser(ctx, userID)
}

// OrgMember reports membership of the user in the organization.
func (s *Service) OrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	if s.orgs == nil {
		return false, kernel.ErrNotFound
	}
	if _, err := s.orgs.GetOrg(ctx, orgID); err != nil {
		return false, err
	}
	return s.orgs.IsOrgMember(ctx, orgID, userID)
}

// ListOrgs pages organizations for the admin console.
func (s *Service) ListOrgs(ctx context.Context, opts kernel.ListOptions) ([]Organization, string, error) {
	return s.orgs.ListOrgs(ctx, opts)
}

// CreateOrg adds an organization and, when requested, puts the creator in it.
func (s *Service) CreateOrg(ctx context.Context, name, slug string) (Organization, error) {
	if name == "" {
		return Organization{}, kernel.Invalid("organization name is required")
	}
	if slug == "" {
		slug = slugify(name)
	}
	if !validSlug(slug) {
		return Organization{}, kernel.Invalid("slug must be lowercase letters, digits, and dashes")
	}
	now := time.Now()
	org := Organization{ID: kernel.NewID(), Name: name, Slug: slug, CreatedAt: now, UpdatedAt: now}
	if err := s.orgs.CreateOrg(ctx, org); err != nil {
		return Organization{}, err
	}
	return org, nil
}

// GetOrg returns one organization.
func (s *Service) GetOrg(ctx context.Context, id string) (Organization, error) {
	return s.orgs.GetOrg(ctx, id)
}

// UpdateOrg rewrites the organization.
func (s *Service) UpdateOrg(ctx context.Context, id, name, slug string) error {
	org, err := s.orgs.GetOrg(ctx, id)
	if err != nil {
		return err
	}
	if name != "" {
		org.Name = name
	}
	if slug != "" {
		if !validSlug(slug) {
			return kernel.Invalid("slug must be lowercase letters, digits, and dashes")
		}
		org.Slug = slug
	}
	return s.orgs.UpdateOrg(ctx, org)
}

// DeleteOrg removes the organization.
func (s *Service) DeleteOrg(ctx context.Context, id string) error {
	return s.orgs.DeleteOrg(ctx, id)
}

// OrgMembers lists an organization's members with their user snapshot.
func (s *Service) OrgMembers(ctx context.Context, orgID string) ([]OrgMember, error) {
	return s.orgs.ListOrgMembers(ctx, orgID)
}

// AddOrgMember attaches the user to the organization.
func (s *Service) AddOrgMember(ctx context.Context, orgID, userID string) error {
	return s.orgs.AddOrgMember(ctx, OrgMember{OrgID: orgID, UserID: userID, JoinedAt: time.Now()})
}

// RemoveOrgMember detaches the user from the organization; their role
// assignments in the domain are revoked too.
func (s *Service) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
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
