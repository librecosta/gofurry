# OPS-S2-C · CN 数据库初始化契约

适用 Debian 13.7、PostgreSQL 18.7（实例已安装 pgvector 0.8.7）和 Redis 8.2.11。复用一个 CN PostgreSQL 实例和一个 CN Redis 实例。两者只接入 Docker 私网 `librecosta-backend`，不发布宿主机端口。INTL 使用独立实例与凭据。本契约只负责建库、迁移与授权，应用启动继续使用显式 YAML。

## 数据所有权与迁移入口

| 服务 | PostgreSQL 数据库 / Schema | Goose 路径及冻结版本 | Redis |
| --- | --- | --- | --- |
| Admin | `librecosta_admin_cn` / `public`。启动必需，保存自身账号、Session、操作审计。 | `services/api/migrations/` · `202611050002`（2） | 无 |
| AI Gateway | `gateway` / `public`。启动必需，保存运营注册表、加密凭据、请求流水、异步任务、用量及审计。 | `migrations/`（嵌入）· 4 | 启动必需。共享限流、占用租约、熔断、重试预算及可选响应缓存。 |
| Billing | 无。审计时 `develop` 只有项目说明与 Git 忽略文件。 | 无 | 无 |
| GoFurry | `gofurry_cn` / `public`。CN 产品必需，保存 Site、Game、合集、壁纸元数据、事实和游标。仅 Start 可无库运行。 | `services/api/migrations/` · `202611150003`（17） | 可选。缓存、合并并发请求和共享限流。站点有效浏览及点击还需启用 Redis、交互密钥和来源配置。 |
| Harvest | `harvest` / `harvest`。Server 必需，保存调度、租约、限流、运营策略、Observation、Change Stream。`public` 存 Goose 记录。 | `db/migrations/`（嵌入）· 7，首条迁移创建 `harvest` Schema | 无。Worker 经 HTTP 使用 Server，无需数据库凭据。 |
| Identity | `identity` / `public`。启动必需，持有账号、凭据、Session、撤销状态及通知投递账目。 | `services/api/migrations/`（嵌入）· 10 | 启动必需。共享准入、匿名 CSRF、OAuth 状态、待注册资料及加密重试缓存。 |
| Notification | 无。发送服务执行单次 SMTP 尝试，限流在进程内，可选本地标记文件。持久投递状态在 Identity。 | 无 | 无 |

五个有库服务均使用 sqlc + pgx。Admin、GoFurry、Identity 的 sqlc 入口为 `services/api/sqlc.yaml`，查询位于 `services/api/internal/database/queries/`。AI Gateway 和 Harvest 的入口为根目录 `sqlc.yaml`，查询位于 `db/queries/`。业务 UUID 继续遵守各服务原有 UUIDv7 契约。本次没有业务迁移依赖扩展，**无需在任何业务库启用 `vector` 或修改 `template1`**。

产品间经认证 HTTP 交互。Admin 代理 GoFurry Management，GoFurry 消费 Harvest 并可选使用 AI Gateway，Identity 可调用 Notification。GoFurry 的旧站审计只读恢复后的 `gfa/gfg/gfn`，这些库与 Identity 的测试联调库均不在生产清单中。

## PostgreSQL 角色与权限

AI Gateway 和 Identity 已有独立的迁移 DSN。Admin、GoFurry 和 Harvest 的迁移命令与服务使用相同的数据库配置结构，CN 通过不同的私有 YAML 为两种入口选择各自的角色。

**11 个独立登录密码**。5 个 Migrator、5 个 Runtime、1 个 Identity 应用操作员。现有集群管理员、备份账号和 SMTP/HTTP 密钥另计。迁移角色拥有自己的数据库及业务对象。运行角色只获本库 `CONNECT`、必要 Schema 的 `USAGE` 和下表 SQL 权限。录入密码时用受控 `psql` 的 `\password <role>`，确认 `password_encryption` 为 `scram-sha-256`。维护与服务进程分别挂载私有 YAML，Runtime 容器只读取自己的文件和信任材料。

