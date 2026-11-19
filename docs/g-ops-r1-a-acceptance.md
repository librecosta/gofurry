# G-OPS-R1-A 验收

## 删除的机制与入口

- 删除 Steam 关键词、ESRB/PEGI、成人标签和 AI Gateway 驱动的资格判断，以及 Discovery AI 配置、预算和对应测试。Steam 结构化产品类型继续区分 Game、DLC、Demo。原始分级和年龄信息作为来源事实展示给运营人员。
- 删除 Intake 自动发布、资格审核证明与到期撤销机制，删除 CLI 的 `review`、`revoke-review`、`relate`、`revoke-relation` 及 Game 发布入口。D5 导入建立 Draft。Discovery、采集、去重、重试和周期计划继续运行。未解决的跨平台归属保留在候选中，已确认的来源归属继续使用原 GameSource。

## 数据结构

- Goose `202611200001_game_publication_authority` 新增 `games.content_rating`，默认 `unreviewed`。公开视图要求 `published`、`sfw` 和有效快照。
- `game_published_localizations` 按 Game 和 Locale 保存人工确认的名称、简介、媒体、来源归属、Revision 和发布时间。`game_source_text_history` 保留原展示资料及其采集归属，`game_localizations` 继续保存编辑稿，Intake Profile 和 `game_source_metadata` 继续存放来源事实。公开价格、玩家和新闻仍从各自的采集投影读取。公开编辑前需撤回到 Draft。
- `catalog_publication_decisions` 增加内容分级与 Management Request ID。旧决定和回执继续保留。迁移移除旧展示函数、资格证明和资格相关字段、AI 预算表、旧展示表、`intake_managed`、`publication_blocked`，并重建公开目录索引。

## 数据校验

- 迁移前，`gofurry_cn_dev`（PostgreSQL 18.6）有 1 个 Draft、0 个 Published、0 个 Archived。唯一 Game 来自一次人工提交的 Steam Intake。Game 发布决定、Admin 对应的发布操作回执、旧展示和公开限制记录均为 0。迁移后为 1 个 Draft（`unreviewed`）、0 个 Published、0 个 Archived，公开视图及快照均为 0。
- 已将开发库备份还原到独立 PostgreSQL 18，执行 Goose 并比对数据。正式切换前生成备份，检查归档清单，停止相关开发进程，确认目标库无其他客户端连接，再执行迁移并恢复 GoFurry 与 Admin。备份 SHA-256 为 `1926537177038C69080C15D01AF3BBA0F6DC9AF73D55C22764E2DE8E02E73017`。
- BROK（Steam `949480`）的 Public ID `j2byfcwgr8cqnkt4`、Game UUIDv7 `01a1f44a-c88e-76a2-92b5-a7a3a81304c0`、Revision 1、Intake、GameSource、Target、Job、Attempt、Observation、Collection 绑定、计划及 Schedule 均保留。迁移前后身份、关系及保留列的摘要一致。迁移后重复提交复用了原候选与 Harvest Job，公开详情返回 404。
- 隔离迁移测试覆盖旧 Intake 发布、缺少可核验发布归属的记录及受限公开记录退回 Draft，保存原发布历史、来源文本和归属，保留既有 Archived。旧发布链路欠缺可核验的审核内容版本与人工分级，迁移审计记录了撤回原因。开发库没有需要回填人工快照的历史游戏。

## 发布权威

- Admin 会话的 Owner 或 Operator 通过 GoFurry Management 审核并发布。GoFurry 验证 Admin 服务与操作者断言，校验 Revision，重新计算预览标识，并检查当前状态、展示名称、收录范围确认和 SFW 分级。目录决定、公开快照和操作回执在同一事务写入，Admin 审计可通过 Request ID 核对。
- 两个区域使用相同的 SFW 公开策略。开发人员角色及缺少认证的请求被拒绝。发布页面显示拟公开文案、媒体和来源。Published 可撤回至 Draft 或归档，Draft 可归档，Archived 恢复至 Draft。来源采集和 Observation 重放保持状态、人工分级和公开快照。

## 实跑验证

| 环境 | 验证 | 结果 |
|---|---|---|
| GoFurry | `task check:api`、Web 类型检查与 SSR 构建、sqlc generate / diff / vet | 通过 |
| GoFurry Web | `task check:web` | 88 个浏览器用例通过，3 个需显式启用的实时天气用例按默认配置跳过 |
| GoFurry 隔离 PostgreSQL 18 / Redis | `task check:catalog`、`task check:legacy-import`、Goose 从旧 Schema 升级 | 通过，含 Site、Navigation、Yearbook、标签、合集及 29 个浏览器用例 |
| GoFurry / Harvest 隔离链路 | `task check:collection`，运行固定版本的 Harvest 服务和 Worker，Steam 上游使用受控响应 | 通过，含 Discovery、Draft 建档、去重、计划、Consumer 重放和快照隔离 |
| Admin | `task check:api`、`task check:web`、`task check:embed`、sqlc generate / diff / vet | 通过 |
| Admin + GoFurry 隔离链路 | `task check:gofurry:operations` | 8 个浏览器用例通过，含 Owner 审核发布、Operator 权限、Developer 拒绝、旧 Revision 和区域策略 |
| 开发库及本地服务 | 实库备份、还原演练、迁移、BROK 对比、GoFurry API / SSR 与 Admin 健康检查、旧 CLI 拒绝 | 通过 |

## 后续阶段

- R1-B 完善编辑稿、已发布快照、媒体选择和编辑体验。
- R1-C 完成运营发布的全流程验收及交互打磨。

## 实现提交

- GoFurry `78188f557ef015dddb2f7ad421cf002899fc4797`
- Admin `68b4c9fd42c7f6c745f244c6fd0e9ed166ffc6cb`
