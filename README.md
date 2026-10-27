# Mythic Hoard

开源 Minecraft Mod / 整合包分发后端，定位**私群分发**（与统一账号体系的皮肤站 Mirror 配套），提供 Modrinth 兼容读 API + `.mrpack` 生成。

## 状态

用户体系已接通（v0.3.0）：

- **Project → Version → File 三级模型**：Postgres（sqlc）或内存存储；本地磁盘或 S3 兼容对象存储
- **SSO 登录**：Mirror 作 OIDC IdP（授权码流），会话存 httpOnly cookie `hoard_session`（7 天、服务端可撤销）；`HOARD_ADMIN_SUBJECTS` 白名单命中者登录即 `role=admin`
- **团队 + PAT**：团队创建/成员增删/成员列表；`mhp_` 前缀个人访问令牌（sha256 存储、scopes、有效期、last_used），上传要求 `UPLOAD` scope
- **用户级上传**：`POST /v2/project`、`POST /v2/version`（multipart，jar 元数据自动解析）、`DELETE /v2/project/{id}`
- **Mod 元数据解析**（`internal/modmeta`）：fabric.mod.json / quilt.mod.json / mods.toml / neoforge.mods.toml，自动填充 name/id/version/依赖/图标抽取与冲突标记
- **版本依赖关系**（Modrinth 形状）+ `.mrpack` 合规（`minecraft` 依赖、env、overrides）
- **Modrinth 兼容读**（子集）：`/v2/search`（含 facets，内层 OR / 外层 AND）、项目/版本/依赖查询、tag 列表、图标
- **搜索双实现**：`pg_trgm`（默认，含精确总数）/ Meilisearch（首次访问自动开通索引：filterableAttributes + `maxTotalHits=1e6` 使总数精确）；队列 inline / asynq
- **管理侧**：管理员 token 的 legacy 上传、审核状态流转（`listed|under_review|rejected|archived`）、删除；`HOARD_REQUIRE_REVIEW` 可强制新上传进审核

未做：上游镜像代理、画廊/截图。邀请有效期 7 天（`InviteTTL`，重新邀请刷新；过期邀请在所有读取路径上视同不存在）。内容举报与皮肤审核队列在 **Mirror**（`POST /reports`、`/api/admin/reports*`、`/api/admin/skins*`），不在本仓库。完整差距见 `docs/plans/status.md`。

## 端点

### 基础设施

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 存活探针 |
| GET | `/metrics` | Prometheus 指标 |

### 认证（OIDC RP；`HOARD_OIDC_ISSUER` 为空则整组 404）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/auth/oidc/login` | 发起 SSO；`?next=` 回跳地址须命中 CORS 白名单（防开放重定向） |
| GET | `/auth/oidc/callback` | 授权码回调，落会话 cookie |
| POST | `/auth/logout` | 撤销会话 |
| GET | `/auth/me` | 当前身份（`sub`/`username`/`role`） |

### 用户（会话 cookie 或 `mhp_` PAT）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/team` | 创建团队（创建者即 owner） |
| GET | `/api/user/teams` | 我的团队 |
| POST | `/api/team/{id}/members` | 变更**已有成员**角色 / 移交所有权（owner 或站点管理员）；非成员 → 409（改用邀请）；唯一 owner 降级 → 409 |
| DELETE | `/api/team/{id}/members/{sub}` | 移除成员或自己退出；唯一 owner 退出 → 409（须先移交） |
| POST | `/api/team/{id}/invites` | 邀请用户加入（owner 或站点管理员）；已是成员 → 409；有效期 7 天，重复邀请刷新 |
| GET | `/api/team/{id}/invites` | 团队的待处理邀请 |
| DELETE | `/api/team/{id}/invites/{sub}` | 撤回邀请 |
| GET | `/api/user/invites` | 我收到的待处理邀请 |
| POST | `/api/user/invites/{teamId}/accept` | 接受邀请（写入成员并消费邀请） |
| DELETE | `/api/user/invites/{teamId}` | 拒绝邀请 |
| GET | `/api/user/pats` | 我的令牌（不含明文） |
| POST | `/api/user/pats` | 创建令牌（明文只返回一次） |
| DELETE | `/api/user/pats/{id}` | 撤销令牌 |
| POST | `/v2/project` | 创建项目（multipart `data` + `file`，需 `UPLOAD` scope） |
| POST | `/v2/version` | 发布版本（需项目团队成员身份 + `UPLOAD` scope） |
| DELETE | `/v2/project/{id}` | 删除自己的项目 |
| GET | `/api/team/{id}/members` | 团队成员（公开读） |

