# STEAM-R1-F1 · 验收

**PASS**。GoFurry 基线为 `develop` 上的 `fe78b5a`。2026-11-23 使用原生 Windows Go 1.27.4、PostgreSQL 18.6、Node 24.22.1、pnpm 12.8.1 与 Edge 验收。本轮交付 [公开媒体投影契约](steam-r1-f1-published-media-projection.md)。

## 真实快照与 HTTP

复用 R1-E 的隔离环境和已保存 Paws、Stray 正式快照。R1-F1 从现有环回 PostgreSQL 克隆独立 CN、INTL 测试库，Harvest 集成关闭。两款游戏的 HTTP 数据逐对象与已发布 JSONB 比较，来源与采集验证见 [R1-E 验收](steam-r1-e-end-to-end-acceptance.md)。

| CN Profile 样本 | 请求语言 → 媒体语言 | 正式采用的角色图 / 截图 / 影片 | Profile / 媒体 JSON |
| --- | --- | ---: | ---: |
| Paws · AppID 434000 | `en → en` | 1 / 2 / 1 | 2,353 / 1,485 B |
| Paws · AppID 434000 | `zh → en`（fallback） | 1 / 2 / 1 | 2,352 / 1,484 B |
| Stray · AppID 1332010 | `zh → zh` | 1 / 8 / 2 | 6,030 / 4,302 B |
| Stray · AppID 1332010 | `en → zh`（fallback） | 1 / 8 / 2 | 6,029 / 4,301 B |
| 跨来源快照 | `en → en` | 3 / 2 / 1 | 2,572 / 1,364 B |
| 近界限快照 | `en → en` | 5 / 8 / 4 | 115,437 / 113,165 B |

角色图计数不含封面。体积为无压缩本机 HTTP JSON 及其中紧凑 `published_media` 的 UTF-8 字节数。近界限夹具的 URL 长度为 2,048 字节，4 条影片各有 7 种独立播放字段。完整保留后仍正常返回，沿用原有 Profile 读取方式。

- 真实影片含 HLS H264、DASH H264 和 DASH AV1。Movie ID 在 JSON 中为字符串，截图保留各自的两个 URL 和人工采用顺序。Stray 保留 Library Capsule 封面和 Library Hero。真实样本没有 Library Logo，合成正式快照验证 Logo、2x 角色和同 URL 的不同角色。
- 跨来源合成 Game 的封面、角色图、截图、影片来自四个 GameSource 和四个 Observation。DTO 保留全部采用对象，同时省去内部身份。超出 JavaScript 安全整数的合成影片 ID 返回为 `"9007199254740993"`。
- 相同隔离库先后由旧版 `68057bb` 与本轮 API 启动。新 Profile 移除新增字段后与旧响应逐字段一致。包括 `cover_url`、`sources[].media`，不带 `profile=1` 的精简详情也一致。旧媒体夹具保留 MP4 直链，真实 HLS/DASH-only 影片的旧 URL 数组为空。
- CN 只返回本区资料，Draft、Archived、有对应快照但未公开的记录及 INTL 夹具均为 404。公开端口上的 Management 路由为 404。同游戏有中文草稿媒体但只有英文正式媒体时回退至英文正式快照。
- 独立 INTL 数据库和 API 使用本地发布夹具，验证中文快照、英文回退与 INTL/SFW 响应头。两个真实 CN 测试游戏在 INTL 返回 404，INTL 旧版游戏省略新 DTO。
- CN、INTL 原生 SSR 对接本轮 API 均为 200。CN 正式封面与 canonical 保留。INTL 缺英文译文夹具回退中文并标记 `noindex, follow`。Edge 打开 Stray 的既有画廊，显示 8 张已发布截图。

## 专项检查

- `TestPublicMediaURL` 覆盖官方静态 CDN 与镜像、格式匹配、伪造域名、非 HTTPS、凭据、查询、片段、非默认端口、路径遍历、编码分隔符、重复编码、非法编码、控制和长度边界。
- `TestPublishedProfileMedia` 与 `TestPublishedProfileMediaBounds` 覆盖语言优先与双向回退、旧版省略、截图与影片顺序、角色上限、无损 ID、保留有效字段、过滤私有内容及 100,471 B 的单元边界 JSON。
- 现有 `TestSteamMediaNative` 增加公开 DTO 断言。用 Paws、Stray 私有真实载荷复验 Consumer 后，首次 Publish 写入公开快照，新增来源和草稿保存保持旧 DTO。Publish Changes 后 DTO 才变更。410/epoch Reset、上游 503 和候选撤回后，DTO 逐字节一致。隔离测试通过，私有 overlay 只调整已有测试的受控库名。
- Catalog、Collection、HTTP Handler 原生包测试通过。在干净的暂存区导出中运行 `task check`，Go 检查与构建、Web 构建、类型检查通过，Playwright 88 项通过、3 项按套件配置跳过。原工作树的 `task tools` 会命中用户忽略的旧诊断文件 `services/api/.local/ops-data-60/diagnose.go`，既有修改和文件已保留。
- 共享 CN 开发库只读 Goose 状态仍为 `202611230002`。本阶段沿用现有 Schema，未执行迁移。私有测试输入、日志及运行配置在 GoFurry 忽略的 `.local/steam-r1-f1`。

本阶段交付 API 返回的播放源与角色。公开 Web 的结构化画廊与播放留给 R1-F2，本轮的 HTTP 与 SSR 验收未请求 Steam 媒体资源。
