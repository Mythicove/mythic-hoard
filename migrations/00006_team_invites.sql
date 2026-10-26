-- +goose Up
-- 团队邀请：加入团队需被邀请人同意，故引入待处理邀请表。
-- 行存在即 pending；接受则写入 team_members 并删除本行，拒绝/撤销同样删除。
-- invited_by 不设外键：管理员以静态 token 操作时无对应 users 行（存空串）。

CREATE TABLE team_invites (
    team_id    TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_sub   TEXT NOT NULL REFERENCES users(sub) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    invited_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_sub)
);
CREATE INDEX idx_team_invites_user ON team_invites (user_sub);

-- +goose Down
DROP TABLE team_invites;
