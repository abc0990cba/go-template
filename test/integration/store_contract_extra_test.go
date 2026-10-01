//go:build integration

// The second half of the store contract: the feature stores the original
// suite did not cover. RunStoreContractExtra is called from runStoreContract,
// so every scenario here runs against both the memory fake and PostgreSQL.
package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/tokens"
	"github.com/your-org/go-template/internal/users"
	"github.com/your-org/go-template/internal/webhooks"
)

// RunStoreContractExtra exercises the remaining store surfaces both
// adapters must share.
func RunStoreContractExtra(t *testing.T, store storeContract) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("email tokens", func(t *testing.T) {
		token := email.Token{ID: "et1", UserID: "u1", Kind: email.KindReset, ExpiresAt: now.Add(time.Hour), CreatedAt: now}
		if err := store.CreateEmailToken(ctx, token, "hash-et1"); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := store.GetEmailTokenByHash(ctx, "hash-et1")
		if err != nil || got.ID != "et1" || got.Kind != email.KindReset {
			t.Fatalf("get by hash: %+v err=%v", got, err)
		}
		if err := store.MarkEmailTokenUsed(ctx, "et1"); err != nil {
			t.Fatalf("mark used: %v", err)
		}
		got, err = store.GetEmailTokenByHash(ctx, "hash-et1")
		if err != nil || got.UsedAt == nil {
			t.Errorf("used token: want UsedAt set, got %+v err=%v", got, err)
		}
		if _, err := store.GetEmailTokenByHash(ctx, "no-such-hash"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("unknown hash: want ErrNotFound, got %v", err)
		}
	})

	t.Run("mfa factors recovery and challenges", func(t *testing.T) {
		if err := store.UpsertMFAFactor(ctx, mfa.Factor{UserID: "u1", Secret: "s3cret", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("upsert factor: %v", err)
		}
		if err := store.ConfirmMFAFactor(ctx, "u1"); err != nil {
			t.Fatalf("confirm: %v", err)
		}
		factor, err := store.GetMFAFactor(ctx, "u1")
		if err != nil || !factor.Confirmed || factor.ConfirmedAt == nil {
			t.Errorf("confirmed factor: %+v err=%v", factor, err)
		}

		// Recovery codes are single-use, hashed at rest.
		for _, id := range []string{"rc1", "rc2"} {
			if err := store.CreateMFARecoveryCode(ctx, mfa.RecoveryCode{ID: id, UserID: "u1", CodeHash: "hash-" + id, CreatedAt: now}); err != nil {
				t.Fatalf("create recovery %s: %v", id, err)
			}
		}
		codes, err := store.ListMFARecoveryCodes(ctx, "u1")
		if err != nil || len(codes) != 2 {
			t.Fatalf("list recovery: %d err=%v", len(codes), err)
		}
		if err := store.ConsumeMFARecoveryCode(ctx, "u1", "hash-rc1"); err != nil {
			t.Fatalf("consume: %v", err)
		}
		if err := store.ConsumeMFARecoveryCode(ctx, "u1", "hash-rc1"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("recovery codes are single use: want ErrNotFound, got %v", err)
		}
		if err := store.DeleteMFARecoveryCodes(ctx, "u1"); err != nil {
			t.Fatalf("delete recovery codes: %v", err)
		}
		if codes, _ = store.ListMFARecoveryCodes(ctx, "u1"); len(codes) != 0 {
			t.Errorf("after delete: want 0, got %d", len(codes))
		}

		// Challenges resolve by token hash, count attempts, and vanish.
		challenge := mfa.Challenge{ID: "ch1", UserID: "u1", TokenHash: "hash-ch1", ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now}
		if err := store.CreateMFAChallenge(ctx, challenge); err != nil {
			t.Fatalf("create challenge: %v", err)
		}
		if got, err := store.GetMFAChallengeByHash(ctx, "hash-ch1"); err != nil || got.ID != "ch1" {
			t.Errorf("get challenge: %+v err=%v", got, err)
		}
		if err := store.IncrementMFAChallengeAttempts(ctx, "ch1"); err != nil {
			t.Fatalf("increment: %v", err)
		}
		if got, _ := store.GetMFAChallengeByHash(ctx, "hash-ch1"); got.Attempts != 1 {
			t.Errorf("attempts: want 1, got %d", got.Attempts)
		}
		if err := store.DeleteMFAChallenge(ctx, "ch1"); err != nil {
			t.Fatalf("delete challenge: %v", err)
		}

		// Expiry sweeps only remove what is past its time.
		if err := store.CreateMFAChallenge(ctx, mfa.Challenge{ID: "ch-old", UserID: "u1", TokenHash: "hash-ch-old", ExpiresAt: now.Add(-time.Minute), CreatedAt: now}); err != nil {
			t.Fatalf("create expired challenge: %v", err)
		}
		if err := store.CreateMFAChallenge(ctx, mfa.Challenge{ID: "ch-live", UserID: "u1", TokenHash: "hash-ch-live", ExpiresAt: now.Add(time.Minute), CreatedAt: now}); err != nil {
			t.Fatalf("create live challenge: %v", err)
		}
		if err := store.DeleteExpiredMFAChallenges(ctx); err != nil {
			t.Fatalf("delete expired: %v", err)
		}
		if _, err := store.GetMFAChallengeByHash(ctx, "hash-ch-live"); err != nil {
			t.Errorf("live challenge swept: %v", err)
		}
		if _, err := store.GetMFAChallengeByHash(ctx, "hash-ch-old"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("expired challenge: want ErrNotFound, got %v", err)
		}
		if err := store.DeleteMFAFactor(ctx, "u1"); err != nil {
			t.Fatalf("delete factor: %v", err)
		}
		if _, err := store.GetMFAFactor(ctx, "u1"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("deleted factor: want ErrNotFound, got %v", err)
		}
	})

	t.Run("refresh token families", func(t *testing.T) {
		refresh := func(id, family, hash string, expires time.Time) tokens.RefreshToken {
			return tokens.RefreshToken{ID: id, UserID: "u1", FamilyID: family, TokenHash: hash, ExpiresAt: expires, CreatedAt: now}
		}
		live := now.Add(time.Hour)
		for _, tok := range []tokens.RefreshToken{
			refresh("rt1", "fam1", "hash-rt1", live),
			refresh("rt2", "fam1", "hash-rt2", live),
			refresh("rt3", "fam2", "hash-rt3", live),
			refresh("rt-grace", "fam3", "hash-rt-grace", now.Add(-time.Minute)),
			refresh("rt-old", "fam4", "hash-rt-old", now.Add(-8*24*time.Hour)),
		} {
			if err := store.CreateRefreshToken(ctx, tok); err != nil {
				t.Fatalf("create %s: %v", tok.ID, err)
			}
		}
		got, err := store.GetRefreshTokenByHash(ctx, "hash-rt1")
		if err != nil || got.ID != "rt1" || got.FamilyID != "fam1" {
			t.Fatalf("get by hash: %+v err=%v", got, err)
		}

		// Single-token and whole-family revocation.
		if err := store.RevokeRefreshToken(ctx, "rt1"); err != nil {
			t.Fatalf("revoke token: %v", err)
		}
		if got, _ = store.GetRefreshTokenByHash(ctx, "hash-rt1"); got.RevokedAt == nil {
			t.Error("revoked token: want RevokedAt set")
		}
		if err := store.RevokeRefreshFamily(ctx, "fam1"); err != nil {
			t.Fatalf("revoke family: %v", err)
		}
		if got, _ = store.GetRefreshTokenByHash(ctx, "hash-rt2"); got.RevokedAt == nil {
			t.Error("family revoke must cover rt2")
		}
		if got, _ = store.GetRefreshTokenByHash(ctx, "hash-rt3"); got.RevokedAt != nil {
			t.Error("family revoke must not cover fam2")
		}

		// Expiry sweeps remove only tokens past the 7-day grace window —
		// recently expired ones must stay resolvable for reuse detection —
		// and logout-everywhere revokes the rest.
		if err := store.DeleteExpiredRefreshTokens(ctx); err != nil {
			t.Fatalf("delete expired: %v", err)
		}
		if got, _ := store.GetRefreshTokenByHash(ctx, "hash-rt-grace"); got.ID != "rt-grace" {
			t.Error("recently expired token must survive the sweep (reuse detection)")
		}
		if _, err := store.GetRefreshTokenByHash(ctx, "hash-rt-old"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("long-expired refresh token: want ErrNotFound, got %v", err)
		}
		if err := store.RevokeRefreshTokensForUser(ctx, "u1"); err != nil {
			t.Fatalf("revoke for user: %v", err)
		}
		if got, _ = store.GetRefreshTokenByHash(ctx, "hash-rt3"); got.RevokedAt == nil {
			t.Error("revoke-for-user must cover fam2")
		}
	})

	t.Run("org invitations lifecycle", func(t *testing.T) {
		// The invitation's org is a real foreign key: create it first.
		if err := store.CreateOrg(ctx, rbac.Organization{ID: "org-inv", Name: "Invite Org", Slug: "invite-org", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("create org: %v", err)
		}
		pending := invitations.Invitation{ID: "inv1", OrgID: "org-inv", Email: "new@example.com", Role: "member",
			TokenHash: "hash-inv1", Status: invitations.StatusPending, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}
		if err := store.CreateOrgInvitation(ctx, pending); err != nil {
			t.Fatalf("create: %v", err)
		}
		if got, err := store.GetOrgInvitationByHash(ctx, "hash-inv1"); err != nil || got.ID != "inv1" {
			t.Fatalf("get by hash: %+v err=%v", got, err)
		}
		if _, err := store.GetOrgInvitation(ctx, "inv1"); err != nil {
			t.Fatalf("get by id: %v", err)
		}
		if list, total, err := store.ListOrgInvitations(ctx, "org-inv", 10, 0); err != nil || len(list) != 1 || total != 1 {
			t.Fatalf("list: %d/%d err=%v", len(list), total, err)
		}
		if pend, err := store.ListPendingOrgInvitationsByEmail(ctx, "new@example.com"); err != nil || len(pend) != 1 {
			t.Fatalf("pending by email: %d err=%v", len(pend), err)
		}

		acceptedAt := now.Add(time.Hour)
		if err := store.SetOrgInvitationStatus(ctx, "inv1", invitations.StatusAccepted, &acceptedAt, "u9"); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if got, _ := store.GetOrgInvitation(ctx, "inv1"); got.Status != invitations.StatusAccepted || got.AcceptedBy != "u9" || got.AcceptedAt == nil {
			t.Errorf("accepted invitation: %+v", got)
		}
		if pend, _ := store.ListPendingOrgInvitationsByEmail(ctx, "new@example.com"); len(pend) != 0 {
			t.Errorf("accepted invitation still pending: %d", len(pend))
		}

		// Old pending invitations expire in bulk.
		old := pending
		old.ID, old.TokenHash = "inv2", "hash-inv2"
		old.ExpiresAt = now.Add(-time.Hour)
		if err := store.CreateOrgInvitation(ctx, old); err != nil {
			t.Fatalf("create stale: %v", err)
		}
		if err := store.ExpireOldOrgInvitations(ctx); err != nil {
			t.Fatalf("expire old: %v", err)
		}
		if got, _ := store.GetOrgInvitation(ctx, "inv2"); got.Status != invitations.StatusExpired {
			t.Errorf("stale invitation: want expired, got %s", got.Status)
		}
	})

	t.Run("file metadata and soft delete", func(t *testing.T) {
		insert := func(id, refType, refID string, createdAt time.Time) files.File {
			return files.File{ID: id, OwnerID: "u1", Key: "key-" + id, Filename: id + ".txt",
				ContentType: "text/plain", SizeBytes: 12, RefType: refType, RefID: refID, CreatedAt: createdAt}
		}
		if err := store.InsertFile(ctx, insert("f1", "ticket", "t1", now)); err != nil {
			t.Fatalf("insert f1: %v", err)
		}
		if err := store.InsertFile(ctx, insert("f2", "ticket", "t1", now.Add(time.Second))); err != nil {
			t.Fatalf("insert f2: %v", err)
		}
		if err := store.InsertFile(ctx, insert("f3", "export", "u1", now)); err != nil {
			t.Fatalf("insert f3: %v", err)
		}
		if err := store.InsertFile(ctx, insert("f4", "export", "u1", now)); err != nil {
			t.Fatalf("insert f4: %v", err)
		}

		if got, err := store.GetFile(ctx, "f1"); err != nil || got.Filename != "f1.txt" {
			t.Fatalf("get: %+v err=%v", got, err)
		}
		if byRef, err := store.ListFilesByRef(ctx, "ticket", "t1"); err != nil || len(byRef) != 2 {
			t.Errorf("by ref: %d err=%v", len(byRef), err)
		}
		if byOwner, err := store.ListFilesByOwner(ctx, "u1", 10, 0); err != nil || len(byOwner) != 4 {
			t.Errorf("by owner: %d err=%v", len(byOwner), err)
		}
		// created_at is the store's stamp, so the age cutoff sits in the
		// future; the sweep must pick exactly the live export files.
		if swept, err := store.ListExportFilesOlderThan(ctx, now.Add(time.Hour)); err != nil || len(swept) != 2 {
			t.Errorf("export sweep: %+v err=%v", swept, err)
		}

		if err := store.SoftDeleteFile(ctx, "f4"); err != nil {
			t.Fatalf("soft delete f4: %v", err)
		}
		if swept, _ := store.ListExportFilesOlderThan(ctx, now.Add(time.Hour)); len(swept) != 1 || swept[0].ID != "f3" {
			t.Errorf("export sweep after tombstone: %+v", swept)
		}

		if err := store.SoftDeleteFile(ctx, "f1"); err != nil {
			t.Fatalf("soft delete: %v", err)
		}
		if byRef, _ := store.ListFilesByRef(ctx, "ticket", "t1"); len(byRef) != 1 || byRef[0].ID != "f2" {
			t.Errorf("deleted file still listed by ref: %+v", byRef)
		}
		if _, err := store.GetFile(ctx, "f1"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("soft-deleted record: want ErrNotFound, got %v", err)
		}
	})

	t.Run("api keys and webhook endpoints", func(t *testing.T) {
		secret := apikeys.Prefix + "contractsecret00"
		if err := store.CreateAPIKey(ctx, apikeys.Key{ID: "ak1", UserID: "u1", Name: "ci", Prefix: secret[:len(apikeys.Prefix)+6], CreatedAt: now}, "hash-ak1"); err != nil {
			t.Fatalf("create key: %v", err)
		}
		if got, hash, err := store.GetAPIKeyByPrefix(ctx, secret[:len(apikeys.Prefix)+6]); err != nil || got.ID != "ak1" || hash != "hash-ak1" {
			t.Fatalf("get by prefix: %+v/%s err=%v", got, hash, err)
		}
		if err := store.TouchAPIKey(ctx, "ak1"); err != nil {
			t.Fatalf("touch: %v", err)
		}
		if got, _, _ := store.GetAPIKeyByPrefix(ctx, secret[:len(apikeys.Prefix)+6]); got.LastUsedAt == nil {
			t.Error("touch: want LastUsedAt stamped")
		}
		if err := store.RevokeAPIKey(ctx, "ak1", "u1"); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		if _, _, err := store.GetAPIKeyByPrefix(ctx, secret[:len(apikeys.Prefix)+6]); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("revoked key: want ErrNotFound, got %v", err)
		}

		endpoint := webhooks.Endpoint{ID: "wh1", URL: "https://hooks.example.com/x", Secret: "s", Events: []string{"*"}, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := store.InsertWebhookEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("insert endpoint: %v", err)
		}
		if got, err := store.GetWebhookEndpoint(ctx, "wh1"); err != nil || got.URL != endpoint.URL {
			t.Fatalf("get endpoint: %+v err=%v", got, err)
		}
		endpoint.Active = false
		if err := store.UpdateWebhookEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("update endpoint: %v", err)
		}
		if got, _ := store.GetWebhookEndpoint(ctx, "wh1"); got.Active {
			t.Error("update: want Active false")
		}

		deliveryAt := func(offset time.Duration) time.Time { return now.Add(offset) }
		for _, delivery := range []webhooks.Delivery{
			{ID: "d1", EndpointID: "wh1", Event: "e.old", Payload: "{}", Status: "success", Attempts: 1, CreatedAt: deliveryAt(time.Second)},
			{ID: "d2", EndpointID: "wh1", Event: "e.new", Payload: "{}", Status: "pending", CreatedAt: deliveryAt(2 * time.Second)},
		} {
			if err := store.InsertWebhookDelivery(ctx, delivery); err != nil {
				t.Fatalf("insert delivery: %v", err)
			}
		}
		logged, err := store.ListWebhookDeliveries(ctx, "wh1", 10, 0)
		if err != nil || len(logged) != 2 || logged[0].ID != "d2" {
			t.Fatalf("deliveries newest first: %+v err=%v", logged, err)
		}
		d2, _ := store.ListWebhookDeliveries(ctx, "wh1", 10, 0)
		d2[0].Status = "success"
		if err := store.UpdateWebhookDelivery(ctx, d2[0]); err != nil {
			t.Fatalf("update delivery: %v", err)
		}
		if logged, _ = store.ListWebhookDeliveries(ctx, "wh1", 10, 0); logged[0].Status != "success" {
			t.Errorf("delivery update lost: %+v", logged[0])
		}
		if err := store.DeleteWebhookEndpoint(ctx, "wh1"); err != nil {
			t.Fatalf("delete endpoint: %v", err)
		}
		if logged, _ := store.ListWebhookDeliveries(ctx, "wh1", 10, 0); len(logged) != 0 {
			t.Errorf("deliveries must cascade with the endpoint: %d", len(logged))
		}
	})

	t.Run("notifications read state", func(t *testing.T) {
		mk := func(id string, offset time.Duration) notifications.Notification {
			return notifications.Notification{ID: id, UserID: "u1", Type: "test", Title: "t-" + id, Link: "/x", CreatedAt: now.Add(offset)}
		}
		if err := store.CreateNotification(ctx, mk("n1", time.Second)); err != nil {
			t.Fatalf("create n1: %v", err)
		}
		if err := store.CreateNotification(ctx, mk("n2", 2*time.Second)); err != nil {
			t.Fatalf("create n2: %v", err)
		}
		list, err := store.ListNotifications(ctx, "u1", 10, 0)
		if err != nil || len(list) != 2 || list[0].ID != "n2" {
			t.Fatalf("list newest first: %+v err=%v", list, err)
		}
		if unread, err := store.CountUnreadNotifications(ctx, "u1"); err != nil || unread != 2 {
			t.Fatalf("unread: %d err=%v", unread, err)
		}
		if err := store.MarkNotificationRead(ctx, "n2", "u1"); err != nil {
			t.Fatalf("mark read: %v", err)
		}
		if unread, _ := store.CountUnreadNotifications(ctx, "u1"); unread != 1 {
			t.Errorf("unread after one read: want 1, got %d", unread)
		}
		if err := store.MarkAllNotificationsRead(ctx, "u1"); err != nil {
			t.Fatalf("mark all: %v", err)
		}
		if unread, _ := store.CountUnreadNotifications(ctx, "u1"); unread != 0 {
			t.Errorf("unread after mark all: want 0, got %d", unread)
		}
	})

	t.Run("audit trail filters", func(t *testing.T) {
		entries := []audit.Entry{
			{ID: "au1", ActorID: "u1", ActorEmail: "u1@example.com", Action: "user.created", ResourceType: "user", ResourceID: "u2", Diff: "{}", CreatedAt: now},
			{ID: "au2", ActorID: "u2", Action: "webhook.created", ResourceType: "webhook", ResourceID: "wh-x", Diff: "{}", CreatedAt: now.Add(time.Second)},
			{ID: "au3", ActorID: "u1", Action: "user.deleted", ResourceType: "user", ResourceID: "u3", Diff: "{}", CreatedAt: now.Add(2 * time.Second)},
		}
		for _, entry := range entries {
			if err := store.RecordAudit(ctx, entry); err != nil {
				t.Fatalf("record %s: %v", entry.ID, err)
			}
		}
		// Newest first; Postgres assigns its own row ids, so order is
		// asserted through the actions.
		all, err := store.ListAudit(ctx, kernel.AuditOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}})
		if err != nil || len(all) != 3 ||
			all[0].Action != "user.deleted" || all[1].Action != "webhook.created" || all[2].Action != "user.created" {
			t.Fatalf("list newest first: %+v err=%v", all, err)
		}
		if byActor, _ := store.ListAudit(ctx, kernel.AuditOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}, Actor: "u1"}); len(byActor) != 2 {
			t.Errorf("actor filter: %d", len(byActor))
		}
		if byAction, _ := store.ListAudit(ctx, kernel.AuditOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}, Action: "user.created"}); len(byAction) != 1 || byAction[0].ResourceID != "u2" {
			t.Errorf("action filter: %+v", byAction)
		}
		if byResource, _ := store.ListAudit(ctx, kernel.AuditOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}, Resource: "user", ResourceID: "u2"}); len(byResource) != 1 {
			t.Errorf("resource filter: %+v", byResource)
		}
		// The subject sees what they did and what was done to them.
		if subject, _ := store.ListAudit(ctx, kernel.AuditOptions{ListOptions: kernel.ListOptions{Mode: "offset", Limit: 10}, Subject: "u2"}); len(subject) != 2 {
			t.Errorf("subject filter: %+v", subject)
		}
	})

	t.Run("settings round trip", func(t *testing.T) {
		if err := store.UpsertSetting(ctx, settings.Setting{Key: "contract.flag", Value: "true", UpdatedBy: "u1", UpdatedAt: now}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		got, err := store.GetSetting(ctx, "contract.flag")
		if err != nil || got.Value != "true" {
			t.Fatalf("get: %+v err=%v", got, err)
		}
		if err := store.UpsertSetting(ctx, settings.Setting{Key: "contract.flag", Value: "false", UpdatedBy: "u1", UpdatedAt: now.Add(time.Second)}); err != nil {
			t.Fatalf("upsert again: %v", err)
		}
		if got, _ = store.GetSetting(ctx, "contract.flag"); got.Value != "false" {
			t.Errorf("upsert must overwrite: got %q", got.Value)
		}
		listed, err := store.ListSettings(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		found := false
		for _, setting := range listed {
			if setting.Key == "contract.flag" {
				found = true
			}
		}
		if !found {
			t.Error("listed settings: contract.flag missing")
		}
	})

	t.Run("full text search", func(t *testing.T) {
		at := now.Add(3 * time.Second)
		if err := store.CreateUser(ctx, users.User{ID: "usearch", Email: "searchable@example.com", Name: "Searchable Sam", CreatedAt: at, UpdatedAt: at}, ""); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		typ := tickets.TicketType{ID: "tt-search", Key: "search-key", Name: "Search", Steps: []tickets.TicketStep{{Name: "Review", ApproverRole: "approver"}}, Active: true, CreatedAt: at, UpdatedAt: at}
		if err := store.CreateTicketType(ctx, typ); err != nil {
			t.Fatalf("seed type: %v", err)
		}
		if _, err := store.CreateTicket(ctx, tickets.Ticket{ID: "t-search", OrgID: "org1", TypeID: typ.ID, TypeKey: typ.Key,
			Title: "Uniquewidget request", Priority: "low", Status: tickets.TicketDraft, CreatedBy: "u1", CreatedAt: at, UpdatedAt: at}); err != nil {
			t.Fatalf("seed ticket: %v", err)
		}

		users_, err := store.SearchUsers(ctx, "Searchable", 10)
		if err != nil || len(users_) != 1 || users_[0].ID != "usearch" {
			t.Errorf("user search: %+v err=%v", users_, err)
		}
		found, err := store.SearchTickets(ctx, "org1", "uniquewidget", 10)
		if err != nil || len(found) != 1 || found[0].ID != "t-search" {
			t.Errorf("ticket search: %+v err=%v", found, err)
		}
		if miss, _ := store.SearchTickets(ctx, "elsewhere", "uniquewidget", 10); len(miss) != 0 {
			t.Errorf("ticket search outside the org: %d hits", len(miss))
		}
	})

	t.Run("rbac orgs roles members", func(t *testing.T) {
		if err := store.CreateRole(ctx, rbac.Role{Key: "contract-role", Name: "Contract", CreatedAt: now}); err != nil {
			t.Fatalf("create role: %v", err)
		}
		// Creation is idempotent (ON CONFLICT DO NOTHING in Postgres): a
		// duplicate key keeps the original record.
		if err := store.CreateRole(ctx, rbac.Role{Key: "contract-role", Name: "Dup", CreatedAt: now}); err != nil {
			t.Errorf("duplicate role key: want silent no-op, got %v", err)
		}
		if got, _ := store.GetRole(ctx, "contract-role"); got.Name != "Contract" {
			t.Errorf("duplicate create must keep the original: got %q", got.Name)
		}
		if got, err := store.GetRole(ctx, "contract-role"); err != nil || got.Name != "Contract" {
			t.Fatalf("get role: %+v err=%v", got, err)
		}
		if err := store.UpdateRole(ctx, rbac.Role{Key: "contract-role", Name: "Contract", Description: "updated"}); err != nil {
			t.Fatalf("update role: %v", err)
		}
		if listed, err := store.ListRoles(ctx); err != nil {
			t.Fatalf("list roles: %v", err)
		} else {
			found := false
			for _, role := range listed {
				if role.Key == "contract-role" {
					found = true
				}
			}
			if !found {
				t.Error("role list: contract-role missing")
			}
		}
		if err := store.DeleteRole(ctx, "contract-role"); err != nil {
			t.Fatalf("delete role: %v", err)
		}
		if _, err := store.GetRole(ctx, "contract-role"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("deleted role: want ErrNotFound, got %v", err)
		}

		org := rbac.Organization{ID: "org-c1", Name: "Contract Org", Slug: "contract-org", CreatedAt: now, UpdatedAt: now}
		if err := store.CreateOrg(ctx, org); err != nil {
			t.Fatalf("create org: %v", err)
		}
		// Creation is idempotent (ON CONFLICT (id) DO NOTHING).
		if err := store.CreateOrg(ctx, org); err != nil {
			t.Errorf("duplicate org id: want silent no-op, got %v", err)
		}
		if got, _ := store.GetOrg(ctx, "org-c1"); got.Name != "Contract Org" {
			t.Errorf("duplicate create must keep the original: got %q", got.Name)
		}
		org.Name = "Contract Org v2"
		if err := store.UpdateOrg(ctx, org); err != nil {
			t.Fatalf("update org: %v", err)
		}
		if got, _ := store.GetOrg(ctx, "org-c1"); got.Name != "Contract Org v2" {
			t.Errorf("org update lost: %+v", got)
		}
		if err := store.AddOrgMember(ctx, rbac.OrgMember{OrgID: "org-c1", UserID: "u1", Email: "u1@example.com", Name: "name-u1", JoinedAt: now}); err != nil {
			t.Fatalf("add member: %v", err)
		}
		if member, _ := store.IsOrgMember(ctx, "org-c1", "u1"); !member {
			t.Error("membership: want true")
		}
		if members, err := store.ListOrgMembers(ctx, "org-c1"); err != nil || len(members) != 1 {
			t.Errorf("members: %d err=%v", len(members), err)
		}
		if orgs, err := store.ListOrgsForUser(ctx, "u1"); err != nil {
			t.Fatalf("orgs for user: %v", err)
		} else {
			found := false
			for _, o := range orgs {
				if o.ID == "org-c1" {
					found = true
				}
			}
			if !found {
				t.Error("orgs for u1: org-c1 missing")
			}
		}
		if err := store.RemoveOrgMember(ctx, "org-c1", "u1"); err != nil {
			t.Fatalf("remove member: %v", err)
		}
		if member, _ := store.IsOrgMember(ctx, "org-c1", "u1"); member {
			t.Error("membership after removal: want false")
		}
		if orgs, next, err := store.ListOrgs(ctx, kernel.ListOptions{Mode: "offset", Limit: 10}); err != nil || len(orgs) == 0 {
			t.Errorf("list orgs: %d next=%q err=%v", len(orgs), next, err)
		}
		if err := store.DeleteOrg(ctx, "org-c1"); err != nil {
			t.Fatalf("delete org: %v", err)
		}
		if _, err := store.GetOrg(ctx, "org-c1"); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("deleted org: want ErrNotFound, got %v", err)
		}
	})

	t.Run("idempotency reservations", func(t *testing.T) {
		record := kernel.IdempotencyRecord{Key: "idem-1", UserID: "u1", Method: "POST", Path: "/api/x", ExpiresAt: now.Add(time.Hour)}
		stored, already, err := store.ReserveIdempotency(ctx, record)
		if err != nil || already {
			t.Fatalf("first reserve: already=%v err=%v", already, err)
		}
		if _, already, _ := store.ReserveIdempotency(ctx, record); !already {
			t.Error("second reserve: want already=true")
		}
		if err := store.CompleteIdempotency(ctx, "idem-1", "u1", 201, []byte(`{"ok":true}`), "application/json"); err != nil {
			t.Fatalf("complete: %v", err)
		}
		stored, already, _ = store.ReserveIdempotency(ctx, record)
		if !already || stored.Status != 201 || string(stored.ResponseBody) != `{"ok":true}` {
			t.Errorf("replay: already=%v status=%d body=%s", already, stored.Status, stored.ResponseBody)
		}
		// Keys are per-user.
		if _, already, _ := store.ReserveIdempotency(ctx, kernel.IdempotencyRecord{Key: "idem-1", UserID: "u2", ExpiresAt: now.Add(time.Hour)}); already {
			t.Error("same key for another user: want fresh reservation")
		}

		// Expired reservations prune away and free the key.
		expired := kernel.IdempotencyRecord{Key: "idem-old", UserID: "u1", Method: "POST", Path: "/api/x", ExpiresAt: now.Add(-time.Minute)}
		if _, _, err := store.ReserveIdempotency(ctx, expired); err != nil {
			t.Fatalf("reserve expired: %v", err)
		}
		if _, err := store.PruneIdempotency(ctx); err != nil {
			t.Fatalf("prune: %v", err)
		}
		if _, already, _ := store.ReserveIdempotency(ctx, expired); already {
			t.Error("pruned key: want reservable again")
		}
	})
}