| 数据库 | Migrator → Runtime | Runtime 授权 |
| --- | --- | --- |
| `librecosta_admin_cn` | `librecosta_admin_migrator` → `librecosta_admin_runtime` | 账号和 Session 读、插入、更新。审计读、追加。 |
| `gateway` | `gateway_owner` → `gateway_runtime` | Registry 只读。执行账本读写，含 B4 的 `gateway_job_identities`。留存删除仅限请求、桶、汇总与调整记录。审计读、追加，审计到期由维护入口处理。可执行账务函数 `aigw_touch`。 |
| `gofurry_cn` | `gofurry_cn_migrator` → `gofurry_cn_runtime` | 现有业务表 DML，视图只读，业务函数可执行。操作、发布、站点分级和壁纸审计读、追加。旧身份映射、导入回执及 Goose 表为维护范围。 |
| `harvest` | `harvest_migrator` → `harvest_runtime` | `harvest` 内业务表 DML。只读 `public.goose_db_version` 供就绪检查。 |
| `identity` | `identity_migrator` → `identity_runtime` | 账号、凭据、Session 与挑战读写。投递账目允许删过期记录。外部身份仅可读、插入与更新 `unlinked_at`。应用注册资料只读，`applications.updated_at` 列更新权限用于现有行锁。 |
| `identity` | `identity_app_operator` 继承 `identity_application_admin`（NOLOGIN） | `identity-appctl` 维护应用、回调与凭据，可撤销相应授权码、应用会话和 refresh token。部署绑定行只读。 |

运行账号不拥有 Schema 或业务对象，没有 DDL、TRUNCATE、角色管理或跨库授权。业务库撤销 PUBLIC 的 CONNECT/TEMP 及业务 Schema 和对象公共授权。共享实例若保留其他旧库的 PUBLIC CONNECT，在获批网段的 `pg_hba.conf` 中对每个数据库和角色用 `hostssl <db> <role> <approved-subnet> scram-sha-256` 精确放行，对这些角色的其余连接明确拒绝。拒绝规则须排在既有泛化放行之前，覆盖实际走的 TCP 和本地 socket 路径，避免公开 ACL 或认证回退绕过数据库边界。管理员的维护通道沿用实例已有规则。

## Redis ACL

