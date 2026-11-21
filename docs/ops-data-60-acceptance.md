# OPS-DATA-60 开发环境验收

## 数据来源和保全

- 使用 2026-11-15 封板 D5 Seed。逐文件验证 Seed 清单和入选 Logo 的 SHA-256。封板备份摘要为 `7ecb9fd2d31f5f4246ea8a814e222dc346130962c8a3376b83edabcb5a29b65a`。
- 写入前对 `gofurry_cn_dev` 建立 PostgreSQL 自定义格式备份，并在独立的 Windows PostgreSQL 18.6 实例成功完整恢复。备份 SHA-256 为 `f104e5007708ca8c0fd0d530e90a57c01089584a619ebda5e1f103547584272e`。开始时各有一条已发布记录，分别为 LibreCosta 和 BROK。
- 用旧站身份、主要网站 URL 和 Steam AppID 核对增量身份。旧站到新站的逐条映射保存在本机验收清单。通过真实 Owner 会话使用 Admin Management API 创建、补充、审核并发布。
- 网站名称和简介取自 D5 的双语资料。选定记录没有独立的已核验旧站长介绍或需要新增的备用地址。网站 Logo 经过摘要校验，经常规上传绑定到开发对象存储。历史封面未迁入。
- 游戏编辑文案保留 D5 双语内容。开发者、发行商、商店和媒体事实来自真实 Steam Observation。中英文媒体各自保留采集来源。内容适龄与收录依据逐条核对官方介绍，游戏发布使用修订号和服务端预览令牌，并检查公开快照和操作回执。
- 一个网站的旧主地址返回跨主机 308，经官方页面与实采结果核对后，通过 Admin 将主地址改为官网直接入口。Public ID 保留，旧绑定可追溯。
- BROK 与 LibreCosta 的身份、发布状态和编辑数据与恢复出的备份逐项一致。BROK 的 Steam 来源、采集绑定、计划和 Schedule ID 一致。

## 采集处理

- 每个网站登记 HTTP、DNS、TLS 能力的计划，每款游戏登记 `steam.details` 的 EN/US 和 ZH-CN/CN 计划。既有队列、限流和重试策略持续生效。News 与 Players 按既有启用策略运行。
- 修复 Steam 人工建档的重复本地化插入。仅对与来源相同的新建文案补记引用，保留人工改写。提交 `90b1c280095807e86f76e56fdff714cdb49e1e28`。
- 修复网站确认巡检缺失基线时的恢复。首次 Observation 可能早于绑定生效时间。后续真实确认采用现有快照规则建立基线，保留原 Observation 身份，重复读取保持幂等。提交 `a700edc8be1eaa705cd3743c858f1cc1398bf12d`。
- 开发 Harvest Server 写入凭据按授权补齐 `web.http`、`web.dns` 和 `web.tls` 能力。私有配置、备份、逐条审核证据与运行脚本均保留在本地。
- Steam 预填与持续采集的内容相同时，Harvest 会确认原 Observation。GoFurry 通过同一 Consumer 的有界巡检读取现有结果，保存真实来源与确认时间。

## 原生验证

- Windows 原生 `go -C services/api test ./...` 通过。GoFurry 管理 HTTP 回归覆盖 Steam 预填有无修改的本地化归属。原生 PostgreSQL 回归覆盖双语计划、状态隔离与网站巡检恢复。测试事务只操作临时表。
- `task sql:generate` 和 `task sql:check` 通过。GoFurry Goose `202611210003`，Harvest Goose `8`，Admin Goose `202611050002`。本次无新增 Migration。
- 本地原生 Admin API、Vite、GoFurry API、SSR、Harvest Server 和 Worker 通过实际读取，Consumer 已连通。未使用 Docker、Docker Compose、Podman 或 Testcontainers。
- Admin 和 Harvest 无代码变更。所有执行结果和未完成项按下面的最终数据核对记录。

## 实际结果

核对时间 `2026-11-22 01:15:18 中国标准时间`。数字只统计本轮新增条目。

| 指标 | 目标 | 实际 |
| --- | ---: | ---: |
| 录入网站 | 30 | 30 |
| 录入游戏 | 30 | 30 |
| 正式发布网站 | 30 | 30 |
| 正式发布游戏 | 30 | 30 |
| 网站中英文公开详情、Canonical 和目录 | 30 | 30 |
| 游戏中英文公开详情、Canonical 和目录 | 30 | 30 |
| Steam 来源归属和双语公开快照 | 30 | 30 |
| 已核对关联与状态的 Harvest Schedule | 150 | 150 |
| 网站 HTTP 与 TLS 结果均已投影 | 30 | 30 |
| 游戏双语结果均已投影 | 30 | 30 |
| 公开网站 Logo 和游戏封面均可读取 | 60 | 60 |

- 逐条核对了 Admin 状态筛选、检索与详情读取。60 条发布记录均有 Owner 回执，每条只有一次发布操作。抽查浏览器编辑页、双语计划、实际执行状态和采集工作区。
- 30 个网站覆盖 6 个主分类。HTTP 任务 22 项成功、8 项部分成功，均保有真实 HTTP 结果。30 项 TLS 任务成功。DNS 共 30 项失败，投影 0 项。最后失败代码为 `deadline_exceeded` 9 条、`resolver_unavailable` 14 条、`worker_lost` 7 条。用同一开发主机直接连接 Worker 的 Google DoH 端点也超时。逐条列出实际 Job、Attempt、状态和可用数据，保留计划和失败历史。
- Steam 登记 60 项持续采集计划。执行状态为 `succeeded` 60。本轮 Steam Details 共 120 个 Job、135 次 Worker Attempt，含预填、持续采集及真实重试。预填按现有每小时 40 个语言范围额度分批。
- 检查游戏 Steam AppID、价格引用和相关推荐。逐条核对 Header、Capsule、封面及可用截图与视频候选，素材保持官方 Observation 的归属。78 个不同的 Header/Capsule URL 可读。8 条中文范围的 Steam 简介没有确认为中文，公开中文稿来自封板编辑资料。
- 开发库原有记录保留。新增 Steam 来源无重复活动身份或孤儿采集绑定。抽检录入期 Draft 的公开详情为 404，目录中无对应记录。
- 私有 `.local/ops-data-60/manual-verification.md` 含旧站身份、新 Public ID、公开双语详情及 Admin 链接、采集和素材核对结果。逐条证据、异常、回执和恢复验证明细保留在同一目录。
