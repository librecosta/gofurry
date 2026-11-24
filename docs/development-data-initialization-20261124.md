# 开发数据初始化 · 2026-11-24

**结论：PARTIAL。** 开发库迁移和恢复验证通过，开发服务已上线本机，Admin 可读真实业务数据。Details v3 覆盖全部有效游戏。Steam Web API 当日额度已用完，另有上游、Attempt 和来源诊断遗留，逐项记录在后文。

## 环境与迁移

在 Windows 原生环境中构建并启动 Harvest `526a7b3`、GoFurry `8089f12` 和 Admin `7a0a096` 的 `develop`。GoFurry 使用含停用态计划命令的 `3410027` 启动。

| 服务 | 已核实目标 | Goose 迁移前 | Goose 迁移后 |
| --- | --- | ---: | ---: |
| Harvest | 开发 VPN PostgreSQL 18.6，`harvest_dev`，端口 5432 | 14 | 15 |
| GoFurry CN | 同一开发实例，独立的 `gofurry_cn_dev` | 202611240002 | 202611240004 |
| GoFurry INTL | 只发现隔离测试配置，未找到独立的共享开发配置 | — | — |
| Admin | 同一开发实例，独立的 `librecosta_admin_dev` | 202611050002 | 202611050002，无待应用迁移 |

迁移前对应库没有其他客户端会话。使用 `pg_dump --format=custom` 分别保存了 440,500 和 536,863 字节的一致性归档，核验归档目录及 SHA-256，并在独立的本机 PostgreSQL 18 数据库逐一完整恢复。恢复库与开发库的核心表行数及摘要一致。先在恢复库执行并验证 Goose Up，再使用开发 YAML 显式执行两套迁移。迁移后 Harvest 的 98 个 Target、159 个 Schedule、393 个历史 Job、404 个 Attempt、1,288 条 Observation、4 份证据及其 Attribution 均通过迁移前后校验。GoFurry 的 31 个游戏、62 条正式发布快照、31 个站点、158 个已存在的 Plan、414 条当前投影、124 条旧版 Steam 来源资料和两条旧版报价也通过校验。报价的迁移校验按原有列计算，新版本增加确认时间及 Output Schema。

备份、恢复记录、运行副本 YAML 和逐条执行台账保存在各自已忽略的本地位置。新建任务目录的 Windows ACL 限定为当前账户、SYSTEM 和管理员。正式启用采集前依次启动 Harvest Server、Worker、GoFurry CN API/Consumer、Admin Embed 和 Web。Harvest、GoFurry CN 与 Admin 的 `/readyz` 返回 200，GoFurry Web 的 `/games` 返回 200。

## 盘点与执行方式

| 项目 | 数量 |
| --- | ---: |
| 游戏总数 / 有效 Steam 游戏 / 去重 AppID | 31 / 31 / 31 |
| 站点总数 / 有启用采集计划的站点 | 31 / 31 |
| 当前有效 Collection Binding / 已关闭 Binding | 93 / 2 |
| 现有 Collection Plan / 缺失并新增的 Plan | 158 / 217 |
| 现有站点任务 HTTP / DNS / TLS | 31 / 31 / 31 |
| 原有暂停站点 Plan，保留原状 | 3 |
| Steam 一次性 Scope | 217 |
| DEV-DATA-INIT 计划执行 Scope 合计 | 310 |

31 个已登记 Steam Target 各有 7 个本轮 Scope。每个 AppID 执行 Details v3 和 Assets v1 的 EN/US、ZH-CN/CN，Events v1 的 EN、ZH-CN，以及 Players v1 的 CN 出口采样。数据中的实际执行出口单独核对为 `cn`。Events 和 Players 的请求契约没有 Store Country。原有市场计划只有 US 和 CN，此次没有配置 HK 任务，也没有有效的旧 News 简报 Plan。排除两个未匹配到 GoFurry 业务绑定的 Harvest Steam 测试 Target、两个已关闭的备用站点绑定和三个原有暂停站点计划。三种计数分别按 Target、Binding、Plan 统计。

