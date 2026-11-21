# G-OPS-R2 游戏标签管理验收

## Admin 与 API

- 「GoFurry → 游戏标签」打开 `/gofurry/game-tags`。标签定义和类别有独立列表、双语编辑和归档恢复操作。列表支持按中英文名称和状态搜索，标签还可按类别筛选，类别按展示顺序排列。
- 保存和改名沿用既有 JSONB 本地化字段。创建优先使用英文名称生成 Code，缺少可用英文标识时使用 UUIDv7 派生值。旧创建接口可提交历史格式的 Code，编辑接口仅通过路径查找现有 Code。管理页在技术信息中只读展示它。
- `GET /games/editorial-tags` 继续为游戏详情和合集规则返回有效标签。现有 `GET/PATCH /games/:id/tags` 继续处理绑定、解绑、主次角色及修订校验。游戏详情展示已绑定的角色。
- `/games/editorial-tags` 与 `/games/editorial-tag-categories` 均提供 `GET /manage`、创建、`PATCH /:code`、`POST /archive` 和 `POST /restore`。无效输入、对象不存在、定义重复、类别不可用和仍有引用分别返回对应的 400、404 或有明确原因的 409。
- Owner 和 Operator 可写入，Developer 保持只读，匿名无法访问。写入复用 Admin 审计、已验证的 Management Actor、Request ID 和提交回执。

## 数据与一致性

- 复用现有类别、标签、游戏关联和合集规则表。保留 UUIDv7、已有 Code、归档历史和现有合集 Revision 触发器，本轮没有新增迁移。
- 定义写入、游戏标签关联和合集规则编辑在获取行锁前共用事务锁。类别仍有有效标签时拒绝归档，标签被游戏或合集规则引用时拒绝归档。换类保留标签 UUID 和现有引用。换类与关联调整按原触发器更新合集 Revision，单独改名无需重算合集成员。
- GoFurry 开发库 Goose 已应用至 `202611200002`，另有 S-OPS-A 的 `202611210001` 待应用。Admin 已应用 `202611050001` 和 `202611050002`。两库均只读核对迁移状态，业务表保持原状。

## 原生验证

| 验证 | 结果 |
| --- | --- |
| GoFurry API 和 sqlc | `task check:api`、sqlc generate、diff、vet 通过 |
| GoFurry HTTP 与 PostgreSQL | `TestEditorialTagManagementPostgresHTTP` 通过。对隔离临时表运行真实 Management 路由及现有合集触发器，覆盖 UUIDv7、Code、双语搜索、请求防重和审计、归档保护、主次角色、失败回滚、自动合集成员和 Revision。游戏状态及发布审计保持原状 |
| Admin API、Web 和 Embed | `task check:api`、`task check:web`、`task check:embed` 通过 |
| Admin 浏览器 | `gofurry-tag-management.spec.ts` 1 项通过。原生 Vite 与真实 Admin、GoFurry 服务在 Windows loopback 联调，使用现有私有 YAML 的 VPN PostgreSQL 连接和会话隔离临时表。验证真实登录、中文与英文显示、修改、筛选、归档与恢复、审计、匿名及 Developer 拒绝写入、亮色和 390px 暗色布局 |

临时数据随测试会话释放。未导入共享标签，未修改生产环境。Docker、Docker Compose、Podman、Testcontainers 使用次数均为 0。
