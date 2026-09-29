# ADR 0020 — Postgres tsvector with the 'simple' Config for Full-Text Search

**Status:** Accepted

## Context

The product needs one search box over users and tickets, permission-
scoped, without standing up Elasticsearch/OpenSearch (a second search
platform for two entity types is not a trade a template should make).
Postgres full-text search can serve this — the design question is text
configuration and placement.

## Decision

Generated `tsvector` **STORED** columns with **GIN** indexes:
`to_tsvector('simple', name || ' ' || email)` on users and
`to_tsvector('simple', title || ' ' || description)` on tickets
(migration 000016). Queries use `search_vec @@ websearch_to_tsquery('simple', ?)`
ranked by `ts_rank(...) DESC, created_at DESC`. The **`'simple'`
configuration is deliberate**: no stemming, so mixed English/Russian
content (this project's demo data and audience) stays findable without a
language guess. The service layer enforces permissions *before* search:
users need `users:read`, tickets need `tickets:read` plus `read_all`
else results filter to the caller's own. The memory adapter mirrors the
contract with case-insensitive substring matching. Limits: 10 default,
25 max; empty query returns empty.

## Consequences

**Positive:** zero extra infrastructure; indexes update transactionally
with the data (no sync pipeline); `websearch_to_tsquery` never throws on
user input (safe operator syntax); permission scoping composes with the
existing RBAC layer instead of bypassing it.

**Negative:** no stemming/ranking sophistication (`ts_rank` is shallow —
recency tie-break does real work); relevance for large corpora will
plateau; CJK input needs a different parser config (extension territory
— an honest boundary). The memory fake is substring-only, so dev-mode
search is looser than Postgres (contract tests cover the *contract*, not
relevance parity).

## Alternatives considered

- *pg_trgm similarity search* — better for typo-tolerant name lookup,
  worse for phrase/word queries; a candidate companion index, not a
  replacement.
- *Elasticsearch/Meilisearch/Typesense* — the right tool at corpus sizes
  or relevance requirements this template doesn't have; the
  `SearchRepo` port is where one would plug in.
- *Default english config* — silently breaks Russian content; rejected.