在开发库唯一的 `public` Namespace 授权 v3、Assets 和 Events，使用现有 Operator、Writer、Reader、Worker 及已登记的 Consumer。通过 Revision 审核的 Network Policy 变更复用原出口、Permit 和配额。Events 使用原 AppDetails Storefront 桶。Assets 使用 Steam Web API 的共享账户桶，每天最多 100 次。Worker 并发保持原来的 2。通过现有 `collection plan` CLI 的新增 `--disabled` 选项原子写入归属和停用态。GoFurry 保留有效来源关联，用 Harvest 的 Writer 以幂等键提交一次性 Job。217 个新 Plan 均无远端 Schedule ID。核对原有 158 个 GoFurry 计划绑定和 159 个 Harvest Schedule 的标识、启停状态及执行版本，均与迁移基线一致。GoFurry 运行副本暂停 Intake，避免本次运行间接新增游戏。原始私有配置继续保留。本轮未开启新的 Raw Evidence 捕获，现存 4 份原始证据保留。执行验证以持久化的结构化 Observation 和 Attempt 为准。

站点任务使用启动后现有启用 Schedule 的这一轮结果。对超出执行窗口、临时传输故障等可恢复情况，通过已有 Operator Retry 接口逐条补跑。Steam 新任务按两类请求配额低并发执行。正常周期中的旧版 Details v1 结果列在独立统计中。

## 开发服务与恢复入口

- Harvest Server 与 Worker 继续使用 GoFurry 工作树内 `.local/dev-data-init-20261124/harvest-server.yaml` 和 `harvest-worker.yaml` 两份本次运行配置。Server 以 `serve --local --config` 启动，Worker 以 `worker --config` 启动。原 Harvest 私有 YAML 仍保留。如果重启，从这两份运行配置恢复开发 Namespace、Worker Offer 和已有的 Policy Revision。
- GoFurry CN API 与 Web 使用同一任务目录中的 `gofurry-cn.yaml` 和 `gofurry-web.yaml`。Admin 使用现有开发 Embed 配置。Admin、Web、Harvest 和 GoFurry CN API 分别监听本机 8080、17320、18080、17321 端口。Management 仍为本机私有连接。
- `prepared.ndjson` 和 `reconciliation-report.json` 在任务目录记录全部 Scope 和最后核验结果。共享 Web API 配额恢复后，从 GoFurry 工作树按原配置运行 `python .local/dev-data-init-20261124/collect.py official`，复用原幂等键并跳过已终结的 Job。已结束的暂态失败可在原配额与出口策略下通过 Harvest Operator 单独补跑。Players 补跑产生新采样时间。
- 保持 217 个新业务绑定停用，继续沿用新增前的 158 个启用绑定。运行中的旧 Schedule 延续原有节奏。清单中的新数据不改变既有编辑稿和发布状态。

## 原生回归

- Windows 上构建了 Harvest、GoFurry API、Admin Embed 和 GoFurry Web。GoFurry 原生 `task check:api`（vet、测试和编译）与 `task sql:check` 通过。
- 在隔离恢复库运行真实 CLI，确认 `--disabled` 创建停用态 Plan，随后对同一 Plan 重放原命令仍保留同一绑定及停用状态。已有入口的启用默认值未变。
- 使用当前开发账号，在真实 Edge 中经过 Admin 登录、嵌入式 Web、私有 Management 网关读取开发库，核验了 31 个游戏的详情、公告、事实、变化、素材候选和采集入口。这六类 GET 全部返回 200。逐游戏读取限速后符合 Management 的每分钟 120 次限制。代表游戏的素材、公告、采集事实和采集标签均已打开。返回 6 种 Detector。
- Harvest 原生 config、execution、provider、worker 测试通过。GoFurry Web TypeScript 和单独启动的 Edge 媒体回归 10/10 通过。默认 Web 检查的 Playwright 预检因开发 SSR 正占用 17320 端口而未执行其整套用例。隔离媒体套件使用独立端口。
- Admin 核对 AppID `762610` 的来源正文、4,185 字节的安全公告 HTML，以及 CN、US 两个报价市场。AppID `1567440` 有真实有效的玩家日样本。站点 Job `01a20d0e-1ddb-709e-af53-07e0e43b2161` 在采集详情中显示一个 Attempt 和八条结果。也复核了原轮次的 `deadline_exceeded` 失败详情。
- 三个代表游戏的公开 Profile API 和游戏详情 SSR 均返回 200。公开接口可读归一化 Steam 事实，正式发布文案和媒体仍由原快照决定。

