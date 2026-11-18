# CN 私有部署与 D7-A 验收

公开链路是 Nginx、GoFurry Fiber API、TanStack SSR 和区域 PostgreSQL 18。Admin 是可选的 `ops` profile，需要自己的数据库、角色和 Owner。Redis 复用已有实例和专用 ACL，用于共享计数和限流。配置为空或发生短暂故障时，公开读取仍可工作。Logo 和壁纸使用同一区域选定的 S3 兼容存储及 HTTPS 交付端点。

Identity 与 Harvest 独立启动。当前 GoFurry CN 未接入 Identity 登录，Admin 使用独立会话。Identity 沿用自己的候选部署、数据库、Redis 和签名密钥。采集需要 Harvest Server、Worker 和独立 PostgreSQL，按已验收策略显式迁移和初始化，通过 GoFurry 私有 YAML 启用 Consumer。公开 SSR 读取本库内容，已发布数据不依赖两套服务在线。AI Gateway、通知、天气与地理数据按需启用。 [Identity 部署](https://github.com/librecosta/identity/tree/develop/deploy/production) · [Harvest 运行说明](https://github.com/librecosta/harvest/blob/develop/README.md)

## 构建和配置

在构建机从批准版本分别构建 `services/api/Dockerfile`、`apps/web/Dockerfile` 和 Admin 仓库的 `services/api/Dockerfile`。使用明确的 revision 标签或镜像摘要，在 VPS 上加载镜像并保留上一套。

```sh
# 在 GoFurry 仓库根目录运行前两条，在 Admin 仓库根目录运行第三条。
docker build -f services/api/Dockerfile -t gofurry-api:<revision> .
docker build -f apps/web/Dockerfile -t gofurry-web:<revision> .
docker build -f services/api/Dockerfile -t librecosta-admin:<revision> .
```

在仓库外创建配置目录。从 `gofurry.example.yaml`、`admin.example.yaml` 和 `nginx.conf.example` 分别复制成 `gofurry/config.yaml`、`admin/config.yaml`、`nginx/nginx.conf`，按需挂载数据库 CA。Nginx 读取 `site.crt`、`site.key`、`admin.crt`、`admin.key`。保持目录及密钥最小权限，在 Debian 用容器所需的组 65532 或 101 配置只读访问。独立签发或在两个名称一致时为私有演练使用同一张带相应 SAN 的证书。Admin 的信任来源须与实际环回 HTTPS 端口完全相同。CN 两套运行 YAML 使用各自的 PostgreSQL 角色及 `ssl_mode: disable`，GoFurry Redis 使用自己的 ACL。内部 TLS 部署可选 `verify-full` 与 `redis.url: rediss://…`。显式配置的存储写入端点及交付地址均须从 API 可达。配置文件变更随应用重启生效。

以下是 `/srv/gofurry-cn/compose.env` 示例。值仅用于镜像、路径、端口和既有 SSR 运行选项。应用密钥、数据库和存储连接放在受限 YAML。

```dotenv
CN_CONFIG_DIR=/srv/gofurry-cn/config
GOFURRY_API_IMAGE=gofurry-api:<revision>
GOFURRY_WEB_IMAGE=gofurry-web:<revision>
ADMIN_IMAGE=librecosta-admin:<revision>
WEB_PUBLIC_ORIGIN=https://127.0.0.1:18443
CN_HTTPS_PORT=18443
CN_ADMIN_PORT=18444
```

## 迁移、启动和恢复

共享 PostgreSQL 与 Redis 的建库、角色、ACL 及授权顺序见 [OPS-S2-C 数据库初始化契约](database/README.md)。迁移 YAML 存在运行容器挂载目录之外，沿用同一配置结构并使用各库的 Migrator。

本机、SSH 本地转发或核准的 Tailscale 接入都应保留 Compose 的宿主环回端口。Admin 使用独立 `18444` HTTPS，公开站点入口候选为 `18443`。Nginx、API、Admin 共享项目内网络命名空间，Admin HTTP 和 GoFurry Management 仍分别只监听 `127.0.0.1:8082` 和 `127.0.0.1:17324`。SSR 只连项目内的公开 API。保留默认网络作为应用入口和默认出口。Compose 2.33.1+ 为 edge 附加已有 `librecosta-backend`，API 与 Admin 共享该网络命名空间。两个 maintenance 服务独立接入后端网，SSR 只在默认网络。使用 Docker Engine 28+ 的环回端口隔离行为。 [Compose 网络模式](https://docs.docker.com/reference/compose-file/services/#network_mode) · [Docker 端口发布](https://docs.docker.com/engine/network/port-publishing/)

```sh
# 在 GoFurry 仓库根目录，以 shell 函数固定项目与配置文件。
cn() { docker compose -f deploy/cn/compose.yaml --env-file /srv/gofurry-cn/compose.env "$@"; }
cn config --quiet
cn run --rm -v /srv/gofurry-cn/maintenance/gofurry.yaml:/run/maintenance.yaml:ro \
  api-maintenance migrate up --config /run/maintenance.yaml

# 启用 Admin 时也显式迁移其数据库。
cn run --rm -v /srv/gofurry-cn/maintenance/admin.yaml:/run/maintenance.yaml:ro \
  admin-maintenance migrate up --dir /app/migrations --config /run/maintenance.yaml

# 分别给已迁移的数据库执行 database/grants.sql。启用 Admin 时交互初始化一次 Owner。
cn run --rm admin-maintenance account init-owner --config /run/admin/config.yaml

cn up -d --wait
cn --profile ops up -d --wait   # 含 Admin 时使用
cn --profile ops ps
cn --profile ops logs --since 10m edge api web admin
cn --profile ops stop
```

存储与数据库的持久数据在 Compose 项目之外。日志由 Docker `local` 按 10 MiB × 3 压缩轮转。Nginx 记录方法、状态、耗时和请求 ID，交互路由关闭请求日志。使用 `/livez`、`/readyz` 和 `/web-readyz` 监测进程，再定期查询 `/api/v1/sites` 验证数据库和 Schema。启用 Admin 时另外检查其私有 `/readyz`。按常规 PostgreSQL 和 S3 操作备份，D5 数据初始化继续使用已核验 v2 和原有隔离守卫。 [D5-C2 初始化与恢复](../../docs/v0.1-d5-c2-migration-closure.md)

更新前留存上一套镜像和外部配置快照，显式查看迁移状态，必要时备份并在停止写入后迁移。更换镜像或 edge 配置时对整个应用项目执行 `cn --profile ops up -d --force-recreate --wait`，保持共享网络命名空间一致。只部署公开链路时省略 profile。镜像回滚使用上一组镜像及配置，前提是 Schema 仍兼容。涉及不兼容 Schema 时，恢复备份到另一个库并核对对象清单、新写入与身份后再切换连接。应用停止与重建均不停止共享数据库或 Redis。 [Compose run](https://docs.docker.com/reference/cli/docker/compose/run/)

正式 CN 候选应替换 Nginx 公开 `server_name`、证书及 `WEB_PUBLIC_ORIGIN` 为核准的 HTTPS 域名。模板默认保留 `X-Robots-Tag: noindex, nofollow`、Robots 全站禁止、Sitemap 返回 404。D7-B 核准公开路由、生成只含已发布 canonical 的 Sitemap 并在放流量前核对搜索策略。前置宿主 Nginx 如需保留客户端 IP，只信已验证的上一级代理地址并复测覆写行为。Admin 保持环回 TLS，不挂到正式站点路由。 [Nginx 代理头](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_set_header)

## 私有演练结果

- Docker Desktop Linux/amd64、Engine 29.8.1、Compose 5.5.1。真实构建 GoFurry API、SSR、Admin Embed 三张非 root 镜像。Docker Hub 认证不可达后，使用同版本 Google Docker Hub 镜像源完成构建。Compose TLS Nginx 与三服务健康。隔离 PostgreSQL 18.6 使用两个数据库和角色，Goose 分别应用 17/2 个迁移。两套运行连接确认 TLS 1.3。Redis 8.2.1 使用临时 ACL，RustFS 用独立持久卷和测试证书。
- 真实 Bootstrap v2 的 SHA256 与 D5-C2 一致，`verify/plan/apply/verify-target` 均通过。首次应用、复核各 5,892 条，247 个 Site、261 个 Game、16 个合集全部保留，公开记录为零。223 条已有 Logo 引用复用原规则。两项就绪分别为 Bootstrap **PASS**、Publication **INCOMPLETE**。
- Chromium 经 Nginx HTTPS 登录 Admin，验证 Secure/HttpOnly/Strict Session、匿名 401、无 CSRF 403、代理读取与合成记录写入。隔离环境发布两条合成 Site、一个合成 Game 和一个合成合集，用公开 API 与 SSR 读取。验收真实 WebP 上传、HTTPS 交付字节哈希、默认 Logo、Canonical、Hreflang、静态资源 immutable、异常 Host 421、交互请求上限 413、Robots 和禁用的 Sitemap。SSR 容器能访问公开 API，无法访问 Admin 和 Management 监听端口。API 停机时 Nginx 返回 503。
- 首次重启暴露 Nginx 尚未解析 SSR 时 Compose 已返回就绪的问题。现在 edge 的健康检查验证 SSR 上游和 API，并缩短 Docker DNS 重试间隔。强制重建、停止后重新启动，以及重启测试 PostgreSQL/RustFS 后，原 Admin 会话、资料、Logo 哈希均通过复核。
- 独立回归通过 Identity `task check:first-party`（真实 Identity、现有 PG17/Redis 夹具、授权和撤销）及 GoFurry `task check:collection`（真实 Harvest Server/Worker 与模拟上游，包含 Consumer 恢复和 Steam 持久化）。本轮 PostgreSQL 18 部署验收覆盖 GoFurry 和 Admin。Admin 实际双服务 Playwright 8 项、GoFurry Web 88 项通过（3 项按既有无夹具条件跳过），Go、sqlc、Catalog、Admin Web 构建通过。

### 4C8G 预算

同一 Docker Desktop 演练以约 10 req/s 混合 SSR、公开 API 和已认证 Admin 查询，共 240 请求全部 200，P50 8.7 ms、P95 15.5 ms。四个负载采样点的逐容器高水位如下。内存为 Docker Stats 工作集，CPU 比例以单核为 100%。

| 服务 | 采样峰值 MiB | 内存上限 MiB | CPU 核上限 |
| --- | ---: | ---: | ---: |
| Nginx | 12.83 | 96 | 0.25 |
| API | 11.10 | 256 | 0.75 |
| SSR | 69.13 | 512 | 1.00 |
| Admin | 73.52 | 384 | 0.75 |
| 测试 PostgreSQL | 108.50 | 768 | 1.00 |
| 测试 Redis | 6.57 | 128 | 0.25 |
| 测试 RustFS | 110.50 | 384 | 0.50 |

新站应用含 Admin 的采样峰值合计约 167 MiB，内存上限合计 1,248 MiB、CPU 上限合计 2.75 核。GoFurry 公开链路上限为 864 MiB、2 核。复用 PG/Redis 和外部存储时，无需重复计入演练实例。D7-B 在旧站繁忙时实测 VPS 余量。含 Admin 的起始预算约 2.25 GiB 可用 RAM（新应用上限加 1 GiB 系统与数据缓存余量），并核对现有应用 CPU/IO 峰值、PG 连接和 Redis 内存。旧站和共享基础服务未在目标 VPS 上测量，并行可行性仍待现场确认。Identity/Harvest 依赖的资源须按它们的独立负载再核算。

D7-A 支持进入 D7-B 的私有目标机验证。备案、公共 TLS、正式 CDN、旧站快照后增量、公开 Sitemap 与内容审批仍须独立核对。本轮没有访问目标 VPS，无公网流量、生产数据库、生产存储写入或旧路由变更。
