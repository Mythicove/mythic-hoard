# Mythic Hoard

**English** | [简体中文](./README.zh-CN.md)

Minecraft mod / modpack distribution backend, positioned for **private-group distribution** (paired with Mirror, the skin station that provides the unified account system). Offers a Modrinth-compatible read API + `.mrpack` generation.

## Status

User system wired up (v0.3.0):

- **Project → Version → File three-level model**: Postgres (sqlc) or in-memory store; local disk or S3-compatible object storage
- **SSO login**: Mirror acts as OIDC IdP (authorization code flow); session in httpOnly cookie `hoard_session` (7 days, server-revocable); subjects in the `HOARD_ADMIN_SUBJECTS` allowlist get `role=admin` on login
- **Teams + PATs**: team creation / member add-remove / member listing; `mhp_`-prefixed personal access tokens (sha256 at rest, scopes, expiry, last_used); uploads require the `UPLOAD` scope
- **User-level uploads**: `POST /v2/project`, `POST /v2/version` (multipart, automatic jar metadata parsing), `DELETE /v2/project/{id}`
- **Mod metadata parsing** (`internal/modmeta`): fabric.mod.json / quilt.mod.json / mods.toml / neoforge.mods.toml; auto-fills name/id/version/dependencies, icon extraction, conflict flags
- **Version dependencies** (Modrinth shape) + compliant `.mrpack` (`minecraft` dependency, env, overrides)
- **Modrinth-compatible reads** (subset): `/v2/search` (incl. facets, inner OR / outer AND), project/version/dependency queries, tag lists, icons
- **Dual search implementations**: `pg_trgm` (default, exact totals) / Meilisearch (index auto-provisioned on first access: filterableAttributes + `maxTotalHits=1e6` for exact totals); queue inline / asynq
- **Admin side**: legacy uploads via admin token, review state transitions (`listed|under_review|rejected|archived`), deletion; `HOARD_REQUIRE_REVIEW` can force new uploads into review

Not done: upstream mirror proxying, gallery/screenshots. Invites expire after 7 days (`InviteTTL`, refreshed on re-invite; expired invites are treated as nonexistent on every read path). Content reports and the skin review queue live in **Mirror** (`POST /reports`, `/api/admin/reports*`, `/api/admin/skins*`), not here. Full gap list in `docs/plans/status.md`.

## Endpoints

### Infrastructure

| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Liveness probe |
| GET | `/metrics` | Prometheus metrics |

### Auth (OIDC RP; the whole group 404s if `HOARD_OIDC_ISSUER` is empty)

| Method | Path | Description |
|---|---|---|
| GET | `/auth/oidc/login` | Start SSO; the `?next=` return URL must match the CORS allowlist (open-redirect protection) |
| GET | `/auth/oidc/callback` | Authorization code callback, sets the session cookie |
| POST | `/auth/logout` | Revoke the session |
| GET | `/auth/me` | Current identity (`sub`/`username`/`role`) |

### User (session cookie or `mhp_` PAT)

| Method | Path | Description |
|---|---|---|
| POST | `/api/team` | Create a team (creator becomes owner) |
| GET | `/api/user/teams` | My teams |
| POST | `/api/team/{id}/members` | Change an **existing member's** role / transfer ownership (owner or site admin); non-member → 409 (use an invite instead); demoting the sole owner → 409 |
| DELETE | `/api/team/{id}/members/{sub}` | Remove a member or leave; sole owner leaving → 409 (transfer ownership first) |
| POST | `/api/team/{id}/invites` | Invite a user (owner or site admin); already a member → 409; valid for 7 days, re-inviting refreshes |
| GET | `/api/team/{id}/invites` | Pending invites of a team |
| DELETE | `/api/team/{id}/invites/{sub}` | Revoke an invite |
| GET | `/api/user/invites` | My pending invites |
| POST | `/api/user/invites/{teamId}/accept` | Accept an invite (writes membership, consumes the invite) |
| DELETE | `/api/user/invites/{teamId}` | Decline an invite |
| GET | `/api/user/pats` | My tokens (plaintext not included) |
| POST | `/api/user/pats` | Create a token (plaintext returned once) |
| DELETE | `/api/user/pats/{id}` | Revoke a token |
| POST | `/v2/project` | Create a project (multipart `data` + `file`, requires `UPLOAD` scope) |
| POST | `/v2/version` | Publish a version (requires project team membership + `UPLOAD` scope) |
| DELETE | `/v2/project/{id}` | Delete own project |
| GET | `/api/team/{id}/members` | Team members (public read) |

