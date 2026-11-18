# GoFurry 与 Admin 容器运行

本地开发照常使用 Go 和 Vite 命令。服务器使用相同程序和 YAML。GoFurry API、SSR Web 和可选 Admin 各自运行，统一使用 [Compose](../compose/compose.yaml)。需要 Docker Engine 28+、Compose 2.33.1+ 和已有的 `librecosta-backend`。 [环回端口](https://docs.docker.com/engine/network/port-publishing/) · [默认路由](https://docs.docker.com/reference/compose-file/services/#gw_priority)

- 在仓库外准备私有 YAML，可参考 [GoFurry API](gofurry.example.yaml)、[Web](../compose/web.example.yaml) 和 [Admin](admin.example.yaml)。`API_CONFIG_FILE`、`WEB_CONFIG_FILE` 和 `ADMIN_CONFIG_FILE` 指向各自的运行文件。API 与 Admin 通过 `librecosta-postgres:5432` 使用独立 Runtime 角色。GoFurry Redis 使用 `librecosta-redis:6379` 和自己的 ACL。Goose 维护使用单独的 Migrator YAML，按 [数据库契约](database/README.md) 显式执行。
- 默认在宿主 `127.0.0.1` 发布 Web 17320、API 17321、Admin 8082。应用在容器内分别监听 3000、8080、8082。SSR 通过 `http://gofurry-api:8080` 访问 GoFurry API。配置公开站点时，设定实际的 `public_origin`，由现有宿主入口按同源转发 Web 和浏览器请求的 `/api/`。只有在该入口覆写转发头时才配置实际的受信代理地址。
- 启用 Admin 时，两侧 YAML 使用同一个私有管理令牌，Admin 的 `gofurry.base_url` 指向 `http://gofurry-api:17324`。管理端口仅在容器网内使用。生产 Admin 登录使用与 `auth.trusted_origins` 完全一致的受信 HTTPS 环回来源，保留 Secure Session Cookie 和 Origin/CSRF 校验。

在宿主机运行 `task build:linux` 和 `task build:web`，再用现有 Dockerfile 打包。API 使用静态 Linux 二进制，Web 仅需要 Node Runtime。Admin 镜像在 Admin 仓库完成宿主机 Web 与 Embed 编译后打包，启用时以 `ADMIN_IMAGE` 指定。Go 工具链必须预装在 PATH，Task 禁止自动下载工具链。

```sh
docker compose -f deploy/compose/compose.yaml config --quiet
docker compose -f deploy/compose/compose.yaml up -d --build --wait api web
docker compose -f deploy/compose/compose.yaml --profile ops up -d --wait admin
```
