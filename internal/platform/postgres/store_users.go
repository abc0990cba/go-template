package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/users"
)

// CreateUser writes the user and, with a hash, its credential and password identity in one transaction.
func (s *Store) CreateUser(ctx context.Context, user users.User, passwordHash string) error {
	params := db.CreateUserParams{ID: user.ID, Email: user.Email, Name: user.Name, ExternalRef: textOrNull(user.ExternalRef), CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
	if passwordHash == "" {
		_, err := s.q.CreateUser(ctx, params)
		return mapErr(err)
	}
	// A user with a password brings its credential and the "password"
	// identity in the same transaction, mirroring the memory fake.
	return s.tx(ctx, func(q *db.Queries) error {
		created, err := q.CreateUser(ctx, params)
		if err != nil {
			return mapErr(err)
		}
		if err := q.UpsertPasswordCredential(ctx, db.UpsertPasswordCredentialParams{UserID: created.ID, PasswordHash: passwordHash}); err != nil {
			return mapErr(err)
		}
		return q.InsertIdentity(ctx, db.InsertIdentityParams{
			Provider:        "password",
			ProviderSubject: strings.ToLower(user.Email),
			UserID:          user.ID,
			Email:           user.Email,
			Name:            user.Name,
		})
	})
}

// UpdateUser rewrites email, name, and external_ref, and refreshes updated_at.
// The enrichment columns are stream-owned and never touched here.
func (s *Store) UpdateUser(ctx context.Context, user users.User) error {
	_, err := s.q.UpdateUser(ctx, db.UpdateUserParams{ID: user.ID, Email: user.Email, Name: user.Name, ExternalRef: textOrNull(user.ExternalRef)})
	return mapErr(err)
}

// DeleteUser removes the user and reports ErrNotFound when absent.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	n, err := s.q.DeleteUser(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}

// GetUser returns the user by id.
func (s *Store) GetUser(ctx context.Context, id string) (users.User, error) {
	row, err := s.q.GetUser(ctx, id)
	if err != nil {
		return users.User{}, mapErr(err)
	}
	return userFromColumns(row.ID, row.Email, row.Name, row.ExternalRef,
		row.EnrichProfile, row.EnrichContact, row.EnrichedAt, row.CreatedAt, row.UpdatedAt), nil
}

// GetUserByEmail matches case-insensitively and returns the password hash.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (users.User, string, error) {
	row, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return users.User{}, "", mapErr(err)
	}
	return userFromRow(db.User{
		ID: row.ID, Email: row.Email, Name: row.Name, ExternalRef: row.ExternalRef,
		EnrichProfile: row.EnrichProfile, EnrichContact: row.EnrichContact, EnrichedAt: row.EnrichedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), row.PasswordHash, nil
}

// ListUsers pages newest first; cursor/keyset continue after After, and the returned cursor is empty on the last page.
func (s *Store) ListUsers(ctx context.Context, opts kernel.ListOptions) ([]users.User, string, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	var (
		page []users.User
		err  error
	)
	if (opts.Mode == "cursor" || opts.Mode == "keyset") && opts.After != "" {
		var rows []db.ListUsersKeysetRow
		rows, err = s.q.ListUsersKeyset(ctx, db.ListUsersKeysetParams{Query: opts.Query, After: opts.After, Lim: limit + 1}) // #nosec G115 -- limit is clamped to 1..100
		if err == nil {
			page = make([]users.User, 0, len(rows))
			for _, row := range rows {
				page = append(page, userFromColumns(row.ID, row.Email, row.Name, row.ExternalRef,
					row.EnrichProfile, row.EnrichContact, row.EnrichedAt, row.CreatedAt, row.UpdatedAt))
			}
		}
	} else {
		off := int32(0)
		if opts.Offset > 0 {
			off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
		}
		var rows []db.ListUsersOffsetRow
		rows, err = s.q.ListUsersOffset(ctx, db.ListUsersOffsetParams{Query: opts.Query, Off: off, Lim: limit + 1})
		if err == nil {
			page = make([]users.User, 0, len(rows))
			for _, row := range rows {
				page = append(page, userFromColumns(row.ID, row.Email, row.Name, row.ExternalRef,
					row.EnrichProfile, row.EnrichContact, row.EnrichedAt, row.CreatedAt, row.UpdatedAt))
			}
		}
	}
	if err != nil {
		return nil, "", mapErr(err)
	}
	next := ""
	if len(page) > int(limit) {
		next = page[limit-1].ID
		page = page[:limit]
	}
	return page, next, nil
}

// CountUsers returns the number of users, 0 when the query fails.
func (s *Store) CountUsers(ctx context.Context) int {
	n, err := s.q.CountUsers(ctx)
	if err != nil {
		return 0
	}
	return int(n)
}

