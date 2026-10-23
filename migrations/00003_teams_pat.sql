-- +goose Up
-- 团队与个人访问令牌（Phase 3 W2）。
-- 权限模型：owner（管成员/删项目）+ member（传版本）两级，不做细粒度位域。

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

ALTER TABLE projects ADD COLUMN team_id TEXT REFERENCES teams(id) ON DELETE SET NULL;
CREATE INDEX idx_projects_team ON projects (team_id);
