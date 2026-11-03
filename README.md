# GoFurry

V0.1-B3-F2 gives `/start` distinct Light/Dark flat acrylic materials and a compact user menu, retaining the legacy SVG/geometry, circular theme reveal and verified Go/R2 background integration. No extension, accounts or business database yet.

## Setup and development

Pinned tools: Node **24.22.1**, pnpm **12.8.1**, Go **1.27.3**, Task **3.53.1**. Root version files and `package.json` are authoritative; `task tools` checks consistency and Go configuration/JSON rules. Task selects `GOTOOLCHAIN=go1.27.3`; direct Go commands must use that version too.

Run `task install`. Copy `config.example.yaml` to `config.yaml` once (PowerShell: `Copy-Item config.example.yaml config.yaml`), checking first that no private file exists. Then use separate terminals:

```sh
task dev:api
task dev:web
```

- Web: <http://127.0.0.1:17320>; development `/api/**` proxies through Nitro route rules to API port 17321. Production has no general automatic API proxy or cross-region fallback.
- API: <http://127.0.0.1:17321/api/v1/status>. Both expose `/livez` and `/readyz`.
- Standalone API: `bin/api --config config.yaml` (`bin/api.exe` on Windows). Probe: `bin/api healthcheck --config config.yaml`. The probe derives host/port from that YAML, including wildcard listeners.
- Production Web: `task build:web`, then `pnpm --filter @gofurry/web start`. For local Node, set `HOST=127.0.0.1 PORT=17320` (PowerShell: `$env:HOST='127.0.0.1'; $env:PORT='17320'`). The container retains internal port 3000.

## API configuration and JSON

`config.example.yaml` defines one schema: required `server.address`, `server.environment` (`development` / `production`), `deployment.region` (`CN` / `INTL`) and positive `logging.slow_request` (e.g. `1s`), plus optional `storage`. Unknown YAML fields fail. Cobra requires `--config` for serving, probing and importing. Viper reads exactly that file: no automatic search, merging, environment bindings, .env, or individual business-field flag overrides. Private `config.yaml` is ignored everywhere and excluded from build contexts.

Fiber uses standard-library `encoding/json/v2` and `encoding/json/jsontext` through non-variadic adapters. Existing status/error fields and null collections are preserved. Invalid UTF-8, duplicate JSON names and trailing JSON values are rejected; no Sonic dependency or JSON experiment flags remain.

## Region, language and theme

`WEB_REGION=CN` (default) or `WEB_REGION=INTL` selects the Web deployment. Set it in the local shell or Web Compose environment. API region is selected only in its YAML. Operators must keep them in the same region and use separate hosts, secrets, databases and user-data storage for CN/INTL. Reviewed public SFW backgrounds may be shared between regions. There are currently no user-data readers or database connections; future data clients must stay within their deployment with no cross-region fallback.

CN defaults to `zh`, INTL to `en`; both offer both languages. Locale never selects a region. Both currently expose SFW policy metadata; CN remains permanently SFW. INTL NSFW is deferred and has no configuration switch or implementation here.

The diagnostic page has minimal Light/Dark and language selectors. Host-only `gf-theme` / `gf-locale` cookies remember preferences for one year; SSR and hydration use the same sanitized snapshot. Initial theme is Light without a preference, independently of OS theme. Invalid cookies fall back safely. Personalized HTML and preference responses are private/no-store. There is no localStorage second source or client-only theme flash.

Consumed StyleX semantic tokens come from the fixed legacy `apps/cn/web/app/assets/styles/tokens.less`: warm beige/charcoal canvas, surfaces/text, Manrope + Noto Sans SC, mono role, dot grid, radii and motion. Fonts are bundled locally without LESS, Nuxt or Tailwind. Reduced-motion is respected. This establishes global foundations, not a new homepage or shared UI package.

## Start page

Open <http://127.0.0.1:17320/start> after `task dev:api` and `task dev:web`. The homepage `/` is unchanged. The search group is horizontally centered near 27% of the first viewport, with the legacy 500px input width. Five categories sit above it; desktop platform buttons and the mobile inline expanding grid sit below. Provider IDs remain stable across locale changes. CN defaults to Bing, INTL to Google. Enter or the search icon opens an encoded, trimmed query in a new tab; the default native GET form also works without JavaScript. Composition-confirming Enter never submits. Escape closes the mobile picker; arrows navigate its options, and outside focus/click or a desktop breakpoint closes it with appropriate focus restoration.

The Logo preserves the original `apps/cn/web/app/assets/svgs/logo-mini.svg` paths/viewBox from the fixed legacy reference. Its only asset edit replaces the fixed gray fill with `currentColor`; the unchanged alpha mask uses StyleX `logoBrand` (near-black Light, light-gray Dark). Top-right actions are locale, theme, GoFurry GitHub and User. Their flat acrylic has no border, inset highlight or shadow, with distinct translucent Light/Dark surfaces/text and 300ms hover transitions. Dimensions/typography stay fixed for 中 and EN. Sun means the current Light theme; Moon means Dark; labels describe the target theme.

User opens a right-aligned Base UI Menu with only Sign in and Settings. Both are disabled, focusable placeholders with localized unavailable labels and no navigation, dialog or toast. Base UI owns outside dismissal, keyboard navigation and Escape focus restoration. Its starting/ending attributes drive 420ms opacity/4px vertical transitions; the popup stays mounted through exit. Reduced-motion uses no animation. There is no separate Settings button or Identity integration.

