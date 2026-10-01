package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/webhooks"
)

// seedDemoAudit writes a believable admin trail over the last 30 days.
func seedDemoAudit(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	actions := []struct {
		action, resource string
	}{
		{"user.created", "user"}, {"user.updated", "user"}, {"user.logged_in", "user"},
		{"role.permissions_updated", "role"}, {"org.created", "org"}, {"org.member_added", "org"},
		{"settings.updated", "setting"}, {"webhook.created", "webhook"},
	}
	for i := 0; i < demoAuditN; i++ {
		a := pick(rng, actions)
		actorN := 1 + rng.Intn(demoLoginable)
		entry := audit.Entry{
			ID:           demoIDPrefix + "a-" + pad4(i+1),
			ActorID:      demoIDPrefix + "u-" + pad4(actorN),
			ActorEmail:   demoIDPrefix + "u-" + pad4(actorN) + demoDataDomain,
			Action:       a.action,
			ResourceType: a.resource,
			ResourceID:   demoIDPrefix + "u-" + pad4(1+rng.Intn(demoUserCount)),
			Domain:       "*",
			Diff:         "{}",
			IP:           fmt.Sprintf("203.0.113.%d", 1+rng.Intn(200)),
			RequestID:    demoIDPrefix + "r-" + pad4(1+rng.Intn(9999)),
			CreatedAt:    now.Add(-time.Duration(rng.Int63n(int64(30 * 24 * time.Hour)))),
		}
		if err := repos.Audit.RecordAudit(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// seedDemoNotifications gives the bell something to show; a third stay
// unread so the badge is live at first login.
func seedDemoNotifications(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	titles := []struct{ typ, title, body, link string }{
		{"job.done", "Job user.enrich_batch finished", "Completed successfully.", "/jobs"},
		{"job.failed", "Job user.report_generate failed", "transient upstream status 503", "/jobs"},
		{"security.password_changed", "Password changed", "Your password was reset via email.", "/profile"},
		{"impersonation.started", "An administrator signed in to your account", "Contact support if this was not expected.", "/profile"},
	}
	// Address them to the loginable users so demo logins see a full bell.
	for i := 0; i < demoNotifN; i++ {
		userID := demoIDPrefix + "u-" + pad4(1+rng.Intn(demoLoginable))
		t := pick(rng, titles)
		notification := notifications.Notification{
			ID: demoIDPrefix + "n-" + pad4(i+1), UserID: userID,
			Type: t.typ, Title: t.title, Body: t.body, Link: t.link,
			CreatedAt: now.Add(-time.Duration(rng.Int63n(int64(14 * 24 * time.Hour)))),
		}
		if i%3 != 0 { // every third stays unread
			readAt := notification.CreatedAt.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
			if readAt.After(now) {
				readAt = now
			}
			notification.ReadAt = &readAt
		}
		if err := repos.Notifications.CreateNotification(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

// seedDemoAPIKeys issues two keys attributed to the demo admin; only
// deterministic hashes are stored, so nobody can authenticate with them.
func seedDemoAPIKeys(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoAPIKeyN; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("demo-key-%d", i)))
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(60 * 24 * time.Hour))))
		var expires *time.Time
		if i == 2 {
			t := createdAt.Add(720 * time.Hour)
			expires = &t
		}
		key := apikeys.Key{
			ID: fmt.Sprintf("%skey-%d", demoIDPrefix, i), UserID: demoIDPrefix + "u-0001",
			Name:       pick(rng, []string{"CI integration", "Local scripts", "Analytics exporter"}),
			Prefix:     "gtpl_demo" + strconv.Itoa(i),
			Scopes:     []string{"users:read", "jobs:read"},
			LastUsedAt: &createdAt, ExpiresAt: expires, CreatedAt: createdAt,
		}
		if err := repos.APIKeys.CreateAPIKey(ctx, key, base64.RawURLEncoding.EncodeToString(sum[:])); err != nil {
			return err
		}
	}
	return nil
}

// seedDemoWebhooks writes one inactive endpoint (no real external calls)
// plus a delivery history for the log UI.
func seedDemoWebhooks(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	endpoint := webhooks.Endpoint{
		ID:          demoIDPrefix + "wh-0001",
		URL:         "https://hooks.demo.example/platform",
		Secret:      "demo-secret-not-used-for-delivery",
		Events:      []string{"user.logged_in", "job.done", "job.failed"},
		Description: "Sample subscription (inactive — flip it on with your own URL)",
		Active:      false, OrgID: "", CreatedBy: demoIDPrefix + "u-0001",
		CreatedAt: now.Add(-30 * 24 * time.Hour), UpdatedAt: now.Add(-30 * 24 * time.Hour),
	}
	if err := repos.Webhooks.InsertWebhookEndpoint(ctx, endpoint); err != nil {
		return err
	}
	for i := 0; i < demoWebhookN; i++ {
		deliveredAt := now.Add(-time.Duration(rng.Int63n(int64(14 * 24 * time.Hour))))
		status := "success"
		if i%5 == 0 {
			status = "failed"
		}
		attempts := 1
		lastError := ""
		responseStatus := 200
		if status == "failed" {
			responseStatus = 503
			lastError = "endpoint answered 503"
			attempts = 3
		}
		delivery := webhooks.Delivery{
			ID: demoIDPrefix + "wd-" + pad4(i+1), EndpointID: endpoint.ID,
			Event:   pick(rng, []string{"user.logged_in", "job.done", "job.failed"}),
			Payload: "{}", Status: status, Attempts: attempts,
			LastResponseStatus: responseStatus, LastError: lastError,
			CreatedAt: deliveredAt,
		}
		if status == "success" {
			delivery.DeliveredAt = &deliveredAt
		}
		if err := repos.Webhooks.InsertWebhookDelivery(ctx, delivery); err != nil {
			return err
		}
	}
	return nil
}
