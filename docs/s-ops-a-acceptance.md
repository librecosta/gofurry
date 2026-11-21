# S-OPS-A 网站录入验收

## 实施结果

- Admin 的「新建网站」只要求主访问 URL、中文名称和主分类，可选填英文名称、双语简介和最多 8 个固定标签。成功后进入详情页。详情页继续提供补充分类、备用地址、长介绍和图片上传。
- 不带协议的主地址按 HTTPS 处理。创建时由 GoFurry 根据规范化后的 Host 生成 Slug，处理 `www`/`wwwN`、子域名、IDN 和长度。同一 Slug 可对应不同 Public ID，常规编辑名称或地址会保留原有 Slug。只填英文简介时，英文资料使用中文名称作为展示名称。
- 创建事务检查已有的有效主地址和备用地址。相同地址的 HTTP/HTTPS 与根路径尾斜杠变体会提示已存在，可跳转到对应条目。路径和查询不同的站点仍可分别录入。
- 首次发布要求有效名称、主地址、主分类和 SFW 审核。保留既有人工权限、操作审计、撤回和恢复流程。公开导航继续使用固定分类与标签，相关推荐继续使用分类和标签。网站语言筛选、输入和批量操作已移除，双语资料和 SSR locale 继续使用。

## 数据库

- 新增前向迁移 `202611210001_site_intake.sql`。迁移重建两个公开网站视图，移除 `sites.supported_languages` 及语言索引，并新增有效地址查询索引。既有 Public ID、Slug 和公开视图的可见性规则保留。遇到任何非空的网站语言数组时，迁移会中止，供维护者先导出和核对。
- 通过原生 pgx 只读检查 VPN PostgreSQL 18.6 的 `gofurry_cn_dev`。检查时网站记录为 0，非空语言数组为 0。Goose 已应用至 `202611200002`，本次迁移仍为待应用。共享库升级需先核验可恢复备份并取得授权。

## 原生验证

| 验证 | 实际结果 |
| --- | --- |
| GoFurry API | `task check:api` 通过；sqlc generate、diff、vet 通过 |
| GoFurry Web | 界面类型检查和 SSR 构建通过 |
| 原生 HTTP 与 PostgreSQL | `TestSiteIntakePostgresHTTP` 通过。使用 VPN PostgreSQL 单会话临时表运行前向 DDL，并向本机真实 GoFurry Management 和公开 HTTP 路由发请求。验证迁移保全、权限、最小 Draft、重复地址、双语、分类、标签筛选及相关推荐、发布门槛、撤回、恢复和历史链接 |
| Admin API 与 Web | `task check:api`、`task check:gofurry`、`task check:web` 通过 |
| Admin Playwright | `task check:web:browser` 通过，5 条通过，21 条需要显式夹具而跳过。网站新建用例使用原生 Vite 和隔离 API 响应，验证双语切换、窄屏与暗色、未保存提醒、标签上限、重复地址提示及保存跳转 |
| 日常开发服务 | 检查时常用 GoFurry API、Management、SSR 和 Admin 本机端口均未监听。共享库仍使用旧 Schema，尚未执行已迁移共享库上的整链路验收 |

Docker、Docker Compose、Podman 和 Testcontainers 使用次数均为 0。共享业务表无测试写入，生产环境无改动。

## 待人工验收

共享库获授权并完成备份核验与迁移后，用日常 Admin 账号确认新建、详情补充、人工发布、公开站点和标签筛选。完整 D4 服务夹具依赖容器，本轮未执行。
