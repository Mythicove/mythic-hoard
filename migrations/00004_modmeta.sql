-- +goose Up
-- mod 元数据解析（W4）：license/icon 与表单冲突标旗。

ALTER TABLE projects ADD COLUMN license TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN icon_key TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN meta_flags TEXT[] NOT NULL DEFAULT '{}';
