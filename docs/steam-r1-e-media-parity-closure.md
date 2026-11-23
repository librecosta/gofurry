# STEAM-R1-E · 媒体能力封板报告

**Steam Media Data Parity · PASS**。现有三种 Offer、结构化候选、编辑选择、Admin 预览与发布快照完成验收。本报告冻结 R1 的数据和编辑发布契约。

**Public Media Experience Parity · PARTIAL**。现有公开投影支持封面、截图和兼容直链。实际 Paws 和 Stray 样本的 HLS/DASH 视频没有进入公开播放路径，公开 DTO 也未保留扩展图片的角色信息。R1 整体为 **PARTIAL**，本轮未宣布完整封板。测试证据与实测性能见 [端到端验收](steam-r1-e-end-to-end-acceptance.md)。

## 交付与冻结边界

| 阶段 | 状态 | 冻结契约 |
| --- | --- | --- |
| R0-A / R0-B · 证据与保留 | PASS | AppDetails 原始响应与 SHA-256、Attempt 归属、受控读取。默认按来源 Scope 最多 5 版，历史窗口 90 天，保护活跃来源最近版本，容量由 Server 显式配置 |
| R1-A · AppDetails 媒体 | PASS | `steam.details:1:1:steam_v1` 保留旧字符串契约。显式 `steam.details:1:2:steam_v2` 保留基础图、字符串 Movie ID、名称、封面、Highlight、源顺序、各播放格式与结构化截图 |
| R1-B · 扩展图片 | PASS | `steam.assets:1:1:steam_assets_v1` 独立 StoreBrowse/Permit。Store、Library、Community 类型、原 Slot、Asset ID 与 URL 进入载荷。Library Capsule、Hero、Logo 和 2x 变体保持区分，缺失有效 |
| R1-C · 来源、编辑、发布 | PASS | GoFurry 按版本和 Scope 持久化，以有效绑定校验 GameSource/Target。独立 Draft/Published JSON v2 保存 Origin 和完整采用对象，旧媒体 URL 仍可读写。显式 Plan `output_schema`，旧计划默认 v1 |
| R1-D · Media Studio | PASS | Admin 固定私有网关，Session/CSRF/角色/Actor/审计，按稳定对象选择，调整展示顺序并保存。每语言封面 1、每角色 1、截图 8、视频 4。MP4/WebM/HLS 预览、失败提示和发布差异 |
| R1-C-F1 · 候选分页 | PASS | 管理媒体候选每页 12，UUIDv7 keyset。目录继续使用 Public ID，真实联调 12 + 12 + 1 |
| E · 公开播放与展示 | PARTIAL | 本地正式快照驱动 API/SSR。公开视图有已发布封面、按序截图和兼容直链，实际只有 HLS/DASH 的影片无公开播放路径 |

GoFurry 候选按 Namespace、Target、Capability、Output Schema、Locale、Store Country 和 Egress 区分来源。同一 Steam AppID 的不同来源、时点和 Observation 可分别追溯。采集保留 32 KiB Segment 上限，带 `truncated` 的部分结果保持可识别。管理选择以 GameSource、Observation、稳定媒体 ID 指定，保留每项原始 Order 与来源时间，由编辑数组位置决定展示顺序。公开内容只有在通过 Review、Preview Token 和 Revision 校验并正式发布后才改变。已发布快照无需实时调用 Harvest 或读取 Raw Evidence。

## 与旧站能力对照

