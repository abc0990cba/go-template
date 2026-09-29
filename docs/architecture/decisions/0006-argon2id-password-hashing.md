# ADR 0006 — Argon2id for Password Hashing

**Status:** Accepted

## Context

Password storage is the one cryptographic choice almost every web app
gets legally and reputationally graded on. bcrypt is aging (64 MiB-class
GPU attacks, 72-byte truncation); PBKDF2 is GPU/FPGA-friendly; scrypt
lacked PHC winner status. OWASP's current guidance lists **Argon2id**
first.

## Decision

Hash passwords with Argon2id, parameters `time=1, memory=64 MiB,
threads=4, keyLen=32`, 16-byte random salt, encoded as
`base64(salt) + ":" + base64(hash)`, compared with
`crypto/subtle.ConstantTimeCompare`. The function is exported
(`service.HashPassword`) so seeders share one hash across bulk rows.

## Consequences

**Positive:** memory-hardness makes GPU cracking expensive; parameters
live in one function and are trivially tunable; stdlib `golang.org/x/crypto`
means no third-party crypto code.

**Negative:** 64 MiB per hash bounds login throughput (the k6 login
scenario, 50 VU at p95 < 300 ms, exists to keep this honest); CPU-only
servers should lower memory and raise time — the single-function location
makes that a one-line change plus a migration note for old hashes
(rehash-on-login is the standard upgrade path and is deliberately left as
an adopter extension).

## Alternatives considered

- *bcrypt* — battle-tested but weaker against GPU attack and truncates at
  72 bytes.
- *scrypt* — fine, but Argon2id is the PHC winner and OWASP's first pick.
- *argon2 with stronger defaults (t=2/3)* — template defaults favor
  demo/dev speed; production adopters should raise them.