使用 DB 0。**必需账号 2 个**，GoFurry 的共享缓存或站点计数获准后增至 **3 个**。 [Redis ACL](https://redis.io/docs/latest/management/security/acl/) · [Redis TLS 与访问边界](https://redis.io/docs/latest/management/security/)

| ACL 用户 | 允许的键空间 | 权限与配置 |
| --- | --- | --- |
| `identity_cn` | `lcid:cn:v1:*` | `HELLO/PING`、准入和短期状态所需的 KV、TTL 及 Lua。`redis.url` 用 `rediss://`，`trust.ca_file` 包含核准 CA。Redis 客户端池上限 4。 |
| `ai_gateway_cn` | `aigw:{runtime}:*`、`aigw:cache:*` | KV、TTL、Lua、熔断 Hash、租约/限流 ZSET、`TIME`。前缀没有区域字段，CN Redis 实例不得与 INTL 共用。用 `redis.url` 的 `rediss://`，CA 由镜像系统信任链提供。`redis.url` 可带 `pool_size=4&max_active_conns=8` 作为起步值。 |
| `gofurry_cn` | `gofurry:v1:{CN}:*` | KV、TTL、Lua 与限额 ZSET。配置 `redis.address/username/password/db`，池基准 8、活动连接上限 16。现有客户端只支持明文。 |

[redis.acl.example](redis.acl.example) 提供精确命令表，初始账号禁用且无密码。先确认名称未占用，按现有实例管理方式把需要的行合入私有 ACL 文件，保留已有运维账号和规则。获批启用时在对应行末加入 `on #<独立随机密码的 SHA256>`，应用私有 YAML 保存对应明文。禁止把带密码哈希的 ACL 文件提交 Git。新实例关闭默认免密用户。既有实例先确认无依赖，再按运维变更流程处理。使用现有 Redis ACL 装载/校验方式，逐个账号验证认证、键读写及 TTL，确认其他前缀、`SELECT 1`、`FLUSHALL`、`CONFIG`、发布订阅均被拒绝。模板不授予全局键扫描或管理命令。共享实例建议保留 `noeviction` 并监测内存和拒写，避免缓存挤掉安全状态及活动租约。

## YAML、连接池与执行顺序

| 服务 | 显式连接配置 | 建议起步上限 / 副本 |
| --- | --- | --- |
| Admin | `database.host/port/name/user/password/ssl_mode/ssl_root_cert`。迁移与运行文件同 Schema、账号不同。 | PG 5，最小 0，连接超时 3s、空闲 5m、寿命 30m。迁移 1。 |
| AI Gateway | `database.url` 运行、`database.migration_url` 维护。DSN 显式 `sslmode=verify-full` 与核准 CA。 | `database.max_connections: 4`，代码连接超时 3s。维护命令单次运行。 |
| GoFurry | 与 Admin 相同的字段名和 TLS 策略，使用 GoFurry 自己的数据库与 YAML。 | PG 8，最小 0，连接超时 3s、空闲 5m、寿命 30m。启用 Consumer、Yearbook、Heat 各再预留 1，合计最多 11。SSR 无 DB 池，迁移 1。 |
| Harvest | `database.host/port/name/user/password/tls_mode/max_connections`。`verify-full` 的 CA 由镜像系统信任链提供。维护另用 YAML。 | Server PG 4（代码范围 2–32）。迁移也设 4 以保留 Goose 会话锁连接，Worker 0。 |
| Identity | `database.url` 运行、`database.migration_url` 迁移及绑定、`database.application_admin_url` 应用维护。PG DSN 用 `sslmode=verify-full` 和显式 `sslrootcert`。 | 运行 PG 4（代码固定），连接超时 3s、空闲 5m、寿命 1h。迁移 1，绑定/Appctl 短期池 4。 |

五个服务各一副本并以 GoFurry 全部后台任务计，运行态 PG 预算为 **28 个连接**，另留维护、备份和监控余量。先查同实例旧业务峰值、`max_connections`、保留连接及逐库活动，再批准扩副本。所有连接用与证书 SAN 匹配的私网别名，校验 CA 和 SCRAM。配置、DSN、密钥仅保存在仓库外的受限文件中。

1. 从可信维护通道确认 CN 实例版本、备份、网络及名称余量后，以 PostgreSQL 集群管理员运行 `psql -X -d postgres -v confirm_cn=librecosta-cn -f provision.sql`。[provision.sql](provision.sql) 只为这五个库建立无密码的角色和带标记的 UTF-8 数据库，串行建库，先关闭新库连接、撤销 PUBLIC 权限再开放。重复执行保留数据、密码及人工关闭的连接状态，遇无标记同名对象或异常所属/成员关系即停止。若中断在 CREATE DATABASE 与打标之间，先人工核实那个仍禁止连接的空库及其所有者再处理。现有同名库需要独立审查，禁止补标后直接接管。
2. 独立录入 11 个 SCRAM 密码，完成 HBA 和 TLS，先准备 Identity 与 AI Gateway 的 Redis ACL。五套迁移互不依赖，建议逐库执行以便故障归属。命令中的可执行文件名为逻辑名称，容器使用已有 ENTRYPOINT 或维护命令（GoFurry 为 /api，AI Gateway 镜像内用 --entrypoint /gatewayctl）。源码执行时将 /app/migrations 换为对应仓库的迁移目录。

   | 服务 | 显式迁移命令 |
   | --- | --- |
   | Admin | `admin migrate up --dir /app/migrations --config <admin-maintenance.yaml>` |
   | AI Gateway | `gatewayctl migrate up --config <gateway-maintenance.yaml>` |
   | GoFurry | `gofurry migrate up --dir /app/migrations --config <gofurry-maintenance.yaml>` |
   | Harvest | `harvest migrate up --config <harvest-maintenance.yaml>` |
   | Identity | `identity-migrate --config <identity-maintenance.yaml> up` |

3. 每个库以自己的 Migrator 登录，执行 `psql -X -d <database> -f grants.sql`。[grants.sql](grants.sql) 在单库事务内先校验标记、所有权及冻结 Goose 版本，再应用授权。每次升级需先核查新对象和 DML、复审版本及矩阵。既有脚本不会为未知版本放权。使用实际运行账号验证本库插读或服务探针、跨库连接拒绝、无 CREATE/ALTER/TRUNCATE、审计表不可修改、迁移版本表不可写。
4. 按上线范围执行领域初始化。Admin 首次用受控终端运行 `admin account init-owner --config <admin-runtime.yaml>`。Identity 只在全新部署显式执行 `identity-migrate --config <identity-maintenance.yaml> bind`，冻结真实 CN Issuer，然后用应用操作员注册获准客户端。Harvest 需要采集时运行一次 `harvest policy init --config <harvest-server-init.yaml>`，先确认各 Source、Quota、Egress 与 Steam 每日预算。后续使用版本化策略 API。AI Gateway 使用独立维护入口进行 Registry 管理和审计留存（`gatewayctl retention run`），本地 `seed` 命令不用于生产。GoFurry 若初始化 D5 内容，沿用已验收的隔离导入与数据/对象提升流程。从受控备份恢复时保留身份、导入回执和对象键，核对所有权和授权，不覆盖已有写入。 [D5-C2 恢复契约](../../../docs/v0.1-d5-c2-migration-closure.md)
5. 以 Runtime 配置启动获准产品并做实际读、写及就绪检查。Admin、Harvest、Identity、AI Gateway 独立上线。GoFurry 公开 SSR 不等待它们。迁移失败保持应用停写，确认 Goose 元数据和对象状态后重跑原命令。已发流量的数据回退走经验证的独立恢复流程，禁止自动 Down 或清库。

## 实施前需确认

- **当前名称与私有网络。** `gateway`、`harvest`、`identity` 沿用仓库实例命名。核实生产集群是否同名、占用者和已有授权，尤其是旧站同实例时。D7 的 GoFurry API 与 Admin 共享 edge 的网络命名空间，接数据私网时实际应给 edge 和各迁移容器加入已建的 `librecosta-backend`。Identity 现有候选 Compose 也须核对数据面和维护容器的网络附件，保留原入站限制。
- **Redis 与证书。** 如果生产 Redis 只允许 TLS，GoFurry 共享功能暂不可启用。若要求它上线即计数，先确认可接受经隔离的内部明文监听，或在独立变更中补客户端 TLS。两种必需 Redis 连接保持 TLS，并在镜像里验证 CA。Harvest PostgreSQL 的系统 CA 信任同样需要实测。数据库初始化无需等待 GoFurry Redis 决策。
- **Identity 和内容。** Identity 既有认证端到端夹具仍为 PG17。PG18 已跑原生迁移和隔离绑定，正式启动前仍需带实际 CA 和权限在 PG18 上跑注册、会话、撤销与应用操作员验收。真实 CN Issuer 必须在首次绑定前确认。D5-C2 的 Bootstrap Ready 为 PASS，现有 CLI 有隔离目标守卫，内容提升和 Publication 审批与建库分别执行。

**五库方案可进入获批的空库 Provisioning。** 执行前核实名称、网络、TLS 和现有授权。业务放流量仍以对应服务验收为准。

## 证据

审计基准为各仓库 `develop`，Admin 按远端提交审计，本机分叉未纳入。 [Admin b907eba](https://github.com/librecosta/admin/tree/b907eba769526a6106b020089e8c34aaaea74dc0/services/api) · [AI Gateway 1b194c8](https://github.com/librecosta/ai-gateway/tree/1b194c8c48960506ffc2b07c1d466b56ab9797b8) · [Billing 09d8784](https://github.com/librecosta/billing/tree/09d878412a0038e224122806bb350abd77a246af) · [GoFurry 0324179](https://github.com/librecosta/gofurry/tree/0324179975b1c0422c40915ae0b8bd10de52e336/services/api) · [Harvest 349730b](https://github.com/librecosta/harvest/tree/349730b5c3ff57be084dfac72dfb77f19dacb54c) · [Identity cf82dce](https://github.com/librecosta/identity/tree/cf82dcefb893cc5cf2a3a142902cf098df7ee840) · [Notification 04a074a](https://github.com/librecosta/notification/tree/04a074a84358225f41a52caeafe2dba7aa6a21aa)

本轮使用一次性 PostgreSQL 18.7 和 Redis 8.2.11 容器。五个原生 Goose 命令在各自 Migrator 下共应用 40 条迁移。PG 另验证建库与授权可重复执行、待完成库可恢复、人工关闭保持关闭、未确认及同名冲突拒绝、五个 Runtime 和 Identity 应用操作员的权限、六次实际跨库连接拒绝、Identity 隔离绑定、应用操作员注册 UUIDv7 应用及配置回调和停用、拒绝 Runtime 执行应用维护，以及 Harvest 使用 Runtime 初始化策略。Redis 实际验证三个账号认证、Lua、TTL，以及跨前缀（含 Lua）、切库、全局删除、配置、发布订阅拒绝。演练使用隔离的本地连接，生产 TLS、完整业务认证、现存数据库授权及宿主网络尚未验证。D7 Compose 配置检查通过。