## 实际采集结果

统计截至 2026-11-24 18:52:48（北京时间）。以 Harvest 持久化的 Job / Attempt 和当前 Consumer 世代为准。每次 Retry 是独立 Job，同一业务 Scope 只计算一次覆盖率。

| 指标 | 实际结果 |
| --- | --- |
| 本轮进入队列的 Job，包含补跑 | 374。成功 247，部分成功 9，失败 118，仍在运行或等待 0 |
| Steam 一次性 Scope | 成功 162，部分成功 0，失败 38，配额待恢复 17，其他待处理 0 |
| 站点 Scope，含补跑后的最终结果 | 成功 85，部分成功 8，失败 0 |
| 原有 Details v1 周期任务 | 成功 29，部分成功 0，失败 33，单独统计 |
| 新增有效 Observation，归属本轮 Job | 493 条，其中 493 条的位置已通过 Consumer ACK，493 条按 Observation ID 匹配 GoFurry 当前投影或玩家日样本，当前投影还核对 Content Hash |
| 当前 Scope 的再次确认 | 799 个当前结果在本轮窗口内获确认，且 Observation 已存在于 GoFurry 当前投影。此数含新增及同内容确认 |
| Consumer | `public` ACK 427 → 948，推进 521 个 Stream 位置，Stream 最新 948。Reset Required=false，ACK Pending=false，记录的缺口 0 |
| Steam Web API 实际配额 | 100/100 个 Permit，已记录成功 54、失败 44、拒绝 0 |

共享 Steam Web API 配额已用完。当前 UTC 额度到 2026-11-25 00:00 UTC（北京时间 08:00）恢复。尚未提交的 Assets 和 Players Scope 保留在本地台账，应用没有提前排入会过期的队列。

| Capability | 已盘点 Scope | 成功 / 部分成功 / 失败 / 配额待恢复 | 有效投影 Scope | 涉及 AppID |
| --- | ---: | ---: | ---: | ---: |
| `steam.details` | 62 | 62 / 0 / 0 / 0 | 62/62（100.0%） | 31/31（100.0%） |
| `steam.assets` | 62 | 43 / 0 / 8 / 11 | 43/62（69.4%） | 23/31（74.2%） |
| `steam.events` | 62 | 46 / 0 / 16 / 0 | 44/62（71.0%） | 22/31（71.0%） |
| `steam.players` | 31 | 11 / 0 / 14 / 6 | 11/31（35.5%） | 11/31（35.5%） |

| 站点 Capability | 已执行 Scope | 成功 / 部分成功 / 失败 | 窗口内有效且匹配 GoFurry 当前投影 |
| --- | ---: | ---: | ---: |
| `web.http` | 31 | 23 / 8 / 0 | 31 |
| `web.dns` | 31 | 31 / 0 / 0 | 31 |
| `web.tls` | 31 | 31 / 0 / 0 | 31 |

- 正文持久化覆盖 31/31（100.0%），AppDetails 结构化媒体覆盖 31/31（100.0%），StoreBrowse 实际返回素材覆盖 23/31（74.2%）。容量裁剪与来源缺失由 Observation 保存，未推断缺失素材。
- 已核对结构化影片 62/62 个媒体 Scope，截图 62/62 个。有效 StoreBrowse Scope 共 537 个引用，合计 0 个媒体或素材 Scope 报告裁剪。
- 本轮新增 103 个 Store Events 事件记录，覆盖 22 个 AppID，103 个有原始正文和安全 HTML。上游明确返回空列表的 Scope 有 0 个。GoFurry 当前版本中，中文请求保留了 18 条 Steam 声明为中文的文章和 67 条 Steam 声明为英文的文章，回退内容继续标为英文。
- v3 报价覆盖 CN 31/31、US 31/31、HK 0/31（没有已配置的 Scope）。玩家 UTC 当日采样覆盖 11/31。US/en 归一化发行与语言分别覆盖 31/31 和 31/31。
- R4 已写入 341 条当日来源事实，31 个绑定新增首次观察可用时间，可信 First Available 审核记录为 0。六个 Detector 可读。已关闭日待处理 0，未关闭日及相邻日队列 62。Checkpoint 0，变化 0。采集发生在同一未关闭 UTC Day。
- 迁移前存在的 1288 条 Harvest Observation 中缺失 0 条。游戏及站点身份、游戏文案、所有 Draft 媒体和完整详情、正式发布快照的摘要均与迁移基线一致。从 Admin 读取了结果，未进行素材采用、人工文案修改或发布。

