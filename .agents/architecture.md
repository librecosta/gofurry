# Runtime boundaries

- `apps/web`: TanStack Start + React SSR, StyleX and one Base UI Popover. Nitro's Node preset produces a self-contained `.output`. The Web does not depend on API availability to render or report health.
- `services/api`: a single Go module. `cmd/api` owns configuration, logging and signal context. `internal/server` owns Fiber, readiness and shutdown hooks. Middleware is assembled under `internal/middleware`; local configuration/logging are under `internal/platform`. `routers` maps HTTP URLs to `internal/handlers`.
- Middleware order: Request ID → Contrib Zerolog → Recover → Helmet → probes/routes. The outer logger asks Fiber's error handler to render before logging, so errors and panics receive their actual status and correlated ID. Logs use an allowlist of fields.
- API readiness becomes true in `OnListen`, false in `OnPreShutdown`. `GracefulContext` drains requests for up to 10 seconds; post-shutdown hooks own future resource cleanup. No database, scheduler or other owned resource exists yet. No prefork or trusted proxy configuration is enabled.
- Web themes use CSS `prefers-color-scheme`, so server markup and initial client markup agree. Production CSS is linked from SSR HTML; development adds the official StyleX virtual stylesheet and CSS-only HMR subscriber. Keep Start before React; StyleX transforms must run before React Fast Refresh.
- Compose binds separate Web/API host ports on loopback, gives each service its own image/probe and a 20-second stop window. It has no production integrations. `deploy/nginx` is an inactive local example.

Do not turn reference template directories into empty scaffolding. Add business boundaries, persistence and reusable packages only when real functionality requires them.
