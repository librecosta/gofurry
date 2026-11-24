# 统一游戏素材库

MEDIA-U1 将来源素材、运营保留和产品展示拆成三个独立的数据库关系。数据和 Management API 随 GoFurry 的区域部署隔离。

| 对象 | 存储与职责 |
| --- | --- |
| 来源素材 | `game_assets` 保存逻辑身份，`game_asset_versions` 保存来源版本，`game_asset_variants` 保存实际资源。Consumer 与当前来源投影、游标在同一事务中写入。 |
| 运营保留 | `game_asset_library` 保存明确选中的版本和操作来源。运营选择跨平台、语言和市场。移除选择只删除这一关系。 |
| 产品展示 | `game_asset_presentations` 和 `game_asset_presentation_items` 按游戏、语言和用途保存主封面及有序画廊。既有正式发布操作将新版编辑稿复制到独立的 `published` 快照。 |

## 来源身份和变体

逻辑身份由 GameSource、来源外部身份、来源能力、原始角色及来源身份组成。Provider 从 GameSource 取得。Steam 同一个 Movie ID 的各格式、同一个 Screenshot ID 的原图和缩略图共享逻辑身份。StoreBrowse 声明的标准版和 2× 槽位在同一 Observation 中归入一个逻辑图片，变体仍各自保存原始 ID、槽位、角色、文件摘要和上游顺序。不同官方角色即使共用 URL 也各有身份。AppDetails 和 StoreBrowse 的官方图片保留为不同来源。

来源版本按逻辑素材、绑定、Observation、Scope 和实际出口确定。版本保留 Capability、Output Schema、请求与实际出口、语言、商店地区、原始顺序、采集时间和 `truncated`。素材详情可逐项读取 Poster、MP4、WebM、HLS、DASH 及其原始 URL。未提供尺寸时返回空值。Steam 官方无扩展名背景图只接受已验证的主机和匹配 AppID 的路径，格式保持未知。采集刷新会增加来源版本；旧版本的资源 URL 保留，`current` 反映当前有效来源及绑定。重复确认和重放不会改写已经保存的素材。

目前归一化适配器支持 AppDetails Media v2/v3 和 StoreBrowse Assets v1。未来 itch.io 接入须先有真实采集、观察和可验证绑定契约，再添加来源适配器。素材身份、变体、保留操作及展示引用使用相同结构。

Harvest 的技术预算独立于素材库。Media v2/v3 单个 Observation 最多处理 12 条 Movie 和 30 张 Screenshot。StoreBrowse 最多 32 个候选，同样受字节预算约束。超限通过 `truncated` 传递。GoFurry 素材库及展示引用可跨来源和版本累积，无截图、视频或官方图的业务条数上限。

## 私有 Management API

以下路由均在 `/internal/v1/ops/games/{public_id}` 下。读写沿用 Management 服务身份、Actor、区域、Request ID 与回执。写入需要 Operator 或 Owner，比较当前 Game Revision 后在事务中完成，沿用 16 KiB 请求预算。单次最多提交 64 个变更，每页最多 32 条。接入浏览器时由现有 Admin 网关承接身份与 CSRF 边界。

| 方法与路径 | 契约 |
| --- | --- |
| `GET /assets` | 版本摘要分页。支持 `after`、`asset_id`、`platform_code`、`kind`、`category`、`locale`、`source_role`。按版本 UUIDv7 递增，返回 `items` 和可选 `next`。每项有逻辑 `id`、`version_id`、来源、时效及预览 URL。 |
| `GET /assets/{asset}/versions/{version}` | 读取该游戏内指定逻辑素材和来源版本的全部变体。 |
| `GET /asset-library` | 同样的版本分页与筛选，仅返回运营保留的版本。 |
| `PATCH /asset-library` | 提交 `revision` 与 `add`、`remove`，数组填版本 UUID。重复加入同一个版本不增加 Revision。 |
| `GET /asset-presentation` | 用 `locale`（`zh` 或 `en`）、`surface`、`slot` 读取配置。画廊使用升序整数位置作为 `after`。页包含配置 Revision，可用于在两页之间发现并发编辑。 |
| `PATCH /asset-presentation` | 提交 `revision`、`locale`、`set` 和 `remove`。每项以 `slot`、`position` 定位；`set` 传已保留的 `version_id`，也可指定该版本的 `variant_key`。`cover` 位置固定为 0 且引用图片；`gallery` 可使用稀疏整数位置并混排图片与影片。 |

新配置的 `surface` 为 `draft` 或 `published`，`slot` 为 `cover` 或 `gallery`。先加入 Library 再明确设置展示。移除 Library 关系后，仍被编辑稿或正式快照引用的版本保留。正式发布继续使用现有 Preview Token 和 SFW 校验。

## V2 兼容和迁移

旧 Management `media-candidates`、`media-selection`、`draft_media_v2`、`published_media_v2` 和现有公共画廊继续使用原有契约。有来源归属的 V2 采用素材迁入 Library。旧编辑稿和发布快照分别生成只读的 `legacy_draft`、`legacy_published` 引用，保留语言和原 Revision。原 V2 官方图片用 `artwork` 读取；原截图、影片各自保持采用顺序，在兼容映射中截图位于影片前。两种 legacy 映射与将来的产品配置分开，公开页面继续读取原正式发布数据。

使用已核验的显式区域 YAML 执行 `migrate up` 后，通过 `api game-assets backfill --config <YAML>` 补齐现有持久来源和 V2 快照。Backfill 按 Observation 和游戏分事务处理，输出非敏感记录身份及诊断代码。相同输入复跑不新增素材、变体、Library 关系或 Revision。有问题的旧记录保留原值。已首次映射的旧 Revision 后续如发生编辑，复跑会报告 Revision 变化，供运营区分旧版写入与新素材契约。