### Job 与来源样本

| AppID | Scope / 段 | Job | Attempt | Observation | Content Hash |
| --- | --- | --- | --- | --- | --- |
| 434000 | en/US/cn / rich_details | `01a20d14-7167-7156-bf03-79c4a8ff6148` | `01a20d14-7471-706a-9bee-a5974f114e1f` | `01a20d14-8232-7d03-bd9e-6390f6d9840d` | `dc13554f622ce9a241b95d747731a54f92b166be993a68e0f3f4aa84a883b9e9` |
| 434000 | zh-cn/CN/cn / rich_details | `01a20d14-8bdf-7505-8559-12d4d7ef411c` | `01a20d14-962e-7845-ac86-939ef5ce204e` | `01a20d14-9e57-733d-8f12-d723a335cafc` | `7ecce9a3b1038aa0557aa565ef058fa0d622b25f52bbe22351182b851edf64f9` |
| 762610 | en/cn / events | `01a20d15-8a9f-7a30-80aa-4c651a185390` | `01a20d15-8dc5-78b1-94e1-643ad442a850` | `01a20d15-93eb-7a32-b384-c5e8de5b52ab` | `30b8d97ce5ac16968efb9145f6115b7d547a9e36ac9f3622cfc9dc51ab703e5e` |
| 762610 | zh-cn/cn / events | `01a20d15-9f32-71db-aa4d-5b1bad755505` | `01a20d15-a157-73e4-bf86-c1eee9f0daa2` | `01a20d15-a49b-7f5c-b170-9343b7c44dc5` | `3e1b56fa1631b36afd06e0d2a4b456adc7819d9f8172606abd746f1de0a583e7` |
| 1567440 | zh-cn/CN/cn / assets | `01a20d18-fd85-79ff-b92b-a520ca5a2b5b` | `01a20d19-3968-7c43-8fd5-7714edbbd246` | `01a20d19-4763-7081-9529-94e8a735e2ea` | `621bae2fc3a03eac8c349f4e2098ed26f16e2df9f12179220a1ba03383580f9a` |
| 1567440 | cn / players | `01a20d19-5397-71a0-99b7-4390a5f2b2f9` | `01a20d19-5b34-7564-831b-299a0591c0d9` | `01a20d19-6338-7730-998f-ca9a3a0f3bd8` | `b4766eb661bfed1ddf15a9d5a2b9b5e15d337d4340f8e6cf804b81022c0d3d0f` |

### 未完成及部分成功的 Scope

失败码 `worker_lost` 表示 Harvest 回收了已到期且没有接受 Completion 的 Attempt。已按原配额补跑暂态失败。无有效正文的事件页及验证失败的来源均继续保留诊断。

Steam 表按 AppID 和实际请求 Scope 逐项列出。Job 查阅 Harvest Operator；无 Job 的 Scope 可复用停用态 Plan，等共享额度恢复后按原配置显式提交。源失败和诊断保留 Last Known Good。合法空公告已算作已验证结果。

