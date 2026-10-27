# Changelog

**English** | [简体中文](./CHANGELOG.zh-CN.md)

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and semantic versioning.

## [0.3.0] - 2026-10-23

Phase 3 (user system & open uploads): regular users can publish mods without an admin token.

### Added

- **OIDC RP SSO** (Mirror as IdP): `internal/oidcrp` (go-oidc v3; discovery / JWKS rotation / signature / iss / aud / exp / nonce all validated); `/auth/oidc/login|callback`, `/auth/logout`, `/auth/me`; state/nonce in short-lived httpOnly cookies; session cookie `hoard_session` (httpOnly+Lax, 7d, server-revocable); user projection table (migration 00002, re-login does not overwrite a promoted role); `HOARD_ADMIN_SUBJECTS` allowlist maps to admin idempotently
- **Teams & PATs** (migration 00003): `teams`/`team_members` (owner/member two levels); personal access tokens with `mhp_` prefix, sha256 hash storage, scopes (enforced once declared; uploads require UPLOAD), expiry, last_used, revocable; `projects.team_id` nullable for existing rows
- **User-level uploads**: `POST /v2/project` (Modrinth-shaped multipart, auto-creates team ownership), `POST /v2/version` (team membership check), `DELETE /v2/project/{id}` (owner/admin); the admin `/api/mods` queue is retained; `HOARD_REQUIRE_REVIEW` review switch
- **Mod metadata parsing** (migration 00004): `internal/modmeta` reads fabric/quilt/(neo)forge metadata inside jars → auto-fills title/description/loaders/game_versions/license/version/icon; form values win, conflicts recorded in `metadata_flags`; extracted icons go to object storage + served at `/api/mods/{id}/icon`
- **Version dependencies** (migration 00005): required/optional/incompatible/embedded; `GET /v2/project/{id}/dependencies`; mrpack compliance improvements (minecraft dependency version, env, overrides/)
- CORS allowlist mode now permits credentials (SSO cookies)

### Fixed

- Meilisearch semantics bug where multi-value facet filters on the same key were ANDed (should be OR); `total_hits` now uses estimatedTotalHits (shared/search v0.1.2)

## [0.2.0] - 2026-10-23

Phase 2 (production readiness & ops baseline).

### Added

- Redis rate-limit implementation (`internal/redislimit`, atomic Lua fixed window, counts shared across replicas, fail-open fallback to in-process)
- `/metrics` Prometheus endpoint (HTTP latency/counts + Go runtime)
- Per-file size cap `HOARD_MAX_FILE_BYTES` (default 512MiB, 413 when exceeded)
- goose migrations serialized via advisory lock (safe concurrent startup across replicas)

## [0.1.0] - 2026-10-23

First usable version: private-group mod distribution MVP.

### Added

- Project → Version → File three-level model (Postgres/sqlc or in-memory store), local disk or S3
- Modrinth-compatible read subset: `/v2/search` (q + facets/loaders/game_versions/project_type/categories), `/v2/project/{id}`, `/v2/project/{id}/version`, `/v2/version/{id}`, `/v2/tag/*`
- `.mrpack` generation: with `dependencies` (loader mapping) and `mods/` path prefix
- Upload/review: multipart uploads (SHA-1/SHA-512 hashes), review state transitions, `HOARD_ADMIN_TOKEN` auth
- Dual search implementations (pg_trgm / Meilisearch), dual queue implementations (inline / asynq), download counts
- Security: trusted-proxy-aware IP resolution (`shared/clientip`), per-IP fixed-window rate limiting (TTL sweep), security response headers, `ReadHeaderTimeout`/`IdleTimeout`, CORS allowlist (`HOARD_CORS_ORIGINS`, fixed missing PATCH)
- Engineering: sqlc + CI drift check, contract drift gate, Postgres integration tests, gofmt gate, govulncheck

### Known limitations

- **No user system**: uploads/review rely on the admin token only; users/teams/API tokens arrive in Phase 3 (via Mirror OIDC)
- No version dependencies, no mod metadata parsing, no upstream mirror proxy (Phase 3)