### 管理（`role=admin` 的登录会话 **或** `HOARD_ADMIN_TOKEN` Bearer）

两种凭据等价，与 Mirror 的后台一致：日常用 SSO 登录态（`HOARD_ADMIN_SUBJECTS` 映射为 admin），静态令牌只作 bootstrap/救援。**fail closed**：令牌留空不再等于开放管理端点，此时只有 admin 会话能通过。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/mods` | legacy 管理上传（multipart） |
| PATCH | `/api/mods/{id}/status` | 审核状态流转 |
| DELETE | `/api/mods/{id}` | 删除项目 |

### 公开读（Modrinth 兼容 + 本站）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/mods` | 项目列表/搜索（`q`/`offset`/`limit`/`facets`/`loaders`/`game_versions`/`categories`/`project_type`） |
| GET | `/api/mods/facets` | 可用 facet 值（loaders / game_versions / categories） |
| GET | `/api/mods/{id}` | 项目详情 |
| GET | `/api/mods/{id}/icon` | 项目图标 |
| GET | `/api/mods/{id}/download` | 下载最新主文件 |
| GET | `/api/mods/{id}/version/{vid}/file/{fid}` | 下载指定文件 |
| GET | `/v2/search` | Modrinth 兼容搜索 |
| GET | `/v2/project/{id}` | Modrinth 兼容项目 |
| GET | `/v2/project/{id}/icon` | 图标 |
| GET | `/v2/project/{id}/version` | 版本列表 |
| GET | `/v2/project/{id}/dependencies` | 版本依赖关系 |
| GET | `/v2/version/{id}` | 单个版本 |
| GET | `/v2/project/{id}/version/{vid}/mrpack` | `.mrpack` 生成 |
| GET | `/v2/tag/{loader,game_version,category}` | 元数据列表 |

完整契约见 [Mythicove/api](https://github.com/Mythicove/api) `specs/hoard.openapi.yaml`（受本仓库 `specdrift_test.go` 漂移测试保护：spec 里的每个 path+method 都必须真实注册）。

## 运行

```bash
cp .env.example .env   # 按需改；不配 DATABASE_URL 时用内存存储（开发）
go run ./cmd/server    # 监听 :8081
```

与 Mirror 共用同一个 Postgres 库时，**必须**做 schema 隔离，否则 `users`/`sessions`/`files` 等同名表会碰撞：

```
HOARD_DATABASE_URL=postgres://mirror:mirror@db:5432/mirror?sslmode=disable&options=-csearch_path%3Dhoard
```

SSO 的 issuer 需对**浏览器**与**hoard 容器**同时可达。二者不同网时（典型：issuer 是对外域名，容器内走服务名）用分离视界：

```
HOARD_OIDC_ISSUER=https://api.example.com      # 必须与 mirror 的 MIRROR_BASE_URL 完全一致（iss 校验）
HOARD_OIDC_INTERNAL_BASE_URL=http://mirror:8080 # 仅用于容器内访问发现/令牌端点
```

## 开发

```bash
go test ./...   # pgstore 集成测试需 HOARD_TEST_DATABASE_URL；spec 漂移测试需同级 api 仓库（或 MYTHICOVE_API_SPECS）
sqlc generate   # 重新生成 internal/pgstore/db（CI 有漂移门禁）
```

新增持久化实体的顺序：`migrations/0000N_*.sql`（goose，5 位序号）→ 同步 DDL 到 `internal/pgstore/schema.sql` → `internal/pgstore/query.sql` 加命名查询 → `sqlc generate` → 扩 `internal/hoard/store.go` 的 `Store` 接口 → 同时实现 `pgstore.Store` 与 `hoard.MemoryStore`（测试用后者）。

## 许可证

[Elastic License 2.0 (ELv2)](./LICENSE)
