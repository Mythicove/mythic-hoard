# Mythic Hoard

**English** | [简体中文](./README.zh-CN.md)

A distribution backend for Minecraft mods and modpacks, aimed at private-group distribution and paired with Mirror, the skin station that provides the unified account system. It offers a Modrinth-compatible read API and .mrpack generation.

## Status

The user system is wired up (v0.3.0):

- Project, version and file as a three-level model, on Postgres (sqlc) or in-memory storage, with local disk or S3-compatible object storage.
- SSO login: Mirror acts as the OIDC IdP (authorization code flow). Sessions live in the httpOnly cookie hoard_session (7 days, revocable server-side). Subjects in the HOARD_ADMIN_SUBJECTS allowlist get role=admin on login.
- Teams and PATs: team creation, member add and remove, member listing; personal access tokens with the mhp_ prefix (sha256 at rest, scopes, expiry, last_used). Uploads require the UPLOAD scope.
- User-level uploads: POST /v2/project, POST /v2/version (multipart, jar metadata parsed automatically) and DELETE /v2/project/{id}.
- Mod metadata parsing (internal/modmeta): reads fabric.mod.json, quilt.mod.json, mods.toml and neoforge.mods.toml, fills name, id, version and dependencies, extracts the icon and flags conflicts.
- Version dependencies in the Modrinth shape, plus .mrpack compliance (minecraft dependency, env, overrides).
- Modrinth-compatible reads (a subset): /v2/search (with facets, inner OR and outer AND), project, version and dependency queries, tag lists and icons.
- Two search implementations: pg_trgm (default, exact totals) and Meilisearch (index provisioned on first access, with filterableAttributes and maxTotalHits=1e6 for exact totals). Queues run inline or on asynq.
- Admin side: legacy uploads with an admin token, review state transitions (listed, under_review, rejected, archived) and deletion. HOARD_REQUIRE_REVIEW can force new uploads into review.

Not done: upstream mirror proxying, gallery and screenshots. Invites expire after 7 days (InviteTTL, refreshed on re-invite; expired invites are treated as nonexistent on every read path). Content reports and the skin review queue live in Mirror (POST /reports, /api/admin/reports*, /api/admin/skins*), not here. The full gap list is in docs/plans/status.md.

## Endpoints

### Infrastructure

| Method | Path | Description |
|---|---|---|
| GET | /healthz | Liveness probe |
| GET | /metrics | Prometheus metrics |

### Auth (OIDC RP; the whole group returns 404 when HOARD_OIDC_ISSUER is empty)

| Method | Path | Description |
|---|---|---|
| GET | /auth/oidc/login | Start SSO; the ?next= return URL must match the CORS allowlist, to prevent open redirects |
| GET | /auth/oidc/callback | Authorization code callback, sets the session cookie |
| POST | /auth/logout | Revoke the session |
| GET | /auth/me | Current identity (sub, username, role) |

### User (session cookie or mhp_ PAT)

| Method | Path | Description |
|---|---|---|
| POST | /api/team | Create a team; the creator becomes owner |
| GET | /api/user/teams | My teams |
| POST | /api/team/{id}/members | Change an existing member's role or transfer ownership (owner or site admin); non-members get 409, use an invite instead; demoting the sole owner gets 409 |
| DELETE | /api/team/{id}/members/{sub} | Remove a member or leave; the sole owner leaving gets 409, transfer ownership first |
| POST | /api/team/{id}/invites | Invite a user (owner or site admin); already a member gets 409; valid for 7 days, re-inviting refreshes |
| GET | /api/team/{id}/invites | Pending invites of a team |
| DELETE | /api/team/{id}/invites/{sub} | Revoke an invite |
| GET | /api/user/invites | My pending invites |
| POST | /api/user/invites/{teamId}/accept | Accept an invite; writes the membership and consumes the invite |
| DELETE | /api/user/invites/{teamId} | Decline an invite |
| GET | /api/user/pats | My tokens, plaintext not included |
| POST | /api/user/pats | Create a token; plaintext returned once |
| DELETE | /api/user/pats/{id} | Revoke a token |
| POST | /v2/project | Create a project (multipart data and file, requires the UPLOAD scope) |
| POST | /v2/version | Publish a version; requires project team membership and the UPLOAD scope |
| DELETE | /v2/project/{id} | Delete own project |
| GET | /api/team/{id}/members | Team members, public read |