| AppID | Capability | Locale / Market / Egress | 最终状态及原因 | Job |
| --- | --- | --- | --- | --- |
| 290340 | `steam.assets` | en/US/cn | `failed` `transport_error` | `01a20d11-6c74-7a73-8593-d5205d7ccd13` |
| 290340 | `steam.assets` | zh-cn/CN/cn | `failed` `transport_error` | `01a20d13-46e2-785f-86eb-b503c99a8383` |
| 290340 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d25-7150-75df-ba2f-ebaa9799bc77` |
| 290340 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d26-2c71-74c3-9fc4-5900e60eb5b2` |
| 290340 | `steam.players` | cn | `failed` `worker_lost` | `01a20d13-84e7-7917-ad21-522e2570287d` |
| 434000 | `steam.assets` | en/US/cn | `failed` `transport_error` | `01a20d13-a674-720b-b550-687ec20fcd55` |
| 434000 | `steam.assets` | zh-cn/CN/cn | `failed` `transport_error` | `01a20d13-e477-7d34-8ada-fae8ad6c94dd` |
| 434000 | `steam.events` | en/cn | `succeeded` `projection_or_source_unverified:identity_invalid` | `01a20d14-ac00-7c82-a3a3-beaf11a1ac83` |
| 434000 | `steam.events` | zh-cn/cn | `succeeded` `projection_or_source_unverified:identity_invalid` | `01a20d14-c0c2-7729-8cea-d514d556d2a5` |
| 434000 | `steam.players` | cn | `failed` `transport_error` | `01a20d14-20f5-7e72-9576-a7bc06efd286` |
| 702670 | `steam.assets` | en/US/cn | `failed` `transport_error` | `01a20d14-35cf-700c-8788-73bbb450d14c` |
| 702670 | `steam.assets` | zh-cn/CN/cn | `failed` `transport_error` | `01a20d14-6ccc-72bc-87de-5ba71a69be4a` |
| 702670 | `steam.players` | cn | `failed` `transport_error` | `01a20d14-bb68-74af-8102-5cdad3eb8120` |
| 762610 | `steam.assets` | en/US/cn | `failed` `transport_error` | `01a20d14-d5a7-7d0a-9601-8bc34f213e52` |
| 762610 | `steam.players` | cn | `failed` `transport_error` | `01a20d15-6885-749a-878d-4d63c91ef5c8` |
| 843380 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d26-e1f5-72ff-abdd-8d24bdc07ae6` |
| 843380 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d27-964c-7b42-9c08-53851f298433` |
| 843380 | `steam.players` | cn | `failed` `worker_lost` | `01a20d15-a69d-7f2d-bea8-4f3868c5b99f` |
| 909820 | `steam.players` | cn | `failed` `worker_lost` | `01a20d16-2f29-7ae7-9b28-19719d8917b5` |
| 949480 | `steam.players` | cn | `failed` `timeout` | `01a20d16-992e-736c-8a71-be4e65ad36aa` |
| 965580 | `steam.events` | en/cn | `failed` `source_invalid` | `01a20d28-7968-7049-b3c9-6bde486f217c` |
| 965580 | `steam.events` | zh-cn/cn | `failed` `source_invalid` | `01a20d1a-5236-7819-9465-f8c4d546f412` |
| 965580 | `steam.players` | cn | `failed` `timeout` | `01a20d17-116e-7e65-95ad-b38500e984af` |
| 1260320 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d28-8e19-77a3-b6c4-b01078160fce` |
| 1260320 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d29-41b0-75e2-9920-2d67e83e6f71` |
| 1260320 | `steam.players` | cn | `failed` `transport_error` | `01a20d17-7657-74be-b70e-1babaed28446` |
| 1269640 | `steam.players` | cn | `failed` `worker_lost` | `01a20d17-c0ef-7f9c-8a99-cd45c47b8a01` |
| 1376910 | `steam.assets` | en/US/cn | `quota_wait` `daily_quota_exhausted` | — |
| 1376910 | `steam.players` | cn | `failed` `deadline_exceeded` | `01a20d18-41d7-7a4f-907a-9c0c690fd26a` |
| 1567440 | `steam.assets` | en/US/cn | `failed` `transport_error` | `01a20d18-61f9-730e-8167-9d61fe939a67` |
| 1586800 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d29-f225-7459-ab69-0cf37a73a219` |
| 1586800 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d2a-a6f0-702f-9e73-c303a19ef9cb` |
| 1631470 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d2b-55a4-7fac-bea0-8ae31c4d85e2` |
| 1631470 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d2c-04c2-7d29-8d26-45373b4fcb5d` |
| 1660960 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d2c-b9af-73a5-8211-1225579459e5` |
| 1660960 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d2d-6f59-7aa3-82c7-01dea6f79f74` |
| 1996470 | `steam.players` | cn | `failed` `worker_lost` | `01a20d1c-7492-7b16-8aea-e670e5867b0b` |
| 1999520 | `steam.players` | cn | `failed` `worker_lost` | `01a20d1c-da83-7127-bd91-6b27fedb3847` |
| 2102040 | `steam.players` | cn | `failed` `timeout` | `01a20d1d-2dbb-7193-be37-d2ab04fae5a6` |
| 2217000 | `steam.events` | en/cn | `failed` `worker_lost` | `01a20d2e-1e8a-7ca7-a0b7-8ac6d0ffb446` |
| 2217000 | `steam.events` | zh-cn/cn | `failed` `worker_lost` | `01a20d2e-dd2f-7e62-83ab-8a143f4d2701` |
| 2394650 | `steam.players` | cn | `quota_wait` `daily_quota_exhausted` | — |
| 2797340 | `steam.assets` | en/US/cn | `quota_wait` `daily_quota_exhausted` | — |
| 2797340 | `steam.assets` | zh-cn/CN/cn | `quota_wait` `daily_quota_exhausted` | — |
| 2797340 | `steam.players` | cn | `quota_wait` `daily_quota_exhausted` | — |
| 2870350 | `steam.assets` | en/US/cn | `quota_wait` `daily_quota_exhausted` | — |
| 2870350 | `steam.assets` | zh-cn/CN/cn | `quota_wait` `daily_quota_exhausted` | — |
| 2870350 | `steam.players` | cn | `quota_wait` `daily_quota_exhausted` | — |
| 3084280 | `steam.assets` | en/US/cn | `quota_wait` `daily_quota_exhausted` | — |
| 3084280 | `steam.assets` | zh-cn/CN/cn | `quota_wait` `daily_quota_exhausted` | — |
| 3084280 | `steam.players` | cn | `quota_wait` `daily_quota_exhausted` | — |
| 3351430 | `steam.assets` | en/US/cn | `quota_wait` `daily_quota_exhausted` | — |
| 3351430 | `steam.assets` | zh-cn/CN/cn | `quota_wait` `daily_quota_exhausted` | — |
| 3351430 | `steam.players` | cn | `quota_wait` `daily_quota_exhausted` | — |
| 3421300 | `steam.assets` | en/US/cn | `quota_wait` `daily_quota_exhausted` | — |
| 3421300 | `steam.assets` | zh-cn/CN/cn | `quota_wait` `daily_quota_exhausted` | — |
| 3421300 | `steam.players` | cn | `quota_wait` `daily_quota_exhausted` | — |

