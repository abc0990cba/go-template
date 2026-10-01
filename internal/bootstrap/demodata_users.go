package bootstrap

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/users"
)

// demoAccountNames pin readable names onto the six platform demo accounts
// so the login-page table, the README, and the IdentityChip all agree.
var demoAccountNames = map[int]string{
	1: "Ada Admin",
	2: "Max Manager",
	3: "Vera Viewer",
	4: "Rita Requester",
	5: "Ari Approver",
	6: "Sam Staff",
}

// seedDemoUsers creates the marker plus the demo population; users 1..10
// carry a working password (password123), the rest are passwordless accounts
// as if provisioned by an external system.
func seedDemoUsers(ctx context.Context, repos Repos, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoUserCount; i++ {
		id := demoIDPrefix + "u-" + pad4(i)
		first, last := pick(rng, demoFirst), pick(rng, demoLast)
		createdAt := now.Add(-time.Duration(rng.Int63n(int64(demoUserSpan))))
		email := strings.ToLower(first) + "." + strings.ToLower(last) + "." + strconv.Itoa(i) + demoDataDomain
		// The loginable head of the dataset gets predictable addresses so
		// the quickstart can name them: demo-u-0001@demo.example, …
		if i <= demoLoginable {
			email = id + demoDataDomain
		}
		name := first + " " + last
		if fixed, ok := demoAccountNames[i]; ok {
			name = fixed
		}
		user := users.User{
			ID: id,
			// Every demo user carries its id as external_ref, so the
			// enrichment demos can target them: devtools kafka-profile
			// --ref demo-u-0001 merges into that account.
			ExternalRef: id,
			Email:       email,
			Name:        name,
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}
		// Argon2id at ~20 ms per hash: ten hashes are startup-cheap and give
		// the quickstart ten real logins. The hash goes through the service
		// helper so logins verify exactly like Register-issued credentials.
		hash := ""
		if i <= demoLoginable {
			hash = auth.HashPassword(demoPassword)
		}
		if err := repos.Users.CreateUser(ctx, user, hash); err != nil && !errors.Is(err, kernel.ErrConflict) {
			return err
		}
	}
	return nil
}

// seedDemoIdentities links OAuth identities to a slice of the population.
// linked_at is set by the store to "now"; the backfilled oauth_linked
// events carry the historical timestamps instead.
func seedDemoIdentities(ctx context.Context, services *Container, rng *rand.Rand, now time.Time) error {
	for i := 1; i <= demoUserCount; i++ {
		id := demoIDPrefix + "u-" + pad4(i)
		user, err := services.Users.Get(ctx, id)
		if err != nil {
			continue
		}
		switch {
		case i%4 == 0:
			_ = services.Auth.LinkIdentity(ctx, user.ID, oauth.Profile{
				Provider: "google", ProviderSubject: "google-demo-" + strconv.Itoa(i),
				Email: user.Email, Name: user.Name,
			})
		case i%8 == 0:
			_ = services.Auth.LinkIdentity(ctx, user.ID, oauth.Profile{
				Provider: "github", ProviderSubject: strconv.Itoa(700000 + i),
				Email: user.Email, Name: user.Name,
			})
		}
	}
	_ = now
	return nil
}
