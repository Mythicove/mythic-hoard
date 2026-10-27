-- +goose Up
-- 邀请过期：此前 team_invites 的行常驻直到被接受/拒绝/撤回，
-- 被邀请人永远看不到"这个邀请还有效吗"，管理者也无从回收陈旧邀请。
-- 过期时间由服务端计算后写入（单一 TTL 定义在应用层，不在 SQL 里再写一份默认值）。
ALTER TABLE team_invites ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '7 days');
CREATE INDEX idx_team_invites_expires ON team_invites (expires_at);

-- +goose Down
DROP INDEX idx_team_invites_expires;
ALTER TABLE team_invites DROP COLUMN expires_at;
