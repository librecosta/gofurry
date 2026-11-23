# STEAM-R1-F1 · Published Media Projection

## 公开契约

`GET /api/v1/games/{public_id}?profile=1&locale=zh` 或 `locale=en` 可以返回顶层可选字段 `published_media`。它由指定区域的 PostgreSQL 正式快照 `game_published_localizations.media_v2` 投影，沿用既有 `ReadGamePresentation` 查询及 Published/SFW 条件。

| 字段 | 契约 |
| --- | --- |
| `version` | 数字 `2` |
| `locale` / `fallback` | 实际媒体快照语言 `zh` 或 `en`，以及是否从请求语言回退 |
| `cover` | 可选，包含 `asset_type`、`asset_family`、`url` |
| `images` | 有序数组，元素字段与 `cover` 相同。每个角色最多一个，2x 同系列角色独立。两个角色可共用 URL |
| `screenshots` | 有序数组，最多 8 项。每项包含字符串 `screenshot_id`，以及可选的 `thumbnail_url` 和 `full_url`，至少保留一个有效 URL |
| `movies` | 有序数组，最多 4 项。每项包含字符串 `movie_id`、`name`、可选的 `thumbnail_url` 和 `highlight`，以及 `sources` 对象 |
| `movies[].sources` | 可选键 `mp4_480`、`mp4_max`、`webm_480`、`webm_max`、`hls_h264`、`dash_h264`、`dash_av1`，每个值是经验证的 URL |

数组位置就是正式采用顺序。客户端保持影片和截图 ID 为字符串，使用条目位置保留同一快照中不同来源的条目。角色取自正式快照内已采用对象的角色，适用 Store、Library 和 Community。三类数组在 DTO 存在时始终是数组，可为空。可选字段省略时表示没有可公开的值，`highlight=false` 与未提供保持区分。若影片的全部播放来源被过滤，仍保留可公开的影片元信息，`sources` 为空对象。R1-F2 应将其视为不可播放，并把名称当作纯文本显示。

## 快照与回退

查询首先选择与请求语言匹配的已发布 v2 快照。若该语言没有 v2 快照，使用另一个已发布语言的 v2 快照并设置 `fallback: true`。整份选择保持原样，图片、截图与影片都来自同一份语言快照。若没有已发布 v2，或选中的快照过滤后无任何公开对象，省略 `published_media`。

投影按 Cover、Images、Screenshots、Movies 读取全部采用对象。跨 GameSource 和 Observation 的已采用对象保留在同一 DTO，来源 ID 继续只读于持久化快照和私有 Management。公开 DTO 只列出媒体角色、稳定媒体 ID、名称、标记和 URL。它不包含 Origin、内部 UUID、观察或编辑回执、Scope、Namespace、Hash、Ruleset、凭据或原始响应。

公开投影随 Publish 或 Publish Changes 更新。草稿保存、当前候选更新、Consumer Reset 与来源撤回均走各自原有路径。现有 `cover_url`、`sources[].media`、新闻、报价、玩家数、关联游戏、公开 ID、Slug 和不带 `profile=1` 的详情契约仍由原有代码生成。兼容 `movies: string[]` 继续只提供直接媒体文件。R1-F2 可优先读取 `published_media`，旧客户端和只有旧媒体的游戏继续使用既有字段。

## URL 和体积

- URL 必须是有界 HTTPS 引用，原始与序列化形式均最多 2,048 字节。拒绝凭据、Query、Fragment、非默认端口、非法编码和危险控制字符。
- Steam CDN 允许 `.steamstatic.com`、`.steamcontent.com` 的有边界子域以及既有两个 Akamai 官方主机。StoreBrowse 另两个精确镜像主机 `shared.st.dl.eccdnx.com` 和 `shared.cdn.steamchina.queniuam.com` 仅允许静态图片的 Steam App 资源路径。路径逐段解码校验，拒绝遍历、编码分隔符和重复编码，保留合法编码。
- 图片格式为 JPEG、PNG、WebP、AVIF 或 GIF。MP4、WebM、HLS、DASH 字段分别要求 `.mp4`、`.webm`、`.m3u8`、`.mpd`。逐字段过滤，其他有效来源继续返回。截图只有一条有效链接时保留该项。校验只读取字符串和快照。
- 公开投影核验数量、角色、ID、名称和 URL 长度，底层还有既有正式 JSONB 128 KiB 约束。见 [验收报告](steam-r1-f1-acceptance.md) 的近界限 HTTP 样本。
