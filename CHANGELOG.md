# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与语义化版本。

## [0.2.0] - 2026-10-23

Phase 2（生产化与运维基线）。

### 新增

- 限流新增 Redis 实现（`internal/redislimit`，Lua 原子固定窗口，多副本共享计数，fail-open 回退进程内）
- `/metrics` Prometheus 端点（HTTP 时延/计数 + Go runtime）
- 单文件大小上限 `HOARD_MAX_FILE_BYTES`（默认 512MiB，超限 413）
- goose 迁移 advisory-lock 串行化（多副本并发启动安全）

## [0.1.0] - 2026-10-23

首个可用版本：私群 mod 分发 MVP。

### 新增

- Project → Version → File 三级模型（Postgres/sqlc 或内存存储），本地磁盘或 S3
- Modrinth 兼容读子集：`/v2/search`（q + facets/loaders/game_versions/project_type/categories）、`/v2/project/{id}`、`/v2/project/{id}/version`、`/v2/version/{id}`、`/v2/tag/*`
- `.mrpack` 生成：含 `dependencies`（loader 映射）与 `mods/` 路径前缀
- 上传/审核：multipart 上传（SHA-1/SHA-512 哈希）、审核状态流转，`HOARD_ADMIN_TOKEN` 鉴权
- 搜索双实现（pg_trgm / Meilisearch）、队列双实现（inline / asynq）、下载计数
- 安全：可信代理感知 IP 解析（`shared/clientip`）、按 IP 固定窗口限流（TTL 清扫）、安全响应头、`ReadHeaderTimeout`/`IdleTimeout`、CORS 白名单（`HOARD_CORS_ORIGINS`，修复漏 PATCH）
- 工程化：sqlc + CI 漂移检查、契约漂移门、Postgres 集成测试、gofmt 门、govulncheck

### 已知限制

- **无用户体系**：上传/审核仅靠管理员 token；用户/团队/API Token 在 Phase 3（经 Mirror OIDC）
- 无版本依赖关系、不解析 mod 元数据、无上游镜像代理（Phase 3）
