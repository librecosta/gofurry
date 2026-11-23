# STEAM-R1-F2 · 验收

**PASS**。GoFurry develop 基线为 71bea30，包含 R1-F1 465c587。2026-11-23 使用 Windows 原生 Go 1.27.4、PostgreSQL 18.6、Node 24.22.1、pnpm 12.8.1 与 Chromium 内核 Microsoft Edge 158.0.4370.33 验收。[画廊契约](steam-r1-f2-public-media-gallery.md)

## 真实已发布快照

复用 R1-F1 的 CN、INTL 独立环回 PostgreSQL 与既有显式 YAML。Paws 和 Stray 来自 CN 隔离库已发布快照。Paws 正式选择 2 张截图、1 部影片、1 张角色图。Stray 正式选择 8 张截图、2 部影片、1 张角色图与 Library Capsule 封面。画廊只显示正式采用的部分。截图、影片封面、影片名、Library Capsule 与 Library Hero 均可见。样本没有采用 Library Logo，独立夹具覆盖 Logo 和 2× 角色。

Profile API 与 Web SSR 均为 HTTP 200。测试通过公开页打开影片，以 Edge 键盘操作原生控件触发播放。每次都观察到实际 CDN Manifest 与分片响应，播放时间超过 0.25 秒。MSE 路径的测试仅覆写原生 HLS 能力返回，清单、分片及解码均为浏览器真实行为。

| 样本 | 播放方式 | Manifest / 分片 HTTP | 打开影片至检测到时间推进 | 流请求总数 |
| --- | --- | --- | ---: | ---: |
| Paws · 434000 | Edge 原生 HLS | 206 / 206 | 9.74 s | 17 |
| Paws · 434000 | HLS.js / MSE | 200 / 200 | 3.03 s | 6 |
| Stray · 1332010 | Edge 原生 HLS | 206 / 206 | 16.29 s | 12 |
| Stray · 1332010 | HLS.js / MSE | 200 / 200 | 4.05 s | 7 |

页面载入及打开图片画廊时的流请求均为 0。打开单部影片后，点击播放前观察到 2–4 次该影片的播放准备请求，video 保持 paused 且时间为 0。关闭后 video 暂停、移除 src、退出 DOM，位置保持稳定，焦点回到原影片卡。MSE 测试记录实例所属 Blob URL 已被释放。上述耗时是单次本机到真实 Steam CDN 的观测。

独立 INTL 发布夹具通过英文 SSR、中文媒体回退与中文 SSR。CN 与 INTL 的 API 分别返回正确区域与 SFW 响应头，跨区请求为 404。INTL 模拟 CDN 请求失败后仍能读到 Profile，错误留在影片 Dialog。此处的 INTL 夹具验证区域边界，并未用于判断 INTL 地区的实际 Steam CDN 可达性。

## 专项及原生检查

- 公开媒体 Playwright 9 项通过，使用合成 Profile 响应，覆盖 v2 优先级、旧版及空 DTO、仅图或仅视频、跨文档 CSP 与返回目录的滚动恢复、无脚本入口、语言回退、图片错误、排序、字符串长 ID、影片名纯文本渲染、同 URL 不同角色、390px Dark 与 1280px Light、方向键、Escape、焦点还原及 reduced-motion。
- 本机生成的 MP4/WebM 测试文件通过 Edge 真解码。独立模拟验证直接来源失败后转 HLS、CORS 失败、DASH-only、封面但无播放源、禁止外域 HLS 子请求、切换释放旧影片和提前关闭时取消过期的 HLS 动态导入。
- 原生隔离源码的 task check 通过。Go 检查、Web 构建和类型检查通过。通用 Playwright 套件 87 项首次通过，1 项现有 Start Widget 用例重试通过，3 项按套件配置跳过。只在隔离副本中将 Playwright 并发设为 2 并启用一次重试，保留原有断言。原工作树的既有忽略文件 services/api/.local/ops-data-60/diagnose.go 触发工具链检查失败，该文件及其他本地资料均保留。task check:hmr 通过 2 项。
- 复用隔离库已有 Schema。本次未迁移、未写入共享开发库，也未重新采集、修改正式快照或连接公开生产环境。

专项在 Web 构建后运行。

    $env:PLAYWRIGHT_CHANNEL = 'msedge'
    pnpm --filter @gofurry/web exec playwright test --config playwright.media.config.ts

## 构建与加载数据

| 本机构建或浏览器采样 | 体积或行为 |
| --- | --- |
| 初次 SSR 页面加载的 JS 资源 | 449,055 B，按 Edge 的 encodedBodySize 汇总 |
| Gallery 动态 JS | 17,119 B，gzip 5,697 B |
| hls.js light 动态 JS | 331,491 B，gzip 103,574 B |
| 图片 | 卡片图片原生懒加载。截图卡只请求缩略图，打开后才请求原图；缺失或失败时尝试另一 URL |
| 影片 | 打开画廊前不加载 Gallery JS。普通画廊不加载 HLS 代码。原生 HLS 路径不加载 HLS.js |

gzip 数值来自构建文件的本地压缩测量，与页面传输大小分别记录。播放请求数统计至 Dialog 关闭，包含浏览器自身的范围请求和清单处理。

## 验收范围

真实播放覆盖 Windows Edge。其他浏览器、移动设备的实际编解码器与其他地区 CDN 尚未实播。DASH-only 保留封面和明确状态。公开 CDN 的暂时故障不会使游戏 Profile 无法访问。R1 的公开体验封板归 R1-F3。
