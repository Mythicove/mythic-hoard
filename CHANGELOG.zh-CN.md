# Changelog

[English](./CHANGELOG.md) | **简体中文**

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与语义化版本。

## [0.3.0] - 2026-10-23

Phase 3（用户体系与开放上传）：普通用户无需管理员 token 即可发布 mod。

### 新增

- **OIDC RP SSO**（Mirror 为 IdP）：`internal/oidcrp`（go-oidc v3，discovery/JWKS 轮换/签名/iss/aud/exp/nonce 全校验）；`/auth/oidc/login|callback`、`/auth/logout`、`/auth/me`；state/nonce 短期 httpOnly cookie；会话 cookie `hoard_session`（httpOnly+Lax，7d，服务端可撤销）；用户投影表（migration 00002，重登不覆盖已提升 role）；`HOARD_ADMIN_SUBJECTS` 白名单幂等映射 admin
- **团队与 PAT**（migration 00003）：`teams`/`team_members`（owner/member 两级）；个人访问令牌 `mhp_` 前缀、sha256 哈希存储、scopes（声明后强制，上传须含 UPLOAD）、过期、last_used、可撤销；`projects.team_id` 可空兼容存量
- **用户级上传**：`POST /v2/project`（Modrinth 形 multipart，自动建团队归属）、`POST /v2/version`（团队成员校验）、`DELETE /v2/project/{id}`（owner/admin）；管理员 `/api/mods` 队列保留；`HOARD_REQUIRE_REVIEW` 审核开关
- **mod 元数据解析**（migration 00004）：`internal/modmeta` 读 jar 内 fabric/quilt/(neo)forge 元数据 → 自动填充 title/description/loaders/game_versions/license/version/icon；表单优先、冲突写 `metadata_flags`；icon 提取入对象存储 + `/api/mods/{id}/icon` 直出
- **版本依赖**（migration 00005）：required/optional/incompatible/embedded；`GET /v2/project/{id}/dependencies`；mrpack 合规增强（minecraft 依赖版本、env、overrides/）
- CORS 白名单模式允许凭证（SSO cookie）

### 修复

- Meili 同键多值 facet 过滤被 AND 的语义 bug（应 OR）；`total_hits` 用 estimatedTotalHits（shared/search v0.1.2）

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
