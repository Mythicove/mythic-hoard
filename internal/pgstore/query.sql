-- name: CreateProject :one
INSERT INTO projects (slug, title, description, body, project_type, client_side, server_side, status, loaders, game_versions, categories, published_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
RETURNING id, slug, title, description, body, project_type, client_side, server_side, status, downloads, follows, loaders, game_versions, categories, published_at;

-- name: GetProjectByIDOrSlug :one
SELECT id, slug, title, description, body, project_type, client_side, server_side, status, downloads, follows, loaders, game_versions, categories, published_at
FROM projects WHERE id = $1 OR slug = $1 LIMIT 1;

-- name: GetProjectsByIDs :many
SELECT id, slug, title, description, body, project_type, client_side, server_side, status, downloads, follows, loaders, game_versions, categories, published_at
FROM projects WHERE id = ANY(@ids::text[]);

-- name: SearchProjects :many
SELECT id, slug, title, description, body, project_type, client_side, server_side, status, downloads, follows, loaders, game_versions, categories, published_at
FROM projects
WHERE ($1::text = '' OR title ILIKE '%' || $1::text || '%' OR description ILIKE '%' || $1::text || '%')
  AND (cardinality($2::text[]) = 0 OR loaders && $2::text[])
  AND (cardinality($3::text[]) = 0 OR game_versions && $3::text[])
  AND (cardinality($4::text[]) = 0 OR project_type = ANY($4::text[]))
  AND (cardinality($5::text[]) = 0 OR categories && $5::text[])
  AND ($6::text = '' OR status = $6::text)
ORDER BY downloads DESC, created_at DESC
LIMIT $7 OFFSET $8;

-- name: CountSearchProjects :one
SELECT count(*) FROM projects
WHERE ($1::text = '' OR title ILIKE '%' || $1::text || '%' OR description ILIKE '%' || $1::text || '%')
  AND (cardinality($2::text[]) = 0 OR loaders && $2::text[])
  AND (cardinality($3::text[]) = 0 OR game_versions && $3::text[])
  AND (cardinality($4::text[]) = 0 OR project_type = ANY($4::text[]))
  AND (cardinality($5::text[]) = 0 OR categories && $5::text[])
  AND ($6::text = '' OR status = $6::text);

-- name: UpdateProjectStatus :exec
UPDATE projects SET status = $2 WHERE id = $1;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = $1;

-- name: IncrementDownloads :exec
UPDATE projects SET downloads = downloads + 1 WHERE id = $1;

-- name: CreateVersion :one
INSERT INTO versions (project_id, name, version_number, version_type, changelog, loaders, game_versions)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, project_id, name, version_number, version_type, changelog, loaders, game_versions, date_published;

-- name: ListVersions :many
SELECT id, project_id, name, version_number, version_type, changelog, loaders, game_versions, date_published
FROM versions WHERE project_id = $1 ORDER BY date_published DESC;

-- name: GetVersion :one
SELECT id, project_id, name, version_number, version_type, changelog, loaders, game_versions, date_published
FROM versions WHERE id = $1;

-- name: CreateFile :one
INSERT INTO files (version_id, filename, size, sha1, sha512, storage_key, is_primary)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, version_id, filename, size, sha1, sha512, storage_key, is_primary;

-- name: ListFiles :many
SELECT id, version_id, filename, size, sha1, sha512, storage_key, is_primary
FROM files WHERE version_id = $1 ORDER BY created_at;

-- name: ListLoaders :many
SELECT name FROM loaders ORDER BY name;

-- name: ListGameVersions :many
SELECT name FROM game_versions ORDER BY name;

-- name: ListCategories :many
SELECT name FROM categories ORDER BY name;

-- name: UpsertLoader :exec
INSERT INTO loaders (name) VALUES ($1) ON CONFLICT (name) DO NOTHING;

-- name: UpsertGameVersion :exec
INSERT INTO game_versions (name) VALUES ($1) ON CONFLICT (name) DO NOTHING;

-- name: UpsertCategory :exec
INSERT INTO categories (name) VALUES ($1) ON CONFLICT (name) DO NOTHING;

-- name: UpsertUser :exec
-- OIDC 登录投影 upsert（role 保留现值，不被登录覆盖；管理员映射由服务层处理）。
INSERT INTO users (sub, username, email, avatar_url, role)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (sub) DO UPDATE
SET username = EXCLUDED.username, email = EXCLUDED.email,
    avatar_url = EXCLUDED.avatar_url, last_login = now();

-- name: GetUserBySub :one
SELECT sub, username, email, avatar_url, role, created_at, last_login FROM users WHERE sub = $1;

-- name: SetUserRole :execrows
UPDATE users SET role = $2 WHERE sub = $1;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_sub, expires_at) VALUES ($1, $2, $3);

-- name: GetSession :one
-- 返回会话及其用户投影；过期会话视为不存在。
SELECT s.id, s.user_sub, s.expires_at, u.username, u.email, u.avatar_url, u.role
FROM sessions s JOIN users u ON u.sub = s.user_sub
WHERE s.id = $1 AND s.expires_at > now();

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
