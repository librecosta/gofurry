# MEDIA-U1 开发库迁移验收

2026-11-24，GoFurry `develop` 基线 `7e011437`。真实开发目标为已配置的 CN PostgreSQL 18.6 `gofurry_cn_dev`，Goose 从 `202611240004` 迁移至 `202611240005`。持久 INTL 开发库未找到独立的明确配置。区域回归使用不同的本机隔离数据库。

## 备份、写入和内容保护

迁移前确认 CN API 与 Consumer 连接无活动事务，暂停其已核实的本机开发进程。制作了 1,314,514 字节 PostgreSQL custom-format 一致性备份，验证目录及 SHA-256，完整恢复到本机独立 PostgreSQL 18。恢复库的 66 张业务表行数及逐表摘要与开发库相符。在恢复库执行 Goose Up、归一化和第二次归一化，比较前后摘要，再对真实 CN 开发库执行相同操作。实际 Goose Status 确认 `202611240005 applied`。

| 开发库指标 | 实际结果 |
| --- | ---: |
| 游戏及有来源素材的游戏 | 31 / 31 |
| 归一化 Observation | 105 |
| 逻辑素材 | 752 |
| 官方图片类素材 | 356 |
| 截图 | 304 |
| 影片 | 92 |
| 来源版本 | 1,455 |
| Variant | 2,703 |
| 同版本标准图及 2× 配对 | 160 |
| 标记截断的来源 Observation | 0 |
| 原有 V2 草稿语言 / 发布语言 | 0 / 0 |
| Library 关系 / V2 草稿映射 / V2 发布映射 | 0 / 0 / 0 |
| 已失去当前来源的 V2 发布引用 | 0 |
| 无法归一化记录 | 0 |
| 复跑新增 Variant / Library / 映射 | 0 / 0 / 0 |

原有的 66 张业务表摘要迁移前后一致，包含 Game、GameSource、编辑稿和 62 条正式发布本地化快照。新的 6 张表在第二次执行前后行数和摘要一致。备份、SHA-256、每游戏计数及 HTTP 台账保存在 Git 忽略的 `.local/media-u1`。CN 实际有旧正式发布内容，但库内没有 V2 媒体编辑稿或发布快照，因此没有虚构运营采用、调整现有封面或自动加入画廊。

## 验收

- 真实来源恢复演练发现 Steam AppDetails 的官方背景 URL 可不带扩展名。对实际 Steam AppID 路径作验证后，以原始 URL 持久化，格式为未知。最终恢复库和开发库的归一化均无错误。
- 隔离 Consumer 使用此前采得的 Paws 和 Stray 样本，验证 v1/v2/Assets 并存，核对真实 Movie、Screenshot 及 9/15 个 StoreBrowse 槽位。既有 V2 编辑和显式发布继续工作。模拟旧来源撤回后完成 8 条草稿和发布映射及 4 条运营保留。复跑无变化，HTTP 410 与新 Epoch 不会清掉素材版本或变更公开快照。
- 隔离 Management PostgreSQL 验证同一游戏内 Steam 与预留 itch.io 来源身份独立，以及 94 条混合素材按 32 / 32 / 30 分页。分批加入、撤销、重排、重复操作、过期 Revision、Developer 写入限制、回执和 `publish-changes` 快照路径均已验证。原有快照在新版私人编辑过程中保持一致。
- Edge 公开媒体专项 Playwright 10 项通过，包括无脚本 SSR、图片角色与尺寸、原生直接播放、HLS 错误降级和关闭清理。Go vet、全 Go 测试及 Windows API 构建（`task check:api`）、SQLC 差异检查（`task sql:check`）和 Web TypeScript 检查通过。

## 开发服务与回归边界

开发服务继续使用原 CN 私有 YAML；新二进制、PID 和服务日志在 `.local/media-u1`。Harvest、Admin 和公开 Web 沿用原部署。迁移只读了现有持久来源，未安排 Steam 重新采集。公开 API、Admin V2 网关契约和 Web 画廊使用既有正式媒体。

| 原生运行验证 | 结果 |
| --- | --- |
| CN 开发 Public 和 Management readiness | 均为 HTTP 200 |
| CN 开发 Management | 31 款游戏逐页完整回读 1,455 个版本，旧 V2 候选仍有 105 组。CATO 按 Steam、视频及中文来源筛选后，详情保留 Poster、HLS H264、DASH H264 和 DASH AV1。公开监听器访问私有素材路径为 404。 |
| 公共 Profile 与 SSR | 31 款游戏的中英文共 62 个公开 Profile，媒体、本地化及身份字段迁移前后逐项一致。CATO 原公开 URL 的 SSR 返回 HTTP 200 并保留 canonical。 |
| R2 / R3 / R4 | CATO 有 2 个 Rich Details 来源，详情读取 200。Store Events Management 返回 8 项，公开中文新闻返回 4 项。Facts 与 Changes 均返回 200。 |
| CN / INTL 隔离 | 不同的本地 PostgreSQL/API 和 SSR 夹具验证 SFW、中英文回退、跨区域 404、CDN 失败后的页面可用性。 |
| 真实 Steam CDN | Edge 实际播放 Paws 和 Stray。Paws 原生 HLS / MSE 分别约 8.5 / 6.0 秒出现播放进度；Stray 分别约 11.2 / 3.9 秒。四种组合均收到 Manifest 和分片，退出后停止播放，MSE Object URL 已释放。Stray 原生 HLS 第一次收到分片但 17 秒内没有推进，重新打开后通过。 |

本轮没有修改或构建新的 Web UI，浏览器回归使用已有原生 SSR 构建。旧 Admin 的读取契约通过真实开发 Management HTTP 回读，V2 写入和发布在隔离 GoFurry 数据库验证。真实开发库只执行新 Schema 与数据归一化，未保存人工选择或触发发布。
原工作树的全目录格式脚本会扫描此前已存在的忽略目录和本机诊断 Go 文件，报告这些文件未格式化。验收另行检查 Git 跟踪的 Go 文件及本轮新增文件，保留诊断文件原样。
