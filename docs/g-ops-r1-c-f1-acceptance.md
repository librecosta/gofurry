# G-OPS-R1-C-F1 验收记录

2026-11-21，Windows PowerShell、Go 1.27.4、Node 24.22.1、pnpm 12.8.1。使用现有私有 YAML、VPN PostgreSQL 18.6、Redis 和正在运行的本地服务。**Docker 使用次数为 0。**

## 双语言采集计划

首次 Intake 继续用 `en/us` 探测身份，沿用原请求、来源范围和幂等键。Steam 来源确认后，在同一个事务中为同一绑定确保两条 `steam.details` 计划。

| Scope | Steam Language | Store Country | 周期 |
| --- | --- | --- | --- |
| `en/us` | `english` | `US` | 24 小时 |
| `zh-cn/cn` | `schinese` | `CN` | 24 小时 |

两条计划共用原 EgressRegion、调度锚点、Steam 请求预算和 Worker 限流。已有同范围计划保留 ID 和周期，重复调用复用同一 Steam 来源和绑定。HK 英文无需默认调度。

## BROK 真实开发库与采集

写入前核对了 `gofurry_cn_dev`、`harvest_dev` 及现有开发库备份，另为 BROK 留存了只读前后快照。通过 GoFurry 既有 `collection plan` 命令限定到 BROK（Steam `949480`，Public ID `j2byfcwgr8cqnkt4`）的现有 Binding。相同提交两次返回相同的中文 Plan ID。原有英文 Schedule 保持启用，其他暂停的 Schedule 状态核对一致。

| Scope | 补齐前 | 补齐后 Plan ID | Harvest Schedule ID |
| --- | --- | --- | --- |
| `en/us` | 一条，已启用 | `01a1f44a-c947-7ff0-a48e-2c50e4c08dae` | `01a1f44a-d1c8-7d51-9728-d0587e4fd10d` |
| `zh-cn/cn` | 缺失 | `01a1fdef-59f2-7135-9ca5-6cc6f91faa52` | `01a1fdef-5f92-717c-9646-2036718e43aa` |

- 首次调度 Job `01a1fdef-6060-7e5a-86ab-16808877d896` 经 3 次 Worker 尝试后失败，记录 `timeout` 与 `transport_error`。失败结果为 `unavailable`。
- 经维护者确认，对同一个中文 Schedule 执行一次 Run Now。Job `01a1fdf7-3bbe-7a43-998d-93f635a45e18` 第一次尝试成功。合计两条 Job、4 次 Worker 采集尝试。通过保留的出网许可和结果确认了 **1 次成功的真实 Steam Storefront HTTP 请求**。之前 3 次失败尝试是否均已把 HTTP 请求送达 Steam 无法确认。没有额外发起诊断性 Steam HTTP 请求。
- 2026-11-21 19:56:43（北京时间）的真实简体中文结果由 GoFurry Consumer 投影。文案 Observation `01a1fdf7-472d-7935-8fbf-4beee0fe8265` 保存于 `zh-cn/cn`，`localized=true`。媒体 Observation `01a1fdf7-4714-7694-a38c-67e7f19c2856` 含 Header、Capsule 和 8 张截图。价格 Observation `01a1fdf7-46dd-795d-aea7-a40ba064e52b` 也由同一次采集生成。
- Admin 登录后只读核对。英文展示原 US 来源的 Header、Capsule 与 8 张截图候选。中文在首次失败后显示空状态，成功后显示真实 CN 来源文案和等量媒体候选，并观察到缩略图加载。两种语言分别保留实际 Observation 归属。本次已有真实中文文案与媒体，因此无需增加英文素材复用。
- 前后快照确认 Game UUID `01a1f44a-c88e-76a2-92b5-a7a3a81304c0`、Public ID、Steam GameSource、Intake Candidate、Binding 和 Harvest Target 原值一致。原有 4 条 Job、8 次 Attempt、18 条 Observation 和英文 Schedule 原值一致。游戏仍是 Draft、`unreviewed`、Revision 2，两份人工文案、媒体选择和发布快照未变。保留原有英文文案 Observation `01a1f44a-c716-7480-8efb-abc753982cbe` 和媒体 Observation `01a1f44a-c6f9-769b-a266-0a0737587be5`。

## 原生验收

| 项目 | 实际结果 |
| --- | --- |
| GoFurry API | `go vet ./...`、`go test ./...` 和原生编译通过 |
| GoFurry PostgreSQL | `TestIntakePlansPostgres` 覆盖首次创建、单边计划补齐、重放、原周期保留及第二条计划失败时回滚。 `TestSteamProjectionPostgres` 覆盖三种发布状态、中文媒体投影、空采集结果和人工编辑数据不变。现有 `TestSteamMetadataNormalization` 验证英文回退不会被标记为中文文案。测试 SQL 写入仅限事务内临时表并回滚 |
| GoFurry Web | TypeScript 和原生 SSR 构建通过。实际 Vite `/games` 返回 200 |
| Harvest | 原生 `task check` 通过，使用单独编译输出。实际 Server `/readyz` 返回 200，Worker 完成 BROK 中文任务 |
| Admin | `task check:gofurry` 和 Web TypeScript 通过。API `/readyz`、Web 游戏列表和 BROK 编辑页返回 200。浏览器已登录核对中英文来源 |
| 开发库状态 | GoFurry Goose `202611200002`、Harvest Goose `7`、Admin Goose `202611050002`。相关 Migration 均已应用。本轮无新 Migration |
| 运行中的 GoFurry | `/readyz` 返回 200，现有 Consumer 完成真实中文投影。保留维护者已启动的进程。新的首次 Intake 计划逻辑已编译并通过原生数据库测试，将随下一次正常启动载入 |

本轮未创建新的真实游戏。已有游戏通过 BROK 验证正式补齐路径，新 Intake 的建计划路径使用原生数据库测试覆盖。未运行 Docker 专用套件。人工审核发布由维护者继续执行，本轮未保存或发布任何编辑稿。

## 仓库规范与提交

七个仓库的根 `AGENTS.md` 已统一采用 Windows 原生开发、显式 YAML、显式 Migration、保护开发数据及分开报告测试、数据库与实际 HTTP 的规则。GoFurry 默认验证改用原生 SSR 构建。GoFurry、Harvest、Admin、AI Gateway 和 Identity 的容器任务已显式标注，保留既有自动化入口与部署能力。Notification 的 Taskfile 已是原生，Billing 当前无 Taskfile。

| 仓库 | `develop` 提交 |
| --- | --- |
| GoFurry 实现和测试 | `6744bcce6724fe565a1fb736ed5018a7774f2675` |
| GoFurry 开发规范 | `b27ad6ec7fc5a08aad5d730bb5f7fde57d8f0c28` |
| Harvest | `cc57b6578cdca8485e50f345e4fb5babfcd093d9` |
| Admin | `f26d55c86181e9e812a7b1a6a0a45239b5343d0e` |
| AI Gateway | `970cea8f7f4382b61cf45da9d3ee582d1b061828` |
| Identity | `96005c2fd17d11bd91e7c42357ed715d884d8959` |
| Notification | `eaeaf046674291d8505eb3a3f58c4e3d339aa68b` |
| Billing | `e9a1942f875d9035d21f8174e69715401b29c747` |