| 能力 | 采集、存储及编辑端 | 公开端 |
| --- | --- | --- |
| AppDetails 图片与截图 | 原图、缩略图、原始顺序和角色图片已恢复 | 已发布封面与截图显示。Gallery 使用截图原图的兼容投影 |
| StoreBrowse 官方图 | 独立 Store、Library、Community 角色与大小变体已恢复。旧站纵向封面 Last Known Good 使用的来源隔离、运营可见性及保留能力已具备 | 可选 Library 图片作为封面，扩展角色与多变体未直接暴露 |
| 影片与格式 | 影片身份和 MP4/WebM/HLS/DASH 来源分别持久化。Admin 原生 HLS 和 HLS.js 实播通过，DASH-only 有明确状态 | MP4/WebM 兼容直链。公开 JSON 和画廊均没有 HLS/DASH 播放路径，样本无空视频位 |
| 来源与出版稳定性 | 证据、Attempt、有效绑定、候选、编辑和正式快照各自可核对。采集失败、历史清理和 Consumer Reset 不覆盖已发布选择 | 从本区域正式快照投影。图片网络失败时游戏详情仍可读 |

## 生产推广准备

**本机 CN 数据链 PASS，独立 INTL 公共 API/SSR 基础回归 PASS，实际生产推广未执行。** INTL 的账号、CDN 可达性、真实 Steam 出口及编辑发布链需要本区域单独验收。隔离测试的 `intl` Egress 标签和 CN 的 Steam 样本不能作为 INTL 实际来源证据。

1. 逐区域分别确认数据库、区域头、SFW 状态、Management 环回或私网可达性及独立 Admin 会话。核对 Goose 至当前要求版本。开发环境已确认 Harvest v13、GoFurry `202611230002`、Admin `202611050002`。生产版本和状态应在窗口内独立核对。
2. 先升级 Harvest Server，再更新 Worker Offers。保留 v1 Worker 与 Schedule。预览 Namespace 及 Steam Key/共享出口配额按当前规则单独审核。AppDetails Raw Evidence 策略与 StoreBrowse Permit 各自核对。Assets 的最低调度间隔为六小时。
3. 部署识别 v1/v2/Assets 的 GoFurry Consumer 及私有候选 API，再升级 Admin Embed。复核读权限、写入 CSRF、正确的 Steam CDN 媒体与连接 CSP、按需加载 HLS。GoFurry 先于依赖 UUIDv7 分页的 Admin。
4. 只在已授权的 Preview Namespace 明确开启 v2/Assets，逐区域运行小样本并核对 Cursor、候选与已发布快照。已有 Schedule 保持稳定，没有自动创建生产 v2/Assets Schedule 或批量更新游戏。

回滚时先停止相应 Preview 计划和新版编辑写入，保留 v1 调度、已保存快照及证据历史。已有 v2 数据时跳过破坏性 Goose Down。恢复之前先验证备份、快照与版本读取兼容。若回退到拒绝 v2 的 GoFurry Consumer，保持含 v2/Assets 的流隔离或暂停其消费，核对 Cursor 再恢复。

## 独立后续任务

| 状态 | 归属及最小范围 | 验收 |
| --- | --- | --- |
| BLOCKED · 公开 HLS/DASH 播放 | `librecosta/gofurry`。为 Public Game Profile 增加只读的已发布媒体投影，按播放格式暴露必要来源与海报，不暴露私有 Origin。范围包括画廊的 HLS 播放路径及 DASH 降级，无需现有快照补采或修改数据库 Schema | 用已发布的 Paws / Stray 快照，在公开页面确认真实 Manifest、分片、播放时间推进与关闭清理。覆盖 MP4/WebM、DASH-only、CORS 失败的明确降级，以及仍只读取已发布快照 |
| PARTIAL · 公开图片角色 | `librecosta/gofurry`。若公开界面需要 Library Hero、Logo 等角色，基于同一发布 DTO 显式投影，沿用现有封面及旧客户端语义 | 发布一组包含不同 Library 角色的夹具，验证角色、缺失可选图、缩略图和原图语义 |
| PARTIAL · 生产区域验收 | 发布作业。CN 与 INTL 分别核对账户、库、私有 Management、出口及配额 | 逐区域独立验收，保持现有已发布游戏的独立快照和 v1 Schedule |

此次交付为验收文档，核心实现与数据库迁移沿用已有版本。
