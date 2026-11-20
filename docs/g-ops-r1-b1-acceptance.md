# G-OPS-R1-B1 验收

## 数据结构与迁移

- Goose `202611200002_game_editorial_draft` 为 `games` 增加 `draft_slug`。Published 保存 Slug 时暂存于此，发布更改才写入 `slug`。撤回或恢复至 Draft 时合入编辑路径。
- `game_localizations` 是唯一文案编辑稿，以 `text_source_id`、`text_observation_id` 记录未经改写的来源原文。`game_draft_media` 按作品及语言保存当前媒体选择、封面及来源归属。未提交媒体时保留已选内容，显式清空后预览保留空选择。
- `game_published_localizations` 继续保存公开快照，新增 `media_observation_id`。`catalog_publication_decisions` 增加 `publish_changes`，动态回退函数 `game_draft_text()` 已删除。
- 从 `gofurry_cn_dev` 只读备份还原独立 PostgreSQL 18 后执行 Goose。前后均为 1 个 Draft（`unreviewed`）、0 个 Published、0 个 Archived，公开快照为 0。BROK（Steam `949480`）从 0 条编辑语言变为 1 条带采集归属的资料。Public ID `j2byfcwgr8cqnkt4`、UUID `01a1f44a-c88e-76a2-92b5-a7a3a81304c0`、Revision、GameSource、Intake、Harvest Job、Attempt、Observation、绑定、计划和 Schedule 对比一致。第二次执行 Goose 无待应用迁移。开发库和生产库均未写入。
- 独立迁移测试用旧 Schema 构造了已有英文人工稿、缺失中文人工稿的 Published 游戏。迁移保留英文编辑稿，以原快照补入中文和媒体选择，未生成新的发布决定。原快照缺少媒体 Observation ID 的资料保留已知归属。

## 编辑与发布边界

- 首次 Intake 在创建 Game 的事务内写入有效首采文案。真实 Harvest Worker 与 GoFurry Consumer 的集成测试验证后续采集、Reset 和 Observation 重放保留人工文案，已删除的语言也保持为空。来源候选来自有归属的采集记录，保存时验证来源、Observation、语言与选择的媒体 URL。价格、新闻和玩家等来源事实继续由采集更新。
- Published 的编辑保存仅修改编辑稿和待发布 Slug。集成测试在新建的 Catalog Service 实例重读已保存编辑稿，同时验证公开 API 继续返回旧文案和封面。来源采用与媒体清空均需显式保存。
- Admin Owner 和 Operator 通过认证的 Management 入口审核发布。`publish` 用于 Draft，`publish-changes` 用于 SFW 的 Published。事务锁定 Game 并校验状态、Revision、主题确认及 SFW，从持久化编辑稿重算覆盖 Slug、文案、媒体顺序及来源归属的预览摘要。相同内容返回冲突，无新增 Catalog 发布决定或成功回执。
- 发布更改在同一事务内切换正式 Slug、快照、Revision、发布决定和 Management 回执。Admin 审计可按 Request ID 核对操作者。测试覆盖认证和角色拒绝、过期预览、重复回执触发整笔回滚、归档恢复至 Draft、公开身份和首次发布时间保留。
- SSR 测试使用真实 Management 发布。保存后旧路径仍返回原标题、Canonical、hreflang 和目录 Slug，待发布路径 301 到旧路径。发布更改后上述公开地址一并更新，旧地址 301 到新地址，Public ID 保持原值。

## 实跑验证

| 环境 | 验证 | 结果 |
|---|---|---|
| GoFurry API 与 SQL | `task check:api`、sqlc generate / diff / vet | 通过 |
| GoFurry API、SSR 与目录 | `task check:catalog` | 通过，含 PostgreSQL 18、Redis、29 个浏览器用例及公开路径切换 |
| GoFurry 采集与迁移 | `task check:collection`、`task check:legacy-import` | 通过，含旧 Schema 前向迁移和 Harvest Worker 链路。Steam 上游使用受控响应 |
| GoFurry Web | `task check:web` | 88 个用例通过，3 个需显式启用的实时天气用例按默认配置跳过 |
| Admin API、Web 与 Embed | `task check:api`、`task check:web`、`task check:embed`、sqlc generate / diff / vet | 通过 |
| Admin 通用浏览器检查 | `task check:web:browser` | 4 个默认用例通过，19 个需单独认证或启用产品夹具的用例按默认配置跳过 |
| Admin + GoFurry 隔离链路 | `task check:gofurry:operations` | 9 个浏览器用例及 10,000 条目录分页规模检查通过。覆盖 Operator 发布更改、Owner 审计核对、Developer 和匿名拒绝，保留首次审核发布、归档恢复和标签测试 |

## 提交与 CI

- GoFurry 实现提交 `f3f1b41a2f597190ac6d6a140288b71f5d5b4563`。[GoFurry Foundation checks](https://github.com/librecosta/gofurry/actions/runs/44709785381)
- Admin 实现提交 `cd3e252b62261894f6e352eb87f2942465827e99`，夹具补丁 `4de1fa941a130acf40a6f33ee232c7a4a6b5df21`。Admin CI 固定使用上述 GoFurry 实现提交。[Admin checks](https://github.com/librecosta/admin/actions/runs/44710412087)

## R1-B2

- B1 提供显式采用名称或简介、整组采用或清除已选媒体、查看当前快照以及审核发布更改。R1-B2 完成逐张选图、素材排序、完整来源选择器和编辑交互打磨。
