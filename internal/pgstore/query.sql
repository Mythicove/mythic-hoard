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