### Admin (login session with role=admin, or HOARD_ADMIN_TOKEN bearer)

Both credentials are equivalent, matching Mirror's admin console: day-to-day work uses SSO sessions (HOARD_ADMIN_SUBJECTS maps to admin), and the static token is only for bootstrap and rescue. The behavior is fail closed: with an empty token the admin endpoints are not open, and only admin sessions get through.

| Method | Path | Description |
|---|---|---|
| POST | /api/mods | Legacy admin upload (multipart) |
| PATCH | /api/mods/{id}/status | Review state transition |
| DELETE | /api/mods/{id} | Delete a project |

### Public reads (Modrinth-compatible and native)

| Method | Path | Description |
|---|---|---|
| GET | /api/mods | Project list and search (q, offset, limit, facets, loaders, game_versions, categories, project_type) |
| GET | /api/mods/facets | Available facet values (loaders, game_versions, categories) |
| GET | /api/mods/{id} | Project detail |
| GET | /api/mods/{id}/icon | Project icon |
| GET | /api/mods/{id}/download | Download the latest primary file |
| GET | /api/mods/{id}/version/{vid}/file/{fid} | Download a specific file |
| GET | /v2/search | Modrinth-compatible search |
| GET | /v2/project/{id} | Modrinth-compatible project |
| GET | /v2/project/{id}/icon | Icon |
| GET | /v2/project/{id}/version | Version list |
| GET | /v2/project/{id}/dependencies | Version dependencies |
| GET | /v2/version/{id} | Single version |
| GET | /v2/project/{id}/version/{vid}/mrpack | .mrpack generation |
| GET | /v2/tag/{loader,game_version,category} | Metadata lists |

The full contract is specs/hoard.openapi.yaml in [Mythicove/api](https://github.com/Mythicove/api), guarded by specdrift_test.go in this repository: every path and method in the spec must actually be registered.

## Run

```bash
cp .env.example .env   # adjust as needed; in-memory store when DATABASE_URL is unset (development)
go run ./cmd/server    # listens on :8081
```

When sharing one Postgres database with Mirror, schema isolation is mandatory, otherwise same-named tables (users, sessions, files) collide:

```
HOARD_DATABASE_URL=postgres://mirror:mirror@db:5432/mirror?sslmode=disable&options=-csearch_path%3Dhoard
```

The SSO issuer must be reachable from both the browser and the hoard container. When the two are on different networks (typically: the issuer is a public domain while containers use service names), use split horizons:

```
HOARD_OIDC_ISSUER=https://api.example.com      # must exactly match mirror's MIRROR_BASE_URL (iss validation)
HOARD_OIDC_INTERNAL_BASE_URL=http://mirror:8080 # only for in-container discovery and token endpoint access
```

## Development

```bash
go test ./...   # pgstore integration tests need HOARD_TEST_DATABASE_URL; spec drift tests need the api repo alongside (or MYTHICOVE_API_SPECS)
sqlc generate   # regenerate internal/pgstore/db (CI has a drift gate)
```

Order for adding a persisted entity: migrations/0000N_*.sql (goose, 5-digit sequence), sync the DDL into internal/pgstore/schema.sql, add named queries to internal/pgstore/query.sql, run sqlc generate, extend the Store interface in internal/hoard/store.go, then implement both pgstore.Store and hoard.MemoryStore (tests use the latter).

## License

[Elastic License 2.0 (ELv2)](./LICENSE)

The repository was released under AGPL-3.0 (v0.1.0 to v0.3.0) and moved to ELv2 on 2026-10-26. See [LICENSE.history](./LICENSE.history) for the history and what it means.
