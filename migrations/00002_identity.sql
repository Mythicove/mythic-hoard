-- +goose Up
-- 身份投影：OIDC 用户投影表 + web 会话表（见 docs/plans/status.md 身份架构决策）。
-- sub = Mirror users.id（OIDC sub），不建跨服务外键。

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
