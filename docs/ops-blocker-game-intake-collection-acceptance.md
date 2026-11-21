# OPS-BLOCKER 游戏录入与采集中心验收

## 新建游戏

1. 在 Admin 的「游戏管理」进入「新建游戏」。可直接填写中英文名称与简介。服务端根据内容生成链接所需的 Slug，运营无需填写。
2. 可填写 Steam AppID 并点击「自动补充」。GoFurry 先核对来源归属。AppID 已被使用时，表单显示现有游戏入口。未被使用时，服务按 EN/US 与 ZH-CN/CN 分开核对近期 Harvest 观测。需要补采的地区使用有界的单次 Job。表单可查看进度、恢复查询或重试。
3. 表单显示可用的名称、简介、开发者、发行商与已验证的图片。语言和素材可以部分返回。已有手工输入保留。Steam 返回英文回退时，只接受可验证的客观资料和素材，不会将英文文案填入中文。
4. 点击「创建游戏」才写入 Draft。已验证且保持原样的来源文案保留 Observation 归属。英文名称用于生成装饰性 Slug，无可用拼写时使用 `game`。Public ID 区分身份，既有 Canonical 规则继续适用。

Admin 将请求交给 GoFurry Management 的 `GET|POST /games/steam-prefill` 及 `POST /games`。创建事务同时保存 Game、双语编辑稿、Steam GameSource、绑定、双语 `steam.details` 计划与操作回执。Reconcile 负责远端登记。Harvest 故障期间，已经创建的草稿和待同步意图可以继续使用。后台 Discovery 使用原入口。News 和 Players 由运营单独添加，创建游戏时不会自动启用。

## 网站和采集中心

网站创建或保存主要访问 URL 时，在同一数据库事务登记归属。HTTP 和 DNS 默认每天采集，HTTPS 地址额外登记 TLS。Draft 可以采集。地址替换关闭旧绑定并保留历史。暂停意图按内容绑定保存。使用同一来源的其他有效内容仍可保持采集，归档的内容也保留历史。

| 工作区 | GoFurry Management 接口与数据 |
| --- | --- |
| 概览 | `GET /operations/collection/summary` 与现有 Consumer 查询。Harvest 提供队列、失败和在线 Worker 统计。GoFurry 提供待同步意图及最近有效观测。 |
| 采集计划 | `GET /operations/collection/plans`。汇合本地来源与计划、真实 Schedule、最新 Job 和 Observation。可按内容、名称和能力筛选。 |
| 采集执行 | 内容级 `POST /{sites\|games}/:id/collection/{register,plans}`、`POST .../:plan/{state,run}`。启停首先更新本地意图，立即采集核对真实 Schedule 和准确的请求 Scope。 |
| 队列与历史 | `GET /operations/collection/jobs` 及 `GET .../jobs/:job?entity=...`。历史按内容、来源、能力、状态和日期分页。展开后可见 Job、Attempt、等待原因和确认的观测。 |
| 任务控制 | `POST .../jobs/:job/{cancel,retry}`。先核对管理权限、有效内容、Schedule、目标和请求 Scope，再执行并写入现有操作审计。 |
| 采集结果 | `GET /{sites\|games}/:id/collection/:plan/results`。展示最多 12 条当前投影，不同观测状态各自显示。任务详情展示该 Job 已确认的观测。 |

所有 API 路径位于 GoFurry 的私有运营路由内。Admin 延用会话和操作回执。Owner、Operator 可以执行授权操作。Developer 只读。浏览器没有 Harvest 直连入口。连接不可用时显示已保留的数据和连接状态。

Harvest 新增经身份验证的 `GET /api/v1/jobs/collection` 和 `GET /api/v1/jobs/collection/status`。查询受 principal 的 namespace 和 capability 限制。前者每页最多 20 条有 Schedule 的 Job，后者给出 Worker 与采集任务计数。现有重试操作保留 Schedule 归属，方便同一条计划追溯重试链路。Harvest 没有本轮新增迁移。

## Windows 原生验收