### Admin (login session with `role=admin` **or** `HOARD_ADMIN_TOKEN` bearer)

Both credentials are equivalent, matching Mirror's admin console: day-to-day use goes through SSO sessions (`HOARD_ADMIN_SUBJECTS` mapped to admin); the static token is only for bootstrap/rescue. **Fail closed**: leaving the token empty no longer opens the admin endpoints — only admin sessions pass.

| Method | Path | Description |
|---|---|---|
| POST | `/api/mods` | Legacy admin upload (multipart) |
| PATCH | `/api/mods/{id}/status` | Review state transition |
| DELETE | `/api/mods/{id}` | Delete a project |

### Public reads (Modrinth-compatible + native)

| Method | Path | Description |
|---|---|---|
| GET | `/api/mods` | Project list/search (`q`/`offset`/`limit`/`facets`/`loaders`/`game_versions`/`categories`/`project_type`) |
| GET | `/api/mods/facets` | Available facet values (loaders / game_versions / categories) |
| GET | `/api/mods/{id}` | Project detail |
| GET | `/api/mods/{id}/icon` | Project icon |
| GET | `/api/mods/{id}/download` | Download latest primary file |
| GET | `/api/mods/{id}/version/{vid}/file/{fid}` | Download a specific file |
| GET | `/v2/search` | Modrinth-compatible search |
| GET | `/v2/project/{id}` | Modrinth-compatible project |
| GET | `/v2/project/{id}/icon` | Icon |
| GET | `/v2/project/{id}/version` | Version list |
| GET | `/v2/project/{id}/dependencies` | Version dependencies |
| GET | `/v2/version/{id}` | Single version |
| GET | `/v2/project/{id}/version/{vid}/mrpack` | `.mrpack` generation |
| GET | `/v2/tag/{loader,game_version,category}` | Metadata lists |

Full contract in [Mythicove/api](https://github.com/Mythicove/api) `specs/hoard.openapi.yaml` (guarded by this repo's `specdrift_test.go`: every path+method in the spec must actually be registered).

## Run

```bash
cp .env.example .env   # adjust as needed; without DATABASE_URL it uses the in-memory store (development)
go run ./cmd/server    # listens on :8081
```

When sharing one Postgres database with Mirror, schema isolation is **mandatory**, otherwise same-named tables (`users`/`sessions`/`files`, ...) collide:

```
HOARD_DATABASE_URL=postgres://mirror:mirror@db:5432/mirror?sslmode=disable&options=-csearch_path%3Dhoard
```

The SSO issuer must be reachable from both the **browser** and the **hoard container**. When the two are on different networks (typical: the issuer is a public domain while containers use service names), use split horizons:

```
HOARD_OIDC_ISSUER=https://api.example.com      # must exactly match mirror's MIRROR_BASE_URL (iss validation)
HOARD_OIDC_INTERNAL_BASE_URL=http://mirror:8080 # only for in-container discovery/token endpoint access
```

## Development

```bash
go test ./...   # pgstore integration tests need HOARD_TEST_DATABASE_URL; spec drift tests need the api repo alongside (or MYTHICOVE_API_SPECS)
sqlc generate   # regenerate internal/pgstore/db (CI has a drift gate)
```

Order for adding a persisted entity: `migrations/0000N_*.sql` (goose, 5-digit sequence) → sync DDL into `internal/pgstore/schema.sql` → add named queries to `internal/pgstore/query.sql` → `sqlc generate` → extend the `Store` interface in `internal/hoard/store.go` → implement both `pgstore.Store` and `hoard.MemoryStore` (tests use the latter).

## License

[Elastic License 2.0 (ELv2)](./LICENSE)

This repository was previously released under AGPL-3.0 (v0.1.0–v0.3.0) and relicensed to ELv2 on 2026-10-26; see [LICENSE.history](./LICENSE.history) for details.
