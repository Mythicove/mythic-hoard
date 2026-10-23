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
    team_id       TEXT,
    license       TEXT NOT NULL DEFAULT '',
    icon_key      TEXT NOT NULL DEFAULT '',
    meta_flags    TEXT[] NOT NULL DEFAULT '{}',
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

CREATE TABLE users (
    sub        TEXT PRIMARY KEY,
    username   TEXT NOT NULL,
    email      TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    role       TEXT NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_sub   TEXT NOT NULL REFERENCES users(sub) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_user ON sessions (user_sub);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);

CREATE TABLE teams (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
    team_id    TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_sub   TEXT NOT NULL REFERENCES users(sub) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_sub)
);

CREATE TABLE personal_access_tokens (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_sub   TEXT NOT NULL REFERENCES users(sub) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    scopes     TEXT[] NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    last_used  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pat_user ON personal_access_tokens (user_sub);