`start/theme-reveal.ts` adapts the legacy controller without moving theme/cookie ownership out of preferences. React `flushSync` replaces Vue's `nextTick` before the new snapshot. Native View Transitions reveal the new root above the old from the button center to the farthest viewport corner, using 420ms and `cubic-bezier(0.22, 1, 0.36, 1)`. Unsupported API/reduced-motion switches directly. Repeated clicks are guarded; resize, visibility/page changes and unmount release snapshot state. Only color/surface CSS transitions are settled; the R2 opacity fade remains independent.

`start/search.stylex.ts` owns separate Light/Dark material tokens, applied to the search subtree from the SSR-consistent preference. Classification, selected platforms, input/placeholder and mobile options all change color together in the native reveal. The migrated geometry remains: 12px chip radii, 48px borderless input, natural desktop wrapping and 44px/6px mobile options. Surfaces remain translucent without embossed highlights; 300ms hover transitions have no scale/position changes. One keyboard-only input ring replaces stacked shadows; pointer focus only brightens the material. Without suggestions, native input Up/Down is not prevented. The wallpaper is unaffected by theme changes.

`apps/web/src/start/search-platforms.ts` owns the small reviewed provider allowlist. Adult/mixed adult-art providers are excluded in both regions; general engines request strict SafeSearch. WikiFur, Epic and ZCool remain, while Wikisource/Project Gutenberg replace the legacy novel providers. External sites own their moderation. Remote suggestions are disabled: no old API, pending debounce/request, loading UI or fabricated results exists. Input blur, composition and category changes cannot leave stale suggestions. A future real source must reintroduce abort/request-generation checks before enabling suggestions.

CN alone renders small, low-contrast company/filing text with a centered · separator, dark in Light and light-gray in Dark with a restrained opposing text shadow for readability; there is no fill, card, padding or backdrop blur. INTL renders no footer. Public filing metadata is explicit in `apps/web/src/start/filing.config.ts`, with no environment lookup or Go configuration change. Both numbers remain `null`; local review shows non-link pending labels. Before publication, disable `showPending`; populate only this site's approved ICP number and the police platform's issued number, local icon and official query URL. The ICP target is fixed to `https://beian.miit.gov.cn/`. No old-domain registration or invented number/icon is used.

`GET /api/v1/start/backgrounds` returns the reviewed catalog with live object HEAD metadata. It never proxies image bytes. After hydration, a Start server function fetches metadata from this deployment's `WEB_API_BASE_URL` (default `http://127.0.0.1:17321`; Compose uses `http://api:8080`). It forwards no user cookies and rejects a mismatched API region/SFW policy. There is no alternate-region fallback. The browser fetches the first background directly from the public CDN: decoded WebP first, then decoded AVIF fades in over it; failures retain the preview or theme. The theme/dots remain the SSR/loading fallback. B3 retains this loader and both image objects unchanged, without a background selector or user-data access. Both regions may use these reviewed public SFW assets.

Private YAML `storage` contains `endpoint`, `region`, `bucket`, `access_key_id`, `secret_access_key`, `public_base_url`; all six are required when enabled and URLs must be HTTPS. `storage: null` returns an empty catalog and preserves the theme-only mode. AWS SDK for Go v2 uses static credentials from that YAML, without loading AWS profiles, environment credentials or .env. The concrete adapter uses S3-compatible configuration; COS operations and replication are deferred.

Development-only import: `bin/api background-import <file.avif-or.webp> --config config.yaml`. It derives `start/backgrounds/<full-sha256>.<extension>`, checks HEAD, and uses atomic `If-None-Match: *` writes. Existing keys are never overwritten; both new and existing objects must pass R2 HEAD/GET plus public CDN GET type/length/hash checks. Import first checks that the YAML environment is `development`. Curated public keys/dimensions/hashes live in `services/api/internal/handlers/backgrounds.json`; credentials and source/preview image files do not belong in Git. Only import into a separately verified development Bucket/CDN; legacy production objects are read-only.

Focused checks: `task build:web`, `pnpm --filter @gofurry/web typecheck`, then `pnpm --filter @gofurry/web exec playwright test start.spec.ts --workers=1`. Repeat with `WEB_REGION=INTL`. The nine Start cases cover SSR/themed SVG/native defaults, fixed action geometry/current-theme icons/cookies, pointer versus keyboard focus, provider query/path encoding, IME/native input arrows, actual mobile-provider submission/focus/resize, four actions down to 320px, CN-only filing text, native reveal geometry/synchronized colors/interruption/fallbacks, actual Light/Dark computed materials and menu exit/keyboard/placeholders on desktop/mobile. External search/GitHub navigation is intercepted locally. Use the existing `task check:hmr` after StyleX changes; full CI, unrelated Go tests and Docker builds are not needed for B3.

Live background check, with the configured API already running: `pnpm --filter @gofurry/web exec playwright test --config playwright.background.config.ts`. It starts only a loopback Web test process on 17322, uses real Go/R2/CDN responses, holds then releases the actual AVIF request, checks decoding/fade/screenshots, and aborts the AVIF once to verify WebP retention/search. Set `PLAYWRIGHT_CHANNEL=chrome` for installed Chrome. For INTL, run a separate API from its own `--config` YAML and set test-only `START_API_URL` plus `WEB_REGION=INTL`. Normal CI does not require private storage credentials. See [B2 evidence](docs/v0.1-b2.md) for the actual development import and focused results.

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
