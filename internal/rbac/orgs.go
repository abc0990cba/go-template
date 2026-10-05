// Organization administration: CRUD over the Casbin domains and their
// memberships.

package rbac

import (
	"context"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/cache"
	"github.com/your-org/go-template/internal/kernel"
)

// OrgsForUser lists the organizations the user belongs to. Memberships
// change rarely and the list is read on every permission-guarded request
// (and per realtime connection), so it rides the 30s cache; every
// membership mutation invalidates the user's key.
func (s *Service) OrgsForUser(ctx context.Context, userID string) ([]Organization, error) {
	if s.orgs == nil {
		return nil, kernel.ErrNotFound
	}
	if s.permsCache.cache != nil {
		return cache.CachedGetOrLoad(ctx, s.permsCache.cache, s.logger, &s.orgsGroup,
			cache.OrgsCacheKey(userID), cache.CacheTTLUser, func(ctx context.Context) ([]Organization, error) {
				return s.orgs.ListOrgsForUser(ctx, userID)
			})
	}
	return s.orgs.ListOrgsForUser(ctx, userID)
}

// invalidateOrgs drops the user's cached membership list.
func (s *Service) invalidateOrgs(userID string) {
	if s.permsCache.cache != nil {
		s.permsCache.cache.Delete(context.Background(), cache.OrgsCacheKey(userID))
	}
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

// DeleteOrg removes the organization and drops every cached membership
// list (all members' lists change at once).
func (s *Service) DeleteOrg(ctx context.Context, id string) error {
	if err := s.orgs.DeleteOrg(ctx, id); err != nil {
		return err
	}
	if s.permsCache.cache != nil {
		s.permsCache.cache.DeletePrefix(ctx, cache.CacheKeyOrgs)
	}
	return nil
}

// OrgMembers lists an organization's members with their user snapshot.
func (s *Service) OrgMembers(ctx context.Context, orgID string) ([]OrgMember, error) {
	return s.orgs.ListOrgMembers(ctx, orgID)
}

// AddOrgMember attaches the user to the organization.
func (s *Service) AddOrgMember(ctx context.Context, orgID, userID string) error {
	if err := s.orgs.AddOrgMember(ctx, OrgMember{OrgID: orgID, UserID: userID, JoinedAt: time.Now()}); err != nil {
		return err
	}
	s.invalidateOrgs(userID)
	return nil
}

// RemoveOrgMember detaches the user from the organization; their role
// assignments in the domain are revoked too.
func (s *Service) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	s.invalidateOrgs(userID)
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
