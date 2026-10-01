package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"golang.org/x/crypto/argon2"
)

// HashPassword derives an Argon2id encoding of password. It is exported for
// seeding paths that bypass Register (the demo dataset) and must stay the
// single place password encodings are produced.
func HashPassword(password string) string {
	return hashPassword(password)
}

// VerifyPassword reports whether password matches the Argon2id encoding.
// Exported alongside HashPassword for features that take their verifier
// injected (the token grant).
func VerifyPassword(encoded, password string) bool {
	return verifyPassword(encoded, password)
}

func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(hash)
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
