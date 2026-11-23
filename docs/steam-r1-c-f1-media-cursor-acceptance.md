# STEAM-R1-C-F1 · Media Candidate UUIDv7 Cursor 验收

验收日期为 2026-11-23。GoFurry 基线为 `9d9c4f7`，联调用的 Admin 版本为 `46a2d77`。

## 修复范围

`GET /internal/v1/ops/games/{public_id}/media-candidates` 原先在解析 UUIDv7 前调用通用 `search()`。该函数要求 `after` 为 16 位 Public ID，因此服务端会以 HTTP 400 拒绝自己返回的媒体候选游标。

- `services/api/internal/management/game_media.go` 独立校验查询参数。无 `after` 读取首屏。继续分页时，仅允许一个 `after`，其值必须为小写带连字符、RFC variant 的规范 UUIDv7。空值、UUIDv4、其他 UUID 表示、Public ID、重复或未知参数、畸形编码及超过 1024 字节的原始查询均返回 HTTP 400。
- `services/api/internal/management/game_media_test.go` 新增 Management Handler 查询边界测试及隔离 PostgreSQL 分页测试。
- 普通目录继续通过 `search()` 校验 Public ID。媒体候选继续使用现有 SQL，以当前 Game 内的候选行 UUID 严格递增分页，每页最多 12 组。沿用现有身份校验、来源绑定及授权。

## 原生 HTTP 和 PostgreSQL

在 Windows 原生 Go 1.27.4 与 PostgreSQL 18.6 上，将原 R1-C 测试库克隆到环回地址上的 `gofurry_r1c_f1_test`。在已有 Stray AppDetails v2 和 StoreBrowse Assets 各一组的基础上，准备了 23 组带独立候选与 Observation ID 的测试来源。媒体对象复用 R1-C 已保存样本。保留 Paws 作为跨游戏隔离对照。

| 查询 | HTTP | 候选数 | `next` |
| --- | ---: | ---: | --- |
| 无游标 | 200 | 12 | 第 12 组的候选行 UUIDv7 |
| 使用第一页的游标 | 200 | 12 | 第 24 组的候选行 UUIDv7 |
| 使用第二页的游标 | 200 | 1 | 省略 |

- Handler 返回的 25 个 Observation ID 与独立数据库查询的顺序逐项一致，无重复或遗漏。对另一游戏使用该游标时，响应未混入 Stray 的候选。
- 无授权返回 HTTP 401。非法 Game Public ID 和非法查询返回 HTTP 400。
- Sites、Games 和 Collections 目录均使用合法 Public ID 返回预期的下一条记录，且继续拒绝 UUIDv7。
- 真实 HTTP 和 Admin 浏览器验收前后，`games`、`collection_current`、`game_source_media`、`game_source_metadata`、`game_draft_media`、`game_draft_media_v2`、`game_published_localizations` 和 `management_operations` 的数据指纹一致。
- 本轮未执行数据库迁移。共享 CN 开发库仅核对 Goose 状态，最新迁移为 `202611230002`，原 R1-C 测试库也仅作为克隆来源。

## Admin 网关与浏览器

原生 GoFurry API 与现有 Admin Embed 通过环回地址连接，使用 R1-D 的隔离配置。Edge 中使用 Operator 登录 Stray 编辑页，从首屏 12 组候选连续加载至 24、25 组，随后加载按钮消失。Admin 网关三页均返回 HTTP 200，游标取自上一页，来源身份、媒体对象与顺序均和 GoFurry Management 直接响应一致。

加载两页前，在浏览器中临时移除一条已选视频。翻页保留了 8 张截图和剩余视频的身份、顺序及未保存状态。恢复操作后仍为 8 张截图、2 条视频，重新读取的已保存选择与操作前一致。

Developer 可以读取后续页，写入返回 HTTP 403。Operator 缺少 CSRF 时返回 403，携带有效 CSRF 的过时 Revision 返回 409。该 409 对应 Admin 审计的 requested 与 failure 两条记录，由同一 Request ID 关联。两个原生服务均正常停止。

## 原生检查

隔离库专项测试通过。复验命令中的 YAML 使用上文的环回数据库与 Management 配置。

```powershell
go -C services/api test ./internal/management -run '^TestMediaCandidate' -count=1 -args -media-cursor-config <isolated-yaml>
```

原工作目录中，已忽略的旧诊断脚本 `services/api/.local/ops-data-60/diagnose.go` 使用 `encoding/json`，触发仓库检查。保留该脚本及六个既有未暂存文件，在仅含本次拟提交代码的干净导出中运行原生 `task check`，成功退出。Go 检查、测试和构建通过，Web Playwright 88 项通过、3 项按套件配置跳过。

## 验收范围

25 组的分页压力场景来自隔离库夹具，新增来源范围为合成数据。沿用已保存 Steam 样本，未发起新的 Steam 请求。
