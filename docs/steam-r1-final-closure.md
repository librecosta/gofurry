# Steam R1 · 最终封板

2026-11-24（Asia/Shanghai），基于 Harvest `ead956e`、GoFurry `c3312be`、Admin `46a2d77` 封板。[R1-F3 逐项验收与实测](steam-r1-f3-final-acceptance.md)

| 结论 | 状态 | 依据 |
| --- | --- | --- |
| Steam Media Data Parity | **PASS** | 采集、独立资产、持久化、选择、正式快照与来源追溯通过。Paws 和 Stray 的公开 Movie ID 与持久化快照及真实来源一致 |
| Public Media Experience Parity | **PASS** | 已发布图像角色、截图、影片名称和封面均可见。Paws 和 Stray 通过 Edge 原生 HLS 和 HLS.js/MSE 实播，直链 MP4/WebM 通过真解码夹具，DASH-only 显示封面和明确的降级状态 |
| R1 Overall | **PASS** | 两项核心验收通过，冻结数据、编辑发布和公开媒体契约。生产上线仍按 CN、INTL 独立核验 |

R1-E 的 **Public Media Experience PARTIAL** 是当时的验收记录。F1 新增基于正式快照的结构化公开投影，F2 接入公开画廊和 HLS 播放，F3 复验真实 CDN 分片、视频时间推进与资源清理。[R1-E 历史封板](steam-r1-e-media-parity-closure.md) · [F1 公共契约](steam-r1-f1-published-media-projection.md) · [F2 画廊契约](steam-r1-f2-public-media-gallery.md)

## 阶段与能力矩阵

| 阶段 | 状态 | 封板能力与契约 |
| --- | --- | --- |
| R0-A / R0-B | PASS | AppDetails 原始证据、SHA-256、Attempt 归属、鉴权读取、按 Scope 保留、配额与清理 |
| R1-A | PASS | `steam.details:1:1:steam_v1` 保持旧媒体数组，显式选择 `steam.details:1:2:steam_v2` 取得完整影片、结构化截图与基础图片，保留原始顺序及每种格式 |
| R1-B | PASS | `steam.assets:1:1:steam_assets_v1` 独立 StoreBrowse 与 Permit，保留 Store、Library、Community、资源 Slot 与大小变体。规范化结果有确定身份、截断标记和 32 KiB 总上限 |
| R1-C | PASS | 区域持久化、按版本和 Scope 隔离、来源校验、显式 Plan Output Schema、分别保存 Draft 与带来源身份的正式快照。旧计划默认 v1 |
| R1-D | PASS | Admin 跨来源采用及排序、视频预览、独立保存、Revision、Preview Token、权限、CSRF 和审计。每语言一个封面、每角色一图、八张截图、四部影片 |
| R1-C-F1 | PASS | 媒体候选用 UUIDv7 keyset，隔离网关连续 12 + 12 + 1；普通目录继续使用 Public ID |
| R1-E | 阶段验收 | 数据与编辑发布 PASS，公开体验当时 PARTIAL，报告保留 |
| R1-F1 | PASS | 顶层 `published_media` 只读本区域正式发布快照，支持跨来源混合采用和明确的媒体语言回退。既有 `sources[].media` 和精简公开详情保持兼容 |
| R1-F2 / R1-F3 | PASS | 公开 Gallery 展示正式采用的图像角色、截图、影片封面与名称。直接文件、原生 HLS 及按需 HLS.js，DASH 降级。真实 CDN 复验完成 |

| 已恢复的旧站能力 | 公开端行为与边界 |
| --- | --- |
| AppDetails 截图及基础图片 | 缩略图、原图和编辑顺序保留。使用正式发布封面，载入失败由图片组件兜底 |
| Library Capsule、Hero、Logo 及 StoreBrowse 变体 | 按独立角色渲染已采用的官方图。真实样本缺少 Logo，Logo 和 2× 类型区分由夹具复验 |
| 完整 Steam 影片与多格式 | 按正式顺序显示字符串 Movie ID 对应的影片条目、封面和名称。MP4/WebM 和 HLS 已验收，DASH 地址仍保留在契约中，公开画廊没有 DASH 解码路径 |
| 编辑选用及发布稳定性 | 来源候选更新、归档过期和 Consumer 重放不改写人工选择与正式快照。公开投影只使用已发布的数据，无实时 Harvest 依赖，也无私有 Origin 或证据 |
| 区域及旧资料兼容 | CN、INTL 独立数据库和 API。旧版素材游戏可继续浏览，Public ID、Slug、SFW 与既有 SSR/SEO 契约保留 |

## 生产推广核验

本轮上线状态为**未执行**。已配置的开发数据库迁移状态为 Harvest 13、GoFurry CN 202611230002、Admin 202611050002，均无待执行项。实际生产库、账户、出口及 CDN 可达性在各自区域确认。

1. **Harvest。** 先核对 Server、现有 v1 Schedule、Preview Namespace、Raw Evidence 容量及保留配置，再部署支持 v1/v2/Assets 的 Server，最后升级 Worker Offers。通过明确的授权和配额检查后再试验新 Offer。StoreBrowse 保持独立 Permit 与至少六小时调度间隔，审核共享出口、限流和授权条件。保留旧 Worker/Schedule；没有自动创建生产 v2/Assets 计划。
2. **GoFurry。** 每区先核对 Goose、已有发布快照和数据备份，确认 Consumer 能识别三种 Offer、按正确 Cursor 处理并隔离预览流。部署兼容的 Consumer、含 UUIDv7 分页的私有 Management 和含 `published_media` 的公开 API，再部署依赖它们的 Admin Embed 和 Web。私有 Management 保持私网可达。
3. **Admin。** 使用该区域独立的登录身份验收角色权限、CSRF、Revision、分页、保存回执和审计。核查媒体及连接 CSP、HLS 按需分包。发布联调采用经授权的样本与窗口。
4. **CN 公共端。** 核对 CN 库、域名、SFW、zh/en canonical 与 hreflang，比较旧版及结构化已发布样本。使用该网络实测原生 HLS、MSE 的清单、分片和播放推进，查看退化状态及跨区访问拒绝。
5. **INTL 公共端。** 独立核对 INTL 库、域名、账号、私有通道、SFW 与基于当地实际出口的来源和配额。验证英文首选及中文回退、独立发布、跨区访问拒绝和本地网络的 CDN 实播。F3 的 INTL 隔离夹具通过接口、SSR 与故障测试，实际区域 CDN 和账号链路仍待核验。

可独立回退公开 Web 至兼容字段路径，必要时暂停 Preview 计划和新版编辑写入，保留 v1 调度和已保存的 v2 快照、候选与证据。回退到不识别 v2/Assets 的 Consumer 前，须先隔离对应流或暂停相关消费，复核 Cursor 与 ACK 后恢复。已有业务数据的 Schema 不以 Goose Down 作常规回滚手段。

## 保留事项与交接

- Windows Edge 原生 HLS 和 HLS.js/MSE 有实播证据。Firefox、Safari、实体移动浏览器及异地 Steam CDN 未实播，移动端的本轮覆盖为 390 px 布局和交互。
- DASH-only 的公开降级属于当前契约；增加 DASH 播放需单独验证浏览器、CDN、CSP 与资源清理。此次 F3 没有生产变更或新数据库迁移。
- 下一项工作为 `librecosta/admin` 的 **ADMIN-G0 — Current-State Product Audit**。可引用 R1-D/E 的真实编辑发布证据、F3 的网关跨页与权限回归、正式快照的来源追溯，以及本报告的区域上线核验项。此次验收到此结束，STEAM-R2 留待独立启动。