| 站点 Public ID | Scope | 最终状态及原因 | Job |
| --- | --- | --- | --- |
| `d21hd0b7a0c2wjd2` | `web.dns`/cn | `skipped` `preexisting_paused_plan` | — |
| `d21hd0b7a0c2wjd2` | `web.http`/cn | `skipped` `preexisting_paused_plan` | — |
| `d21hd0b7a0c2wjd2` | `web.tls`/cn | `skipped` `preexisting_paused_plan` | — |
| `x4yjt3620d0h3zp8` | `web.http`/cn | `partial` `incomplete` | `01a20d0e-1d56-79e9-814b-f1abb13713ea` |
| `0zz96mqgh9j1pf7p` | `web.http`/cn | `partial` `incomplete` | `01a20d15-2702-77e8-a967-05f90818bb28` |
| `25xp2qy9w75bpd8p` | `web.http`/cn | `partial` `incomplete` | `01a20d19-968b-715c-930d-2b489ac0f76f` |
| `0s4ax186mw7chsze` | `web.http`/cn | `partial` `upstream_auth` | `01a20d1a-bf54-7f42-84f8-506895e7481d` |
| `fr6fz5gwy9fxpvey` | `web.http`/cn | `partial` `incomplete` | `01a20d1a-dd26-7b5f-b570-386633ef8424` |
| `d31vr7chbd992s1w` | `web.http`/cn | `partial` `incomplete` | `01a20d1e-38f3-7a78-b9cc-e7d006368bcb` |
| `k01nf5x20f1bzp3d` | `web.http`/cn | `partial` `incomplete` | `01a20d1f-0d88-7e9a-becd-21d54c521fee` |
| `vxsgd7e6jeg3rq82` | `web.http`/cn | `partial` `incomplete` | `01a20d1f-bf20-725c-aba8-98cfb5d301da` |

未完成的网络任务只在既有策略允许、源故障消失后重试。上游拒绝、来源本身缺失和已暂停的站点计划保留现状。新 Steam 单次 Job 没有 Schedule ID，GoFurry Admin 采集历史按 Schedule 归属。对应 Job、Attempt、Observation 和失败码均可从 Harvest Operator 和本地台账逐条复核。
