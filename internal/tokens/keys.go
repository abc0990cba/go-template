package tokens

// Key rotation for the HS256 signing secrets. Tokens carry a kid header
// naming the key that signed them; verification looks the key up by kid, so
// a deployment rolls from key A to key B by adding B as active while A
// stays listed, and drops A only after every outstanding access token has
// expired anyway (minutes). Tokens without a kid verify against the legacy
// single secret (APP_JWT_SECRET), which keeps the migration path open.

import (
	"encoding/json"
	"fmt"
	"sort"
)

// MinKeyBytes matches the APP_JWT_SECRET floor in config: shorter secrets
// are brute-forceable at HS256's speed.
const MinKeyBytes = 32

// KeySet is an ordered kid → secret map plus the kid new tokens use.
type KeySet struct {
	active string
	keys   map[string][]byte
	// legacy verifies kid-less tokens minted before the keyset existed.
	legacy []byte
}

// keySetFile is the on-disk shape (a mounted secret file).
type keySetFile struct {
	Active string            `json:"active"`
	Keys   map[string]string `json:"keys"`
}

// ParseKeySet validates a keyset file's contents. The active key must exist
// and every secret must meet the length floor.
func ParseKeySet(data []byte, legacySecret string) (*KeySet, error) {
	var file keySetFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("jwt keyset: %w", err)
	}
	if file.Active == "" {
		return nil, fmt.Errorf("jwt keyset: active key is required")
	}
	if len(file.Keys) == 0 {
		return nil, fmt.Errorf("jwt keyset: at least one key is required")
	}
	set := &KeySet{active: file.Active, keys: map[string][]byte{}}
	for kid, secret := range file.Keys {
		if len(secret) < MinKeyBytes {
			return nil, fmt.Errorf("jwt keyset: key %q must be at least %d bytes, got %d", kid, MinKeyBytes, len(secret))
		}
		set.keys[kid] = []byte(secret)
	}
	if _, ok := set.keys[file.Active]; !ok {
		return nil, fmt.Errorf("jwt keyset: active key %q is not in the key list", file.Active)
	}
	if legacySecret != "" {
		set.legacy = []byte(legacySecret)
	}
	return set, nil
}

// Active returns the kid and secret new tokens are signed with.
func (ks *KeySet) Active() (string, []byte) { return ks.active, ks.keys[ks.active] }

// Get resolves a kid to its secret.
func (ks *KeySet) Get(kid string) ([]byte, bool) {
	secret, ok := ks.keys[kid]
	return secret, ok
}

// Kids lists the known key ids, sorted for deterministic output.
func (ks *KeySet) Kids() []string {
	kids := make([]string, 0, len(ks.keys))
	for kid := range ks.keys {
		kids = append(kids, kid)
	}
	sort.Strings(kids)
	return kids
}

// WithKeys attaches a keyset; new tokens carry the active kid and old kids
// keep verifying. A keyset also enables the token endpoints even without
// APP_JWT_SECRET (the keyset's active key is the signing secret).
func (s *Service) WithKeys(keys *KeySet) *Service {
	if keys != nil {
		s.keyset = keys
	}
	return s
}

// keyFor resolves the verification key for one token: its kid when it
// carries one, the legacy secret when it does not.
func (s *Service) keyFor(kid any) ([]byte, error) {
	if kid == nil {
		if s.keyset != nil {
			if s.keyset.legacy != nil {
				return s.keyset.legacy, nil
			}
			return nil, fmt.Errorf("%w: token has no kid", ErrInvalidGrant)
		}
		return s.secret, nil
	}
	if s.keyset == nil {
		// A kid on a token from a single-secret deployment: forged or stale.
		return nil, fmt.Errorf("%w: unknown token kid", ErrInvalidGrant)
	}
	kidStr, ok := kid.(string)
	if !ok {
		return nil, fmt.Errorf("%w: malformed token kid", ErrInvalidGrant)
	}
	secret, known := s.keyset.Get(kidStr)
	if !known {
		return nil, fmt.Errorf("%w: unknown token kid", ErrInvalidGrant)
	}
	return secret, nil
}
