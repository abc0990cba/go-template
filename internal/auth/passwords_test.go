package auth

import (
	"testing"
)

var sink string

// Benchmarks the Argon2id hashing parameters; they dominate register and
// login latency and must stay within the intended CPU budget.
func BenchmarkHashPassword(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = hashPassword("password123")
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	hash := hashPassword("password123")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		sink = sink + ""
		if !verifyPassword(hash, "password123") {
			b.Fatal("verify failed")
		}
	}
}