// LinkIdentity is idempotent for the same owner and reports ErrConflict when another user owns the identity.
func (s *Store) LinkIdentity(ctx context.Context, identity auth.Identity) error {
	owner, err := s.q.GetIdentityOwner(ctx, db.GetIdentityOwnerParams{Provider: identity.Provider, ProviderSubject: identity.ProviderSubject})
	switch {
	case err == nil:
		if owner == identity.UserID {
			return nil
		}
		return fmt.Errorf("%w: identity owned by another user", kernel.ErrConflict)
	case !errors.Is(err, pgx.ErrNoRows):
		return mapErr(err)
	}
	return mapErr(s.q.InsertIdentity(ctx, db.InsertIdentityParams{
		Provider:        identity.Provider,
		ProviderSubject: identity.ProviderSubject,
		UserID:          identity.UserID,
		Email:           identity.Email,
		Name:            identity.Name,
		AvatarUrl:       identity.AvatarURL,
	}))
}

// GetIdentity returns the identity for a (provider, subject) pair.
func (s *Store) GetIdentity(ctx context.Context, provider, subject string) (auth.Identity, error) {
	row, err := s.q.GetIdentity(ctx, db.GetIdentityParams{Provider: provider, ProviderSubject: subject})
	if err != nil {
		return auth.Identity{}, mapErr(err)
	}
	return auth.Identity{
		UserID: row.UserID, Provider: row.Provider, ProviderSubject: row.ProviderSubject,
		Email: row.Email, Name: row.Name, AvatarURL: row.AvatarUrl,
		LinkedAt: row.LinkedAt, LastLoginAt: row.LastLoginAt,
	}, nil
}

// UnlinkIdentity detaches the provider identity from the user.
func (s *Store) UnlinkIdentity(ctx context.Context, userID, provider string) error {
	return mapErr(s.q.UnlinkIdentity(ctx, db.UnlinkIdentityParams{UserID: userID, Provider: provider}))
}

// ListIdentities lists the user's identities, oldest link first.
func (s *Store) ListIdentities(ctx context.Context, userID string) ([]auth.Identity, error) {
	rows, err := s.q.ListIdentities(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	identities := make([]auth.Identity, 0, len(rows))
	for _, row := range rows {
		identities = append(identities, auth.Identity{
			UserID: row.UserID, Provider: row.Provider, ProviderSubject: row.ProviderSubject,
			Email: row.Email, Name: row.Name, AvatarURL: row.AvatarUrl,
			LinkedAt: row.LinkedAt, LastLoginAt: row.LastLoginAt,
		})
	}
	return identities, nil
}

// CreateSession stores a token with the given TTL. impersonatedBy is the
// admin behind an impersonation session ("" for normal logins).
func (s *Store) CreateSession(ctx context.Context, token, userID, impersonatedBy string, ttl time.Duration) error {
	return mapErr(s.q.CreateSession(ctx, db.CreateSessionParams{Token: token, UserID: userID, ImpersonatedBy: impersonatedBy, Secs: ttl.Seconds()}))
}

// GetSession resolves only unexpired tokens and slides the expiry forward
// in the same statement. It reports the impersonator, if any.
func (s *Store) GetSession(ctx context.Context, token string, ttl time.Duration) (auth.Session, error) {
	row, err := s.q.GetSession(ctx, db.GetSessionParams{Token: token, Secs: ttl.Seconds()})
	return auth.Session{UserID: row.UserID, ImpersonatedBy: row.ImpersonatedBy}, mapErr(err)
}

// DeleteSession revokes a token.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return mapErr(s.q.DeleteSession(ctx, token))
}

// DeleteSessionsForUser revokes every session of the user (logout-all).
func (s *Store) DeleteSessionsForUser(ctx context.Context, userID string) error {
	return mapErr(s.q.DeleteSessionsForUser(ctx, userID))
}

// SaveOAuthState stores a single-use state with its expiry.
func (s *Store) SaveOAuthState(ctx context.Context, state auth.OAuthState) error {
	return mapErr(s.q.SaveOAuthState(ctx, db.SaveOAuthStateParams{
		State: state.State, Provider: state.Provider, UserID: state.UserID,
		RedirectAfter: state.RedirectAfter, CodeVerifier: state.CodeVerifier,
		ExpiresAt: state.ExpiresAt,
	}))
}

// ConsumeOAuthState deletes the state first, then honors expiry: consumed or expired states always yield ErrNotFound.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string, now time.Time) (auth.OAuthState, error) {
	row, err := s.q.ConsumeOAuthState(ctx, state)
	if err != nil {
		return auth.OAuthState{}, mapErr(err)
	}
	if now.After(row.ExpiresAt) {
		return auth.OAuthState{}, kernel.ErrNotFound
	}
	return auth.OAuthState{
		State: row.State, Provider: row.Provider, UserID: row.UserID,
		RedirectAfter: row.RedirectAfter, CodeVerifier: row.CodeVerifier,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

// ListSessions returns the user's unexpired sessions newest first, keyed by
// a short prefix of the opaque token.
func (s *Store) ListSessions(ctx context.Context, userID string) ([]auth.SessionInfo, error) {
	rows, err := s.q.ListSessions(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	sessions := make([]auth.SessionInfo, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, auth.SessionInfo{ID: row.ID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt})
	}
	return sessions, nil
}
