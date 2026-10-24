# GoFurry

V0.1-A engineering foundation: a minimal SSR Web and a Go/Fiber API. No product homepage, accounts or database schema yet.

## Local development

Use Node **24.22.1**, Go **1.26.10**, pnpm **12.8.1**, and Task **3.53.1** (root version files and `package.json`). Install the pinned pnpm with `npm install --global pnpm@12.8.1` if needed. Task sets `GOTOOLCHAIN=go1.26.10`, allowing a newer Go installation to download and use the required toolchain. Direct Go commands must use the pinned Go as well.

```sh
task install
pnpm --filter @gofurry/web exec playwright install chromium
task dev:web
# In a second terminal:
task dev:api
```

Web: <http://127.0.0.1:3000>. API: <http://127.0.0.1:8080/api/v1/status>. Both expose `/livez` and `/readyz`. API defaults to development console logs; set `APP_ENV=production` for JSON and `API_ADDR` to change its listener. Web production uses Nitro runtime hooks for JSON access/lifecycle logs; Vite development logs remain console-oriented.

## Verification

```sh
task check
task check:hmr
task check:docker
task check:containers
```

`task check` verifies tool versions, Go formatting/vet/tests/build, production Web build/types and Playwright SSR/interaction tests. It requires Chromium installed above. When download access is limited, an installed Google Chrome can be selected with `PLAYWRIGHT_CHANNEL=chrome` (PowerShell: `$env:PLAYWRIGHT_CHANNEL='chrome'`). HMR verification temporarily edits the new index route and restores it in `finally`; do not edit that file concurrently. `task check:containers` uses prebuilt local images, random host ports and a disposable Compose project to check probes, logs, rotation settings and SIGTERM exit/close hooks, then removes only its own containers/network.

Standalone production Web: `task build:web`, then `pnpm --filter @gofurry/web start`. It defaults to port 3000; Nitro accepts `HOST` / `PORT`. API binary: `bin/api` (`bin/api.exe` on Windows) after `task check:api`.

## Local containers

```sh
docker compose -f deploy/compose/compose.yaml up --build --wait
docker compose -f deploy/compose/compose.yaml down
```

Only the `gofurry-local` project is managed by these commands. Ports default to loopback 3000/8080, overridable with `WEB_PORT` / `API_PORT`. Each image has a non-root runtime and a readiness healthcheck. SIGTERM uses the framework's graceful shutdown; Compose allows 20 seconds. Application output goes to stdout/stderr; Docker `local` rotates at 10 MiB × 5 with compression.

If Docker Hub is unreachable, use the same pinned official images from AWS's Docker Official Images mirror:

```sh
docker compose -f deploy/compose/compose.yaml build --build-arg NODE_IMAGE=public.ecr.aws/docker/library/node:24.22.1-bookworm-slim --build-arg GO_IMAGE=public.ecr.aws/docker/library/golang:1.26.10-bookworm
docker compose -f deploy/compose/compose.yaml up --no-build --wait
```

There is no PostgreSQL dependency. `deploy/nginx/gofurry.conf.example` is optional and inactive; production TLS, proxy trust and deployment automation are deferred.

## Layout

| Path | Responsibility |
| --- | --- |
| `apps/web` | SSR routes, local StyleX tokens, Base UI smoke component, browser checks |
| `services/api` | Fiber monolith, handlers, probes, middleware, lifecycle |
| `deploy` | Independent Docker builds, local Compose, inactive Nginx example |
| `.agents` | Concise architecture constraints |
| `docs/v0.1-a.md` | Verified versions, acceptance evidence and remaining limits |
| `third-party/go-furry` | Existing read-only Submodule, excluded from build/CI |

The legacy Submodule is optional for normal development: `task install` and builds never initialize it. See [Issue #1](https://github.com/librecosta/gofurry/issues/1) for deferred integration decisions and [Issue #2](https://github.com/librecosta/gofurry/issues/2) for the decision to postpone shared libraries.
