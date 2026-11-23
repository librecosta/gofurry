# STEAM-R1-F3 · 公开媒体最终验收

2026-11-24（Asia/Shanghai）。**Steam Media Data Parity PASS · Public Media Experience Parity PASS · R1 Overall PASS**。公开页已能播放 Paws 和 Stray 正式发布的 HLS 影片，完成 R1-E 遗留的公开播放验收。[最终封板](steam-r1-final-closure.md) · [R1-E 阶段验收](steam-r1-e-end-to-end-acceptance.md)

## 基线与环境

| 仓库 | 实测提交 | 依据 |
| --- | --- | --- |
| Harvest | `ead956ebc9a17191cec56ef79d35a86370960253` | [R0-B](https://github.com/librecosta/harvest/blob/ead956e/docs/steam-r0-b-acceptance.md) · [R1-A](https://github.com/librecosta/harvest/blob/ead956e/docs/steam-r1-a-acceptance.md) · [R1-B](https://github.com/librecosta/harvest/blob/ead956e/docs/steam-r1-b-acceptance.md) |
| GoFurry | `c3312be1e19647527c909adcb7237a6b1736ba79` | [R1-C](steam-r1-c-acceptance.md) · [R1-C-F1](steam-r1-c-f1-media-cursor-acceptance.md) · [R1-F1](steam-r1-f1-acceptance.md) · [R1-F2](steam-r1-f2-acceptance.md) |
| Admin | `46a2d7767a8b51c06032ea26349094a6c8e51fb1` | [R1-D](https://github.com/librecosta/admin/blob/46a2d77/docs/steam-r1-d-acceptance.md) |

验收开始时各仓库本地及远端 `develop` 一致。Windows 原生 Go 1.27.4、PostgreSQL 18.6、Node 24.22.1、pnpm 12.8.1、Microsoft Edge 158.0.4370.33。重新构建 GoFurry API 和 Web SSR，使用既有显式私有 YAML、环回隔离数据库及 Admin Embed。Harvest 已归档证据使用只读查询复算。没有新 AppDetails 或 GetItems 请求、迁移、共享业务数据写入或正式发布状态变更。私有输入、日志、截图及测试命令保存在已忽略的 `.local/steam-r1-f3`。

只读 Goose 状态显示 Harvest 开发库已应用 `13`、GoFurry CN 开发库 `202611230002`、Admin 开发库 `202611050002`，三个已配置目标均无待执行项。INTL 使用已有独立隔离库验收。INTL 共享环境及各生产目标的 Goose 状态由各区域上线核验确认。

## 来源到正式快照

| 样本 | AppDetails v2 真实来源 | StoreBrowse v1 真实来源 | 正式采用 |
| --- | --- | --- | --- |
| Paws · 434000 | en/HK，出口 cn，2 部影片、12 张截图 | en/US，出口 cn，9 张资产 | en，影片 `256662121`、2 张截图与 Header |
| Stray · 1332010 | en/CN，出口 cn，3 部影片、8 张截图 | en/US，出口 cn，15 张资产 | zh，按序采用影片 `256896952`、`256844882`，8 张截图，Library Capsule 封面及 Library Hero |

- **采集与来源。** 两款 AppDetails 归档重新计算 SHA-256 并核对 Attempt Attribution，AppID、影片 ID、名称、三类 Manifest 路径及截图 ID 原始顺序与既有规范化样本一致。StoreBrowse 两条隔离 Harvest `present` Observation 的完整载荷与原真实 Worker 样本一致，确认 AppID、`steam.assets`、Schema 1、市场和出口。Paws 在 R1-E 的 en/CN Assets 是隔离重投影，实际源为 en/US。两个真实样本有 Library Capsule 和 Hero，Library Logo 与独立 2× 角色还由既有夹具覆盖。[R1-E 来源验收](steam-r1-e-end-to-end-acceptance.md#e2--来源编辑稿与正式快照)
- **GoFurry 追溯。** 根据正式快照逐项核对 Movie/Screenshot ID、完整对象、已批准绑定、GameSource、Target、Scope 和 Content Hash。Stray 两部影片的 Observation 仍与当前投影直接匹配。Paws 快照保持较早的 Observation 引用，同一绑定和 Scope 的当前投影有新的 Observation ID，Content Hash 与规范化载荷相同。以快照历史引用、同哈希的已绑定投影及归档原始证据交叉核对影片 `256662121`。Stray 的 Capsule、Hero 单独核对其 StoreBrowse Observation、资产 ID、Type、Family、Slot 和 URL。
- **编辑与发布。** 回读隔离 Admin 审计和 GoFurry Operation Receipt，以 Request ID、目标及 Operator 角色核对结构化保存、首次 Publish、Publish Changes 的成功回执。R1-C/E 的实际写入验收覆盖独立 Draft、Preview Token 作废、保存后公开快照保持不变、重新发布后才更新，以及编辑顺序和原始顺序各自保留。F3 保留已发布状态，通过只读回读和无写入的失败校验复验。[R1-C 验收](steam-r1-c-acceptance.md) · [R1-E 发布验收](steam-r1-e-end-to-end-acceptance.md#e2--来源编辑稿与正式快照)
- **Admin 边界。** 真实 Admin → GoFurry → 隔离 PostgreSQL 再次分页得到 12 + 12 + 1；未登录 401、缺 CSRF 403、Developer 可读但带有效 CSRF 写入仍为 403、Operator 过期 Revision 409。过期请求关联到同一 Request ID 的 requested/result 审计，未产生 GoFurry 写入回执。前后 Revision、Draft 和正式快照相同。
- **公开 Profile。** 两款游戏 en/zh Profile 200，`published_media` 与正式快照投影逐项相等，Movie/Screenshot ID 使用字符串，三种真实 HLS/DASH 来源独立保存，截图原图和缩略图分开，采用顺序一致。四来源独立夹具同时保留不同 GameSource 的封面、角色图、截图和电影。旧版 URL 画廊仍可打开，精简详情保持原契约。公开 DTO 不含内部 Origin、Observation、Scope 或 Hash。测试前后正式快照逐字段一致。

## 公开浏览器播放

在公开 SSR 页面打开已发布影片，使用 Edge 原生控件主动播放。MSE 用例只覆盖原生 HLS 能力判定以选择备用路径，清单、分片和解码均来自真实浏览器与 Steam CDN。

| 游戏 | 播放路径 | 成功的 Manifest / 分片 HTTP | 打开预览至时间推进 | 流请求数 |
| --- | --- | --- | ---: | ---: |
| Paws | Edge 原生 HLS | 206 / 206 | 7.02 s | 19 |
| Paws | HLS.js / MSE | 200 / 200 | 4.89 s | 6 |
| Stray | Edge 原生 HLS | 206 / 206 | 15.01 s | 12 |
| Stray | HLS.js / MSE | 200 / 200 | 4.92 s | 7 |

四次均观察到真实分片与用户播放后媒体时间超过 0.25 秒。视频封面、名称与正式顺序可见。初次进入页面与打开图片画廊时流请求为 0。打开单部影片后浏览器做 2–3 次播放准备请求，视频保持暂停且未自动播放。关闭预览后焦点复位，媒体暂停、移除源并退出 DOM，MSE 的 Blob URL 被释放；另用真实 Paws MSE 在同文档路由返回时再次观察到同样的资源清理。上表是单次本机到 CDN 的测量，流请求含浏览器的范围请求及清单处理。

专项 Playwright 9 项通过。MP4/WebM 通过本机测试媒体在 Edge 实际解码及播放。夹具通过 DASH-only 保留封面与不可播放状态、仅缩略图、CDN/CORS/解码错误、直接文件失败后转 HLS、拒绝外域清单子请求、切换媒体和取消过期 HLS 动态导入。图片覆盖 Logo 与 2× 角色、原图/缩略图失败、懒加载、390 px 深色与 1280 px 浅色、键盘、Escape、焦点恢复与无脚本入口。本轮未实测 Firefox、Safari、实体移动浏览器播放及其他地区的 Steam CDN。

## 兼容性、区域与性能

- Harvest 最新代码原生测试通过，保留 `steam.details:1:1:steam_v1`、`steam.details:1:2:steam_v2`、`steam.assets:1:1:steam_assets_v1`。旧计划、Consumer 兼容及独立 Permit、配额和 ACK/Cursor 故障恢复沿用 R1-E 的原生隔离证据。本轮复核当前来源持久化和快照，未重新执行会改写来源或发布的数据回放。[R1-E 恢复验收](steam-r1-e-end-to-end-acceptance.md#e6--恢复)
- CN 和 INTL 各自连接独立 PostgreSQL，通过本区域 API 与 SSR 复验 en/zh、正式媒体语言回退、SFW 响应头、跨库 404。Draft、Archived 和公开监听器上的私有 Management 路径均不可读；隔离 Draft 内容未流入公开结果。Public ID、Slug、canonical 与 hreflang 均保留。模拟 CDN 失败后仍能读取游戏详情。真实 Steam CDN 播放使用 CN 隔离快照、本机浏览器网络，INTL 使用独立正式发布夹具验证区域和错误边界。
- 本轮新跑 GoFurry `go -C services/api test ./...`、原生 API 构建、Web typecheck/build、Edge 专项 Playwright；Harvest 的 provider/execution/handlers/worker 与 Admin 的 gofurry/handlers/routers 原生包测试均成功。原生环回 HTTP 与 PostgreSQL 回读、Admin 网关和真实 CDN 校验分别运行。全量 `task check` 与 HMR 的证据沿用 F2 对相同代码树的干净检查。原工作树既有被忽略诊断文件保留。[F2 原生检查](steam-r1-f2-acceptance.md#专项及原生检查)

| 测量项 | 本轮结果 |
| --- | --- |
| Paws en / zh Profile JSON | 2,353 / 2,352 B |
| Paws en / zh 初始 SSR HTML | 13,360 / 13,378 B |
| Stray en / zh Profile JSON | 6,029 / 6,030 B |
| Stray en / zh 初始 SSR HTML | 20,095 / 19,929 B |
| 页面初始加载 JS | 449,154 B，Edge `encodedBodySize` 合计 |
| Gallery 独立分包 | 17,119 B，gzip -9 为 5,693 B |
| HLS.js 1.6.19 light 独立分包 | 331,491 B，gzip -9 为 103,325 B |
| 按需加载 | 进入公开页时无 Gallery/HLS 分包与媒体流。打开画廊后图片懒加载，截图先读缩略图。打开确需 MSE 的影片才加载 HLS 分包 |

Profile 与 SSR 为环回 HTTP 接收体积。gzip 为本地构建文件压缩值，真实网络指标见上表。Harvest Segment、GoFurry 存储、Admin 分页以及 Draft 保存的测量沿用 R1-E，未采集生产延迟指标。[R1-E 性能与存储](steam-r1-e-end-to-end-acceptance.md#e7--性能与存储)
