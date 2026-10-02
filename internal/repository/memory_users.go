package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

// CreateUser stores a user and, when passwordHash is non-empty, the password
// credential plus the "password" identity. Emails are unique
// case-insensitively.
func (s *MemoryStore) CreateUser(_ context.Context, user users.User, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.users {
		if strings.EqualFold(existing.Email, user.Email) {
			return kernel.ErrConflict
		}
		if user.ExternalRef != "" && existing.ExternalRef == user.ExternalRef {
			return kernel.ErrConflict
		}
	}
	s.users[user.ID] = user
	if passwordHash != "" {
		s.passwords[user.Email] = passwordHash
		s.identities = append(s.identities, auth.Identity{
			UserID: user.ID, Provider: "password", ProviderSubject: strings.ToLower(user.Email), Email: user.Email, Name: user.Name, LinkedAt: time.Now(), LastLoginAt: time.Now(),
		})
	}
	return nil
}

// UpdateUser overwrites an existing user and refreshes UpdatedAt. The
// enrichment columns are stream-owned; a conflicting external_ref on another
// user is rejected, mirroring the partial unique index.
func (s *MemoryStore) UpdateUser(_ context.Context, user users.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return kernel.ErrNotFound
	}
	if user.ExternalRef != "" {
		for id, existing := range s.users {
			if id != user.ID && existing.ExternalRef == user.ExternalRef {
				return kernel.ErrConflict
			}
		}
	}
	user.UpdatedAt = time.Now()
	s.users[user.ID] = user
	return nil
}

// DeleteUser removes the user by id and cascades org memberships.
func (s *MemoryStore) DeleteUser(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return kernel.ErrNotFound
	}
	delete(s.users, id)
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if member.UserID != id {
			members = append(members, member)
		}
	}
	s.orgMembers = members
	return nil
}

// GetUser returns the user by id.
func (s *MemoryStore) GetUser(_ context.Context, id string) (users.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return users.User{}, kernel.ErrNotFound
	}
	return user, nil
}

// GetUserByEmail looks a user up case-insensitively and returns the stored
// password hash alongside.
func (s *MemoryStore) GetUserByEmail(_ context.Context, email string) (users.User, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if strings.EqualFold(user.Email, email) {
			return user, s.passwords[user.Email], nil
		}
	}
	return users.User{}, "", kernel.ErrNotFound
}

// ListUsers returns a page of users ordered newest first (id ascending as a
// tiebreaker) plus the cursor for the next page, empty at the end of the
// list. Limit is clamped to the 1..100 range.
func (s *MemoryStore) ListUsers(_ context.Context, opts kernel.ListOptions) ([]users.User, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page := make([]users.User, 0, len(s.users))
	for _, user := range s.users {
		if opts.Query == "" || strings.Contains(strings.ToLower(user.Email+" "+user.Name), strings.ToLower(opts.Query)) {
			page = append(page, user)
		}
	}
	sort.Slice(page, func(i, j int) bool {
		if page[i].CreatedAt.Equal(page[j].CreatedAt) {
			return page[i].ID < page[j].ID
		}
		return page[i].CreatedAt.After(page[j].CreatedAt)
	})
	if opts.Limit <= 0 || opts.Limit > 100 {
		opts.Limit = 25
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		start = 0
		if opts.After != "" {
			for i, user := range page {
				if user.ID == opts.After {
					start = i + 1
					break
				}
			}
		}
	}
	if start > len(page) {
		return nil, "", nil
	}
	end := start + opts.Limit
	if end > len(page) {
		end = len(page)
	}
	next := ""
	if end < len(page) {
		next = page[end-1].ID
	}
	return page[start:end], next, nil
}

// CountUsers returns the total number of users.
func (s *MemoryStore) CountUsers(_ context.Context) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

// ResetDemoData drops demo-seeded entities (bootstrap reset extension).
func (s *MemoryStore) ResetDemoData(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.users {
		if strings.HasPrefix(id, "demo-") {
			delete(s.users, id)
		}
	}
	for id := range s.jobs {
		if strings.HasPrefix(id, "demo-") {
			delete(s.jobs, id)
		}
	}
	for id := range s.reports {
		if strings.HasPrefix(id, "demo-") {
			delete(s.reports, id)
		}
	}
	for id := range s.orgs {
		if strings.HasPrefix(id, "demo-") {
			delete(s.orgs, id)
		}
	}
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if strings.HasPrefix(member.OrgID, "demo-") || strings.HasPrefix(member.UserID, "demo-") {
			continue
		}
		members = append(members, member)
	}
	s.orgMembers = members
	s.enrichments = nil
	s.enrichSeq = 0
	s.emailTokens = nil
	endpoints := s.webhooks[:0]
	for _, endpoint := range s.webhooks {
		if !strings.HasPrefix(endpoint.ID, "demo-") {
			endpoints = append(endpoints, endpoint)
		}
	}
	s.webhooks = endpoints
	pending := s.deliveries[:0]
	for _, delivery := range s.deliveries {
		if strings.HasPrefix(delivery.ID, "demo-") || strings.HasPrefix(delivery.EndpointID, "demo-") {
			continue
		}
		pending = append(pending, delivery)
	}
	s.deliveries = pending
	// Demo tickets plus anything referencing the seeded ticket types — the
	// foreign keys enforce this on PostgreSQL, the fake mirrors it here.
	for id, ticket := range s.tickets {
		if strings.HasPrefix(id, "demo-") || strings.HasPrefix(ticket.TypeID, "demo-") {
			delete(s.tickets, id)
			delete(s.approvals, id)
		}
	}
	for id := range s.ticketTypes {
		if strings.HasPrefix(id, "demo-") {
			delete(s.ticketTypes, id)
		}
	}
	comments := s.comments[:0]
	for _, comment := range s.comments {
		if strings.HasPrefix(comment.TicketID, "demo-") || strings.HasPrefix(comment.AuthorID, "demo-") {
			continue
		}
		comments = append(comments, comment)
	}
	s.comments = comments
	events := s.ticketEvents[:0]
	for _, event := range s.ticketEvents {
		if strings.HasPrefix(event.TicketID, "demo-") || strings.HasPrefix(event.ActorID, "demo-") {
			continue
		}
		events = append(events, event)
	}
	s.ticketEvents = events
	for key, applied := range s.processedEvents {
		if strings.HasPrefix(applied.UserID, "demo-") {
			delete(s.processedEvents, key)
			delete(s.processedAt, key)
		}
	}
	for id := range s.enrichSeqs {
		if strings.HasPrefix(id, "demo-") {
			delete(s.enrichSeqs, id)
		}
	}
	outbox := s.outboxItems[:0]
	for _, item := range s.outboxItems {
		if !strings.HasPrefix(item.AggregateID, "demo-") {
			outbox = append(outbox, item)
		}
	}
	s.outboxItems = outbox
	return nil
}