- GoFurry `task check:api`、sqlc generate/diff/vet 和 Goose validate 通过。原生 SSR Playwright 在备用端口运行，88 项通过，3 项依赖专用天气夹具而跳过。
- PostgreSQL 18.6 连接级临时表配合原生 GoFurry HTTP。通过 Draft 网站登记和替换地址、无 Harvest 时创建双语 Game 与采集意图、重复 AppID、预填不创建 Game、权限拒绝、按计划暂停和启用、可选 Players 及跨内容隔离。Run Now、取消和重试经过隔离 HTTP 夹具验证，核对了目标、发出次数及幂等请求。另验证英文与中文来源转换、英文回退的归属约束，并确认 Draft、Published、Archived 的人工编辑与采集投影隔离。
- Admin 原生 `check:api`、`check:web`、Embed 测试和 Playwright 通过。常规 Playwright 12 项通过，22 项需要专用夹具而跳过。新增 5 项覆盖中文与英文分别成功、失败后手工创建、无 Slug 字段、采集控制确认、结果、权限以及 390px 和深色主题。
- 原生 Harvest `task check`、sqlc vet 通过。用户在用的 GoFurry API、SSR、Admin 与 Harvest 的就绪检查返回 200，SSR `/games` 与 BROK 公开 API 返回 200。
- 用独立回环监听、临时 Admin 账号和真实 VPN PostgreSQL 完成只读浏览器联调。Admin 读到两条真实 Steam 计划、5 条 Job、Attempt 与已确认的观测。每条计划可读取 3 个当前结果。Harvest 返回 1 个可用 Worker、0 个运行中 Job。匿名返回 401，跨内容读取同一 Job 返回 404。
- 全程 Docker、Docker Compose、Podman、Testcontainers 使用次数均为 0。新增真实 Steam 请求为 0。依赖容器的旧 D4 全量夹具任务未运行，本轮使用上述原生测试和真实只读链路。

## 开发数据与待上线条件

| 开发库 | 已应用 | 待应用 |
| --- | --- | --- |
| `gofurry_cn_dev` | `202611210001` | 前一阶段 `202611210002`，本轮 `202611210003` |
| `harvest_dev` | `7` | 前一阶段 `8` |
| `librecosta_admin_dev` | `202611050002` | 无 |

本轮迁移新增按绑定保存的启停意图和每个 AppID、地区的有界自动补充记录。隔离迁移和兼容性查询使用临时表。共享库未执行升级或写操作，升级仍需确认备份及授权。

BROK 的 Public ID 为 `j2byfcwgr8cqnkt4`，Game UUID 为 `01a1f44a-c88e-76a2-92b5-a7a3a81304c0`，GameSource UUID 为 `01a1f44a-c8b7-79ee-96dd-0d8710ff6208`。前后只读盘点均为 Published/SFW，修订 6，编辑稿及媒体摘要相同。

| Steam Scope | Plan | Harvest Schedule | 当前观测 |
| --- | --- | --- | --- |
| EN/US | `01a1f44a-c947-7ff0-a48e-2c50e4c08dae` | `01a1f44a-d1c8-7d51-9728-d0587e4fd10d` | `01a1f44a-c716-7480-8efb-abc753982cbe` |
| ZH-CN/CN | `01a1fdef-59f2-7135-9ca5-6cc6f91faa52` | `01a1fdef-5f92-717c-9646-2036718e43aa` | `01a1fdf7-472d-7935-8fbf-4beee0fe8265` |

LibreCosta 为 `k01nf5x20f1bzp3d`，保持 Published 修订 14 和 `wiki`、`directory` 标签，Admin 显示「未配置采集」。前后均为 1 个游戏、1 个网站、1 个 Intake 候选，按 application_name 检查未发现活动导入连接。

共享开发环境目前没有专用于 `gofurry_cn` 的 Harvest operator，实际 GoFurry 连接亦未获取消和重试权限。启用这两个操作需要给 GoFurry 配置获授权的 `harvest.operator_token`。计划和历史读取可用现有读取凭据。真实 Steam 新任务、创建、启停、立即运行、取消和重试均未在共享库执行。存量对象的计划按条目显式登记，无批量补齐或重置。

## 提交

| 仓库 | 实现提交 |
| --- | --- |
| GoFurry | `6381190a91be536e5e6be199598ed255b1d3132f` |
| Admin | `304b1f5a0dcb28cc598f5aba5e1182980512cc88` |
| Harvest | `34405bd5fc27bd2202de89f3ec53c5a84bf81827` |

三仓均提交在本地 `develop`。Admin 的 CI 夹具固定到本轮 GoFurry 实现，GoFurry 的 Harvest 夹具也已同步。
