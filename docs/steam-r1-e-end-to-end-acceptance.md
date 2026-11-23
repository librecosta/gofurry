# STEAM-R1-E · 端到端验收

验收日期为 2026-11-23（Asia/Shanghai）。Steam 媒体采集、编辑与发布数据链路为 **PASS**。公开媒体体验为 **PARTIAL**，已发布的 HLS/DASH 影片在公开 API 和公开页均无播放入口。契约冻结与推广边界见 [封板报告](steam-r1-e-media-parity-closure.md)。

## E1 · 基线与环境

| 仓库 | 本地 `develop` | 远端 `develop` | 复核依据 |
| --- | --- | --- | --- |
| Harvest | `ead956e` | `ead956e` | [R0-B](https://github.com/librecosta/harvest/blob/ead956e/docs/steam-r0-b-acceptance.md) · [R1-A](https://github.com/librecosta/harvest/blob/ead956e/docs/steam-r1-a-acceptance.md) · [R1-B](https://github.com/librecosta/harvest/blob/ead956e/docs/steam-r1-b-acceptance.md) |
| GoFurry | `68057bb` | `68057bb` | [R1-C](steam-r1-c-acceptance.md) · [F1](steam-r1-c-f1-media-cursor-acceptance.md) |
| Admin | `46a2d77` | `46a2d77` | [R1-D](https://github.com/librecosta/admin/blob/46a2d77/docs/steam-r1-d-acceptance.md) |

Windows 原生 Go 1.27.4、Node 24.22.1、pnpm 12.8.1、PostgreSQL 18.6、Edge Playwright。复用已有环回 PostgreSQL 实例，为会写入的验证复制原夹具或使用专用测试库。原候选、Admin 账号和真实来源样本保留。GoFurry 和 Admin 使用显式 YAML；公开 Web 使用本地构建的 SSR 产物。私有输入与测试记录位于 GoFurry 忽略的 `.local/steam-r1-e`。

只读 Goose 检查覆盖配置中的共享开发库。Harvest `harvest_dev` 已应用 v13，GoFurry `gofurry_cn_dev` 已应用至 `202611230002`，Admin `librecosta_admin_dev` 已应用至 `202611050002`，均无待应用迁移。新开的隔离库使用既有显式迁移，应用启动不迁移。未修改共享开发业务数据或生产资源。

## E2 · 来源、编辑稿与正式快照

| 游戏 | AppDetails 真实样本 | StoreBrowse 真实样本 | GoFurry 隔离投影 |
| --- | --- | --- | --- |
| Paws · 434000 | `en/HK`，出口 `cn`，2 Movie、12 Screenshot | `en/US`，出口 `cn`，9 个资产 | Details `en/hk/cn`，Assets `en/cn/cn`。后者为早期夹具的重投影 |
| Stray · 1332010 | `en/CN`，出口 `cn`，3 Movie、8 Screenshot | `en/US`，出口 `cn`，15 个资产 | Details `en/cn/cn`，Assets `en/us/cn` |

- 在恢复的 R0-B PostgreSQL 中重算了 4 个 Evidence 的 SHA-256，4 个匹配，4 个 Attribution 可读。AppDetails 媒体与 R0-A 原始归档经 R1-A 规范化后的私有样本逐对象比较。StoreBrowse 样本与 R1-B PostgreSQL 两条 `present` Observation 的载荷完全一致，数据库保留相应 Attempt、实际出口及每个样本各一张已完成 Permit。
- 本次采集侧复验使用已保存样本和隔离上游夹具，新增 AppDetails/GetItems 调用为 **0**。真实 CDN 网络流量来自浏览器媒体验证。
- GoFurry 两个 AppID 的 Details 结构与真实样本一致，包含字符串 Movie ID、名称、封面、Highlight、原始 Order、独立 HLS/DASH 来源，以及 Screenshot ID、排序、缩略图和原图。管理候选扫描到的绑定、Target、GameSource 和 Observation 与本地 PostgreSQL 对应。隔离回放生成独立的 Target/Observation 身份。Paws Assets 的市场和各对象观察时间是既有 CN 夹具的重投影信息，资产身份、角色、Slot、URL 一致。Stray 的 Assets 载荷与真实 US 样本一致。
- 两款真实样本都有独立的 Library Capsule 和 Hero，均缺少 Library Logo。Logo 和 2x 变体的类型隔离由 Harvest 夹具验证；真实样本里的有效 2x 变体保持独立。
- GoFurry 原生监听器从环回 Harvest 协议夹具回放 Paws，保持完整载荷及原 Observation ID。独立数据库查询确认候选位置与 Consumer Cursor 已提交，随后远端才接受 ACK。真实载荷经同一 Consumer 契约送入隔离库，本次未把协议夹具的 ID 当作 Steam 抓取来源。
- Admin Embed 用 Operator 登录，中文编辑稿采用英文来源的 Library Capsule、Library Hero、8 张截图、2 条视频。逐对象比对候选、编辑稿与发布快照，包括完整 Origin、电影格式、截图两个 URL 与来源顺序。人为调序按数组位置保存，原始 Order 继续保留。刷新后选择一致，发布 Revision 和目标 Game 一致。
- 对原已发布的 Stray 实测 Publish Changes。GoFurry Consumer 隔离套件以新建 Paws 实测首次 Publish，再实测 Publish Changes。保存改变 Preview Token 和 Draft Revision，旧 Token 被拒，正式快照和公开 profile 在重新发布前逐字段不变。发布后 Public ID、Slug 和 SFW 边界保留。

## E3 · Media Studio 与权限

| 检查 | 结果 |
| --- | --- |
| 首屏、边界、跨页 | 小于 12 组与恰好 12 组均无后续游标。25 组 fixture 经真实 Admin、GoFurry 和 PostgreSQL 得到 12 + 12 + 1，页体与 GoFurry 直接响应一致。两次加载保留未保存选择，尾页不再显示加载按钮 |
| 候选与采用 | 跨来源、跨语言、缺少 Logo、来源失效后保留已采用素材，均复验。截图 8、视频 4 的限制、截断提示、共享 URL 的不同字符串 ID、清除与撤销、DraftGuard 和画面失败由真实流程及浏览器夹具覆盖。一个封面与同角色一张图由现有选择契约约束 |
| 兼容 | 原版本媒体 URL 和管理更新继续可用，Consumer 过期或新版写入不清空旧快照 |
| 权限与回执 | 未登录拒绝，Developer 可读、写入 403，Operator 通过 Session、Actor 和 CSRF 执行写入。Admin 账号枚举只有 Owner、Developer、Operator，Developer 是本表面的只读角色。Harvest Reader 的限制另由 Raw Evidence 原生套件验证 |
| 写入失败 | 缺少 CSRF 返回 403，旧 Revision 返回 409。原 Request ID 重发旧 Revision 返回 409，Revision 只加一次，同一 ID 的成功 receipt 仍可回读。GoFurry 停机时 Admin 读取 503，保存返回 503/`result_unknown`，请求和结果审计用同一 ID 关联 |

## E4 · 播放回归

| 样本与路径 | 打开到观测播放 | 媒体时间 |
| --- | ---: | ---: |
| Paws · Edge 原生 HLS | 8.58 s | 1.24 s |
| Stray · Edge 原生 HLS | 6.20 s | 1.22 s |
| Paws · Edge HLS.js/MSE | 3.21 s | 0.43 s |
| Stray · Edge HLS.js/MSE | 2.16 s | 0.76 s |

四次均观察到真实 Steam CDN Manifest 和 fMP4 响应，原生路径为 206，HLS.js 测得 200。另对 Paws 定向验证 MSE 清理。打开前无 HLS 分包或流请求，单次打开记录 1 个按需分包、3 个 Manifest 请求、2 个成功分片请求，响应允许 CORS。关闭后视频暂停、元素移除、媒体时间停止增长，唯一 MediaSource Object URL 被回收。

MP4、WebM 使用现有本机短片，经 Edge 实际解码及播放。Fixture 覆盖 CORS 拒绝、DASH-only、仅缩略图和失效来源的不可播放状态。这两个真实游戏只提供 HLS/DASH 样本。Firefox、Safari 和异地 CDN 未在本轮实测。

## E5 · Public API、Game Profile 与 SSR

| 边界 | 实测结论 |
| --- | --- |
| PostgreSQL 正式快照 | **PASS**。保留完整 Movie、所有实际存在的 Sources、图片角色与完整来源身份 |
| Admin | **PASS**。上述 HLS 样本实际播放，DASH-only 有明确状态 |
| Public API | **PARTIAL**。CN 的公开 profile 200，只返回已发布封面、截图、兼容媒体和其他公开资料。兼容投影只允许 MP4/WebM。对真实 HLS/DASH 样本无视频 URL，完整 Sources 和 Library 角色仍只在管理/快照侧，公开封面是 URL 投影 |
| Public Web | **PARTIAL**。关闭 Harvest 集成后，隔离 CN SSR 显示已发布封面及按编辑顺序排列的 8 张截图。画廊没有空视频位，没有发出 HLS/DASH 请求。模拟图片 CDN 失败时页面仍为 200，游戏详情可见。兼容直链 HTML5 Video 有现有浏览器夹具，真实 HLS/DASH 样本在公开端无法播放 |
| INTL | 独立 INTL 测试库及 API/SSR。英文 canonical 正常，缺中文翻译时回退英文且标记 `noindex, follow`。API 返回 INTL/SFW，CN 测试游戏和私有 Management 路由不可读。INTL Admin 账号和真实 INTL Steam 出口未做同等业务联调 |

渲染链路为 SSR → 本区域公开 API → PostgreSQL 正式快照。额外的本地 Store Facts 不改变已发布媒体权威。复现公开视频差距时，对采用了 HLS/DASH-only 视频的 Stray 比较 `GET /internal/v1/ops/games/{id}` 的 `published_media_v2` 和公开 `GET /api/v1/games/{id}?profile=1&locale=zh`，再打开公开页画廊。预期现状为快照包含 Movie 与 Manifest，公开 DTO 和画廊均无对应播放项。

## E6 · 恢复

- Harvest 原生 Worker/HTTP 夹具通过版本共存、Lease/Attempt、重试、Completion replay、证据暂存故障恢复和保留清理。403/500 或缺失/空资产有不同语义。过期证据与 Attribution 清理不影响当前 Observation 或 Change Event。Live Steam 上游超时未在本轮主动注入。
- GoFurry 隔离 PostgreSQL 重跑 Snapshot、增量、同 Observation 再确认、HTTP 410 reset/epoch、上游 503、跨市场空结果、未知 Schema、错误 AppID 和投影事务故障。失败时旧 Cursor 不前进、没有 ACK，恢复后重放继续。来源退役、撤回与快照相互独立，Consumer 未改写编辑稿。
- 在联调克隆中让 Stray 的全部当前候选离开有效结果，候选页为空，公开 Profile 与正式快照仍逐字段一致。Admin 在 GoFurry 暂时不可用时保留 `result_unknown`，通过回执/审计核对结果，浏览器不自动重试。

## E7 · 性能与存储

| 量项 | 结果与口径 |
| --- | --- |
| Harvest 完整 Segment | 既有 R1-A/R1-B 真实样本。Paws Details 6,029 B、Assets 3,539 B。Stray Details 5,593 B、Assets 7,022 B |
| GoFurry 来源载荷 | 隔离 PostgreSQL `octet_length(payload::text)` / `pg_column_size`。Paws Details 5,940 / 1,564 B、Assets 3,441 / 1,449 B。Stray Details 5,489 / 1,608 B、Assets 7,049 / 2,338 B |
| Admin Game / 候选 | 25 组压力夹具。Game 144,005 B，Windows 本机 HTTP 加读取中位数 10.28 ms（预热后 10 次）。三页 82,342 / 82,371 / 6,162 B，后两页浏览器观测等待 6.57 / 8.74 ms |
| GoFurry Management | 首屏、次页本机 HTTP 中位数 3.75 / 4.38 ms（预热后各 10 次）。Draft 写入及随后回读单次 4.79 / 8.01 ms |
| 正式快照 | Stray 中文媒体快照 15,276 B 逻辑 JSON / 2,757 B PostgreSQL 列大小，未含索引和其他表空间 |
| 按需播放 | HLS.js 独立分包约 331.47 kB，gzip 104.53 kB。单次预览请求由上面的 Paws 定向测试测得，网络时间另见 E4 |

本机 HTTP、数据库调用和真实 CDN 数据分开计量，样本规模用于验收，不能推导生产延迟。

## 原生检查与范围

- Harvest `task check` 及隔离 v13 的 `TestSteamVersionedOffersNative`、`TestSteamAssetsNative`、`TestSteamEvidenceNative` 均通过。
- GoFurry 原工作树的 `task tools` 报告被忽略的 `services/api/.local/ops-data-60/diagnose.go` 使用旧 JSON 导入。保留用户文件及既有修改，使用 `68057bb` 干净导出执行 `task check`，Go 检查、构建和测试通过，Web Playwright 88 通过、3 个环境相关用例跳过。隔离 Consumer/媒体与 CN/INTL Catalog 原生测试通过，私有 Go overlay 只替换测试的隔离数据库名称。F1 UUIDv7 行为由隔离 HTTP 与 Admin 浏览器再次验证。
- Admin 使用 `46a2d77` 干净导出运行 `task check:api`、`task check:web`、`task check:embed`。首轮 API 的静态测试发生一次 I/O 超时，重跑全部通过。专项 Playwright 包含真实保存发布、模拟故障、真实原生 HLS 和真实 HLS.js 四次运行；另有真实跨页与播放器清理探针。
- 本轮未运行生产部署、生产数据或生产账号验证。未启动新的批量采集。测试输入、私有配置、截图和日志保留在忽略目录。
