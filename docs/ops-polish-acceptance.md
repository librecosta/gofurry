# OPS-POLISH 验收记录

## 交付提交

| 仓库 | `develop` 实现提交 | 主要改动 |
| --- | --- | --- |
| GoFurry | `5bc7731a46adb24c24b71df05ce9cd92ebd075d0` | `internal/storage/site_assets.go`、`internal/catalog/*publication*.go`、`internal/catalog/game_collections.go`、`internal/catalog/navigation.go`、`internal/management/collection_detail.go`、`internal/collection/management_detail.go`、`internal/database/queries/collection_management.sql`、公开导航与前向迁移 |
| Admin | `4266fe6f44f801c9140617411acfd4e1f1e867de` | `catalog.tsx`、`site-image.ts`、`workspace.tsx`、`game-publication.tsx`、`game-editor.tsx`、`collection-info.tsx`、GoFurry 代理与 Playwright |
| Harvest | `e03ced357b6abbf0174cd87722981c3ac191bfd3` | `db/queries/jobs.sql`、`internal/{execution,handlers}/jobs.go`、路由、API 文档与前向索引迁移 |

GoFurry 的 CI 夹具引用该 Harvest 提交，Admin 的 D4 夹具引用该 GoFurry 实现提交。代码已在本地提交。本轮未推送或运行远程 CI。

## 行为和数据

- 网站 Logo 与 Cover 接收 PNG、JPEG、WebP。浏览器在严格比例内等比缩小并预览最终 WebP，保留透明通道。Logo 为 1:1，最大 1024×1024 和 1 MiB。Cover 为 16:9，最大 2560×1440 和 4 MiB。GoFurry 独立验证编码、尺寸和大小，按 SHA-256 生成 `.webp` 对象键，并在存储验证后用当前 Revision 绑定。历史 Cover 的 `.avif` 键、Content-Type 和 URL 保持兼容，按 AVIF 验证。官方壁纸和 Steam 原始素材契约保持原样。
- 网站、游戏和合集的常规状态操作由确认按钮提交。首次网站发布继续提交 `confirm_sfw`，分类和地址完整性仍由服务端校验。游戏发布和发布更改展示服务端预览并提交 `confirm_relevance`、`sfw`、Revision 和 Preview Token。更新分级继续提供实际的分级选项。状态操作使用系统生成的说明，保留既有身份和回执机制。新写入的网站、游戏和合集发布决定可用 Management Request ID 核对。GoFurry 继续隔离游戏编辑稿和公开快照。
- 只读盘点 `gofurry_cn_dev` 的 1 条网站记录，`translation` 引用为 0。LibreCosta（`k01nf5x20f1bzp3d`）保留 `directory` 与 `wiki`。GoFurry `202611210002` 将有效固定标签从 12 个减为 11 个，8 个分类不变。迁移保留历史 AVIF 键并接受新 WebP 键，允许以既有发布决定表记录合集。若目标库仍有 `translation` 引用，约束验证会令迁移失败并回滚，需要先明确处理引用。
- Admin 通过 GoFurry Management 的 `GET .../{sites|games}/:id/collection` 与 `GET .../collection/:plan/history` 读取。GoFurry 核对有效实体、来源、绑定、计划和作用域，再使用 Harvest 的现有只读凭据。Harvest 新增 `GET /api/v1/schedules/:id/jobs`，仅返回授权 Schedule 的匹配 Job、最近失败与尝试摘要。列表分页，观测状态和最近成功时间来自 GoFurry 已投影的记录，计划启用与下次运行时间来自已核对的 Harvest Schedule。连接不可用时显示最近已知信息。Harvest `00008` 为精确 Schedule 查询增加分页索引及失败索引。
- 采集展示不修改发布状态、人工分级或编辑稿，不改变归档时的计划生命周期。R3 保留按准确 Plan 授权触发采集及归档后 Schedule 治理。本轮没有触发采集任务或新的 Steam 请求。

## 原生验证

- Windows Go 1.27.4。GoFurry、Admin 和 Harvest 的原生 `go vet`、`go test`、`go build` 通过。GoFurry 与 Harvest 的 sqlc diff 和 vet 通过，Harvest 的 Goose 语法校验通过。
- 在 VPN PostgreSQL 18 的连接级临时表验证两条迁移。GoFurry 覆盖已保留标签、历史 AVIF、接受 WebP、拒绝重绑翻译标签、公开标签筛选和相关推荐，以及合成网站与合集的无原因状态切换、角色限制和回执关联。Harvest 验证两个索引创建后回滚。存储测试用本地 HTTP 夹具检查 WebP 写入及历史 AVIF 原 URL 验证。
- GoFurry Web 类型检查与构建通过。原生 Playwright 在备用端口通过 7 项 SSR 与治理检查。Admin 类型检查、Vite 构建及 Go Embed 测试通过。Admin 浏览器常规检查通过 7 项，22 项依赖专用夹具而跳过。另以原生 Chromium 通过 3 项定向检查，覆盖透明 PNG、JPEG 与 WebP 转换、比例和大小提示、失败后保留原封面、只需二次确认、使用服务端发布预览及 Token、采集失败和上次成功分开显示、分页，以及真实 BROK 和 LibreCosta 的只读页面。
- 用户正在使用的 GoFurry API `17321`、SSR `17320`、Admin API `8080` 的 `/readyz` 各返回 200。新代码使用独立回环监听进行 GoFurry Management、Harvest 和 Admin/Vite 联调。采集查询使用 PostgreSQL 只读连接。Admin 登录的测试账号仅存在于连接级临时表。匿名采集查询返回 401，跨实体查询 BROK 的计划返回 404。
- BROK（Steam `949480`，`j2byfcwgr8cqnkt4`）两个有效计划分别显示 Steam English / US 和简体中文 / CN。EN/US 读取观测 `01a1f44a-c716-7480-8efb-abc753982cbe` 和 3 条 Job 历史。ZH/CN 读取观测 `01a1fdf7-472d-7935-8fbf-4beee0fe8265` 和 2 条 Job 历史。两条观测均是 `present`，计划均启用。LibreCosta 没有有效目标和计划，Admin 显示「未配置采集」。

## 共享库和人工验收

| 真实开发库 | 已应用 | 待应用 |
| --- | --- | --- |
| GoFurry `gofurry_cn_dev` | `202611210001` | `202611210002` |
| Harvest `harvest_dev` | `00007` | `00008` |
| Admin `librecosta_admin_dev` | `202611050002` | 无 |

共享开发库未执行升级。升级前需要核对可恢复备份并获得单独授权。需先应用 GoFurry 迁移，再在实际服务启用 WebP 封面绑定与合集审计写入。真实网站素材上传到配置的对象存储、真实条目的发布或状态切换留给维护者验收。生产环境及用户启动的开发进程未变更。

Docker、Docker Compose、Podman、Testcontainers 执行次数为 **0**。
