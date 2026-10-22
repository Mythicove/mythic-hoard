# Mythic Hoard

开源 Minecraft Mod / 整合包分发后端，定位**私群分发**（统一鉴权下的皮肤站配套），提供 Modrinth 兼容读 + `.mrpack` 导出。

## 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/mods` | 上传（multipart：`file` + `title`/`description`/`project_type`/`version_number`/`loaders`/`game_versions`） |
| GET | `/api/mods` | 项目列表 |
| GET | `/api/mods/{id}` | 项目详情 |
| GET | `/api/mods/{id}/download` | 下载主文件 |
| GET | `/v2/search` | Modrinth 兼容搜索 |
| GET | `/v2/project/{id}` | Modrinth 兼容项目 |
| GET | `/v2/project/{id}/version` | Modrinth 兼容版本列表 |
| GET | `/v2/version/{id}` | Modrinth 兼容版本 |
| GET | `/v2/project/{id}/version/{vid}/mrpack` | `.mrpack` 导出（供 packwiz/启动器） |

## 运行

```bash
go run ./cmd/server   # 监听 :8081
```

当前为内存元数据 + 本地文件存储（MVP）；M2 换 Postgres + S3，并接入 `shared` 能力接口。
