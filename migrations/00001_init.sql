-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE projects (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    slug          TEXT NOT NULL UNIQUE,
    title         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    body          TEXT NOT NULL DEFAULT '',
    project_type  TEXT NOT NULL DEFAULT 'mod',
    client_side   TEXT NOT NULL DEFAULT 'optional',
    server_side   TEXT NOT NULL DEFAULT 'optional',
    status        TEXT NOT NULL DEFAULT 'listed',
    downloads     BIGINT NOT NULL DEFAULT 0,
    follows       BIGINT NOT NULL DEFAULT 0,
    loaders       TEXT[] NOT NULL DEFAULT '{}',
    game_versions TEXT[] NOT NULL DEFAULT '{}',
    categories    TEXT[] NOT NULL DEFAULT '{}',
    published_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE versions (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL DEFAULT '',
    version_number TEXT NOT NULL,
    version_type   TEXT NOT NULL DEFAULT 'release',
    changelog      TEXT NOT NULL DEFAULT '',
    loaders        TEXT[] NOT NULL DEFAULT '{}',
    game_versions  TEXT[] NOT NULL DEFAULT '{}',
    featured       BOOLEAN NOT NULL DEFAULT false,
    date_published TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE files (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    version_id  TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    size        BIGINT NOT NULL DEFAULT 0,
    sha1        TEXT NOT NULL DEFAULT '',
    sha512      TEXT NOT NULL DEFAULT '',
    storage_key TEXT NOT NULL,
    is_primary  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE loaders (
    name TEXT PRIMARY KEY
);

CREATE TABLE game_versions (
    name TEXT PRIMARY KEY
);

CREATE TABLE categories (
    name TEXT PRIMARY KEY
);

CREATE INDEX projects_title_trgm ON projects USING gin (title gin_trgm_ops);
CREATE INDEX projects_description_trgm ON projects USING gin (description gin_trgm_ops);
CREATE INDEX versions_project_id_idx ON versions (project_id);
CREATE INDEX files_version_id_idx ON files (version_id);

-- +goose Down
DROP TABLE categories;
DROP TABLE game_versions;
DROP TABLE loaders;
DROP TABLE files;
DROP TABLE versions;
DROP TABLE projects;
