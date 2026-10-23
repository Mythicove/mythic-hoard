-- +goose Up
-- 版本依赖关系（Modrinth 形：required/optional/incompatible/embedded）。

CREATE TABLE version_dependencies (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    version_id      TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
    dependency_type TEXT NOT NULL,
    project_id      TEXT NOT NULL DEFAULT '',
    dep_version_id  TEXT NOT NULL DEFAULT '',
    file_name       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_vdeps_version ON version_dependencies (version_id);
