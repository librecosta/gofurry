# GoFurry

V0.1-B0 engineering governance on the existing TanStack Start / React / StyleX Web and Go/Fiber API. No /start page, extension, accounts or business database yet.

## Setup and development

Pinned tools: Node **24.22.1**, pnpm **12.8.1**, Go **1.27.3**, Task **3.53.1**. Root version files and `package.json` are authoritative; `task tools` checks consistency and Go configuration/JSON rules. Task selects `GOTOOLCHAIN=go1.27.3`; direct Go commands must use that version too.

Run `task install`. Copy `config.example.yaml` to `config.yaml` once (PowerShell: `Copy-Item config.example.yaml config.yaml`), checking first that no private file exists. Then use separate terminals:

```sh
task dev:api
task dev:web
```

- Web: <http://127.0.0.1:17320>; development `/api/**` proxies through Nitro route rules to API port 17321. Production has no automatic API proxy or cross-region fallback.
- API: <http://127.0.0.1:17321/api/v1/status>. Both expose `/livez` and `/readyz`.
- Standalone API: `bin/api --config config.yaml` (`bin/api.exe` on Windows). Probe: `bin/api healthcheck --config config.yaml`. The probe derives host/port from that YAML, including wildcard listeners.
- Production Web: `task build:web`, then `pnpm --filter @gofurry/web start`. For local Node, set `HOST=127.0.0.1 PORT=17320` (PowerShell: `$env:HOST='127.0.0.1'; $env:PORT='17320'`). The container retains internal port 3000.

## API configuration and JSON

`config.example.yaml` defines one schema: `server.address`, `server.environment` (`development` / `production`), `deployment.region` (`CN` / `INTL`) and positive `logging.slow_request` (e.g. `1s`). All fields are required; unknown YAML fields fail. Cobra requires `--config` for serving and probing. Viper reads exactly that file: no automatic search, merging, environment bindings, .env, or individual business-field flag overrides. Private `config.yaml` is ignored everywhere and excluded from build contexts.

Fiber uses standard-library `encoding/json/v2` and `encoding/json/jsontext` through non-variadic adapters. Existing status/error fields and null collections are preserved. Invalid UTF-8, duplicate JSON names and trailing JSON values are rejected; no Sonic dependency or JSON experiment flags remain.

## Region, language and theme

`WEB_REGION=CN` (default) or `WEB_REGION=INTL` selects the Web deployment. Set it in the local shell or Web Compose environment. API region is selected only in its YAML. Operators must keep them in the same region and use separate hosts, secrets, databases and storage for CN/INTL. There are currently no user-data readers or database connections; future data clients must stay within their deployment with no cross-region fallback.

CN defaults to `zh`, INTL to `en`; both offer both languages. Locale never selects a region. Both currently expose SFW policy metadata; CN remains permanently SFW. INTL NSFW is deferred and has no configuration switch or implementation here.

The diagnostic page has minimal Light/Dark and language selectors. Host-only `gf-theme` / `gf-locale` cookies remember preferences for one year; SSR and hydration use the same sanitized snapshot. Initial theme is Light without a preference, independently of OS theme. Invalid cookies fall back safely. Personalized HTML and preference responses are private/no-store. There is no localStorage second source or client-only theme flash.

Consumed StyleX semantic tokens come from the fixed legacy `apps/cn/web/app/assets/styles/tokens.less`: warm beige/charcoal canvas, surfaces/text, Manrope + Noto Sans SC, mono role, dot grid, radii and motion. Fonts are bundled locally without LESS, Nuxt or Tailwind. Reduced-motion is respected. This establishes global foundations, not a new homepage or shared UI package.

## Local containers and logs

Copy `deploy/compose/config.example.yaml` to `deploy/compose/config.yaml` once. Its schema is identical to local configuration, with internal port 8080 and production logging. Select its region to match `WEB_REGION`.

```sh
docker compose -f deploy/compose/compose.yaml config --quiet
docker compose -f deploy/compose/compose.yaml up --build --wait
docker compose -f deploy/compose/compose.yaml down
```

Only `gofurry-local` is managed. Host ports default to loopback **17320/17321**, overridable with `WEB_PORT` / `API_PORT`; internal ports stay 3000/8080. `API_CONFIG_FILE` optionally chooses the bind-mount source path, not application fields. YAML is mounted read-only at `/etc/gofurry/config.yaml`; there is no API Compose `environment` / `env_file`. Missing files fail instead of creating directories. Web region is a server-only Node setting, not a client-controlled Vite variable.

Images run non-root, have readiness probes and a 20-second stop window. Logs go to stdout/stderr; Docker `local` rotates at **5m × 3**, compressed. Production suppresses fast successful requests and probes; errors (including failed probes), slow requests and lifecycle events remain. API slow threshold comes from YAML; Web uses one second. Request logs allowlist route templates/status/duration/IDs, excluding credentials and bodies. Framework banner lines may remain plain text.

If Docker Hub is unreachable, the equivalent official image mirror is supported:

```sh
docker compose -f deploy/compose/compose.yaml build --build-arg NODE_IMAGE=public.ecr.aws/docker/library/node:24.22.1-bookworm-slim --build-arg GO_IMAGE=public.ecr.aws/docker/library/golang:1.27.3-bookworm
```

`deploy/nginx/gofurry.conf.example` remains inactive. No Docker daemon settings or production deployments are managed here.

## Verification

```sh
task tools
task check:api
task build:web
pnpm --filter @gofurry/web typecheck
pnpm --filter @gofurry/web test:e2e
task check:hmr
task check:docker
task check:containers
```

Check INTL defaults separately by setting `WEB_REGION=INTL` and running `pnpm --filter @gofurry/web exec playwright test governance.spec.ts`. Remove that setting before CN/HMR checks. Browser setup: `pnpm --filter @gofurry/web exec playwright install chromium`; an installed Chrome may be selected with `PLAYWRIGHT_CHANNEL=chrome`.

These are focused runtime/configuration/SSR checks, not a multi-browser business matrix. `task check` remains the small general repository entry point. HMR temporarily edits/restores the diagnostic route; do not edit it concurrently. Container smoke uses example YAML, random ports and its own disposable project, checks the mount/log policy/SIGTERM, then removes only its resources. It never overwrites private YAML.

See [B0 verification](docs/v0.1-b0.md), [architecture](.agents/architecture.md) and [historical V0.1-A verification](docs/v0.1-a.md). Legacy source is read-only and excluded from builds/CI. Issue #1 tracks deferred integrations; Issue #2 defers shared UI/Go libraries.
