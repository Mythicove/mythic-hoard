# Mythic Hoard

开源 Minecraft Mod / 整合包分发后端，定位**私群分发**（与统一账号体系的皮肤站配套），提供 Modrinth 兼容读 API + `.mrpack` 生成。

## 状态

MVP 可用（M1–M3 + M5 部分）：

- **Project → Version → File 三级模型**：Postgres（sqlc）或内存存储；本地磁盘或 S3
- **Modrinth 兼容读**（已实现子集）：`/v2/search`（含 facets）、项目/版本查询、tag 列表
- **`.mrpack` 生成**：含 `dependencies`（loader 映射）与 `mods/` 路径
- **搜索双实现**：`pg_trgm`（默认）/ Meilisearch；队列 inline / asynq
- **上传/审核**：multipart 上传、SHA-1/SHA-512 哈希、审核状态流转（管理员 token 鉴权）

未做（对齐 Modrinth 的关键差距）：**用户/组织/团队 + API Token**（当前只有管理员 token，无用户级上传）、版本依赖关系、上传元数据解析（fabric.mod.json/mods.toml）、上游镜像代理、画廊/截图。完整差距见 `docs/plans/status.md`。

## 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/mods` | 上传（multipart，需 `HOARD_ADMIN_TOKEN`） |
| PATCH | `/api/mods/{id}/status` | 审核状态（需管理员 token） |
| DELETE | `/api/mods/{id}` | 删除项目（需管理员 token） |
| GET | `/api/mods` | 项目列表/搜索 |
| GET | `/api/mods/{id}` | 项目详情 |
| GET | `/api/mods/{id}/download` | 下载最新主文件 |
| GET | `/api/mods/{id}/version/{vid}/file/{fid}` | 下载指定文件 |
| GET | `/v2/search` | Modrinth 兼容搜索（q/facets/loaders/game_versions/limit/offset） |
| GET | `/v2/project/{id}` | Modrinth 兼容项目 |
| GET | `/v2/project/{id}/version` | 版本列表 |
| GET | `/v2/version/{id}` | 单个版本 |
| GET | `/v2/project/{id}/version/{vid}/mrpack` | `.mrpack` 生成 |
| GET | `/v2/tag/{loader,game_version,category}` | 元数据列表 |

完整契约见 [Mythicove/api](https://github.com/Mythicove/api) `specs/hoard.openapi.yaml`（受本仓库漂移测试保护）。

## 运行

```bash
cp .env.example .env   # 按需改；不配 DATABASE_URL 时用内存存储（开发）
go run ./cmd/server    # 监听 :8081
```

## 开发

```bash
go test ./...   # pgstore 集成测试需 HOARD_TEST_DATABASE_URL；spec 漂移测试需同级 api 仓库
sqlc generate   # 重新生成 internal/pgstore/db
```

## 许可证

[AGPL-3.0](./LICENSE)
