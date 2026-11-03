# GoFurry

The current `/start` has a bounded single-viewport layout, a constant-gray interactive legacy Logo, clean filing text and distinct Light/Dark acrylic materials, retaining the search, theme reveal and verified Go/R2 background integration. No extension, accounts or business database yet.

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

`config.example.yaml` defines one schema: required `server.address`, `server.environment` (`development` / `production`), `deployment.region` (`CN` / `INTL`) and positive `logging.slow_request` (e.g. `1s`), plus optional `storage` and `search_suggestions`. Unknown YAML fields fail. Cobra requires `--config` for serving, probing and importing. Viper reads exactly that file: no automatic search, merging, environment bindings, .env, or individual business-field flag overrides. Private `config.yaml` is ignored everywhere and excluded from build contexts.

Fiber uses standard-library `encoding/json/v2` and `encoding/json/jsontext` through non-variadic adapters. Existing status/error fields and null collections are preserved. Invalid UTF-8, duplicate JSON names and trailing JSON values are rejected; no Sonic dependency or JSON experiment flags remain.

## Region, language and theme

`WEB_REGION=CN` (default) or `WEB_REGION=INTL` selects the Web deployment. Set it in the local shell or Web Compose environment. API region is selected only in its YAML. Operators must keep them in the same region and use separate hosts, secrets, databases and user-data storage for CN/INTL. Reviewed public SFW backgrounds may be shared between regions. There are currently no user-data readers or database connections; future data clients must stay within their deployment with no cross-region fallback.

CN defaults to `zh`, INTL to `en`; both offer both languages. Locale never selects a region. Both APIs currently expose SFW policy metadata; CN remains permanently SFW. INTL may remember an SFW/NSFW preference, while actual backend content and suggestions remain SFW. No adult-content query module or account integration exists.

Start URLs own language: CN `/start` (zh), `/en/start` (en); INTL `/start` (en), `/zh/start` (zh). `src/locale-path.ts` centralizes prefix construction for future content routes. Explicit URLs are never redirected by Cookie, IP or browser language; `gf-locale` only remembers the choice. Both versions send noindex metadata and X-Robots-Tag and stay out of sitemaps. Future indexable pages must supply independent canonical/hreflang. Other localized business pages are not implemented; the existing engineering diagnostics retain their preference controls.

Theme preference is `system` (first visit), `light` or `dark`, persisted in host-only `gf-theme` for one year. StyleX system themes contain complete Light/Dark values selected by CSS media queries before JavaScript, so SSR does not guess the OS or produce a mismatching hydration tree. React tracks the resolved OS theme only in system mode; manual choices remain fixed. Personalized responses remain private/no-store, without a localStorage second source. INTL's `gf-content-preference` remembers only a future preference; CN ignores NSFW and renders no such controls. API SFW policy is unchanged.

Consumed StyleX semantic tokens come from the fixed legacy `apps/cn/web/app/assets/styles/tokens.less`: warm beige/charcoal canvas, surfaces/text, Manrope + Noto Sans SC, mono role, dot grid, radii and motion. Fonts are bundled locally without LESS, Nuxt or Tailwind. Reduced-motion is respected. This establishes global foundations, not a new homepage or shared UI package.

## Start page

Open <http://127.0.0.1:17320/start> after `task dev:api` and `task dev:web`. The homepage `/` is unchanged. The search group is horizontally centered near 27% of the first viewport, with the legacy 500px input width. Five categories sit above it; desktop platform buttons sit below. On mobile, the selected platform and CaretDown occupy the left end of the same input, with a left-aligned menu below it. Provider IDs remain stable across locale changes. CN defaults to Bing, INTL to Google. Enter or the search icon opens an encoded, trimmed query in a new tab; the default native GET form also works without JavaScript. Composition-confirming Enter never submits. Escape closes the mobile picker; arrows navigate its options, and outside focus/click or a desktop breakpoint closes it with appropriate focus restoration.

Start alone uses a fixed, isolated `100dvh` viewport with a full-height main, dynamic viewport spacing and compact spacing for short windows. Its body overrides and the root dot-height cap are scoped by `:has([data-start-page])`; other routes retain normal scrolling. Background layers are absolute at z-index 0 inside that viewport, below the search/header/footer. This removes the root 720px dot overflow on short desktops and the main's excessive minimum-height/padding combination without hiding document scrollbars or remounting images on resize.

The Logo preserves the original SVG alpha mask, constant light-gray color, geometry and 300ms flat hover. Its Base UI Popover contains Home, Sites and Games, with locale-aware target paths. Only implemented destinations are linked: default-language Home opens the unchanged `/` engineering page; missing localized Home/Sites/Games remain unavailable. The portal positioner uses a fixed strategy before/after measurement and an explicit width, without coordinate transitions or render timers. Base UI hides the unpositioned pass; the popup owns 420ms opacity/4px motion. Initial visible-frame bounds and focus restoration are browser-tested.

Top-right actions are only Settings (GearSix) and User (UserCircle), retaining their flat acrylic and dimensions. User's Base UI Menu contains one disabled Sign in placeholder. Settings opens a compact Base UI Dialog with language, system/light/dark appearance and GitHub; INTL additionally offers SFW/NSFW with an explicit current-SFW-only explanation. Base UI RadioGroup owns keyboard choices; Dialog owns focus trapping, outside/Escape dismissal and restoration. Dialog/backdrop and popovers complete 420ms exits; reduced-motion disables motion. A shared `_start` pathless route shell preserves dialog, query, provider and decoded wallpaper across language URLs. No Identity integration is added.

`start/theme-reveal.ts` retains the legacy 420ms circle reveal, now centered on the selected appearance control. React `flushSync` settles the complete new theme before the snapshot. Unsupported API/reduced-motion switches directly. Repeated clicks are guarded; resize, visibility/page changes and unmount release snapshot state. Only color/surface transitions are settled; R2 opacity fading stays independent.

`start/search.stylex.ts` owns separate Light/Dark and media-query system material tokens. Existing geometry and transparent materials remain: 48px input, desktop platform wrapping and inline mobile selector. Toolbar hover stays at 300ms; search selection/focus and panels use 420ms. Start alone hides the native search cancel button and provides a localized Phosphor X, separated from Submit; clearing cancels suggestions and restores focus. Narrow category chips use smaller type/padding/gaps; remaining overflow has directional edge cues and focus/selection auto-scrolling, without document overflow. One keyboard-only input ring replaces stacked shadows. Native Up/Down remains available without suggestions, and wallpapers are unaffected by theme changes.

`apps/web/src/start/search-platforms.ts` owns the small reviewed provider allowlist. Adult/mixed adult-art providers are excluded in both regions; general engines request strict SafeSearch. WikiFur, Epic and ZCool remain, while Wikisource/Project Gutenberg replace the legacy novel providers. External sites own their moderation. `GET /api/v1/start/search/suggestions?q=...` calls DuckDuckGo's official `/ac/` through the regional Go API, independently of the selected search destination. Only the Search category requests suggestions, after 600ms. AbortController, request generations and IME guards prevent stale results; arrows select, Enter/click searches the current platform, and Escape/outside/blur closes. Empty/error/timeout/429 responses leave no empty/loading/error UI. Platform choices and suggestions share one animated panel, so they cannot overlap. Native Up/Down remains available without results.

Online suggestions require explicit YAML `search_suggestions.enabled: true`. The example is disabled. `proxy_url: ""` connects directly; an explicit HTTP(S)/SOCKS URL selects a proxy, and HTTP_PROXY/HTTPS_PROXY are never read. The Go client limits queries to 128 characters, upstream requests to 3 seconds/64KiB/4 concurrent calls, results to 8 deduplicated phrases, and its TTL cache to 256 entries/10 minutes. Official Fiber limiters bound each API client to 60 requests/minute and the deployment to 240/minute. Behind the existing Web relay, clients share that Web source IP; no untrusted forwarded-IP header is accepted. The targeted same-origin Web server route forwards no cookies/proxy configuration and rejects an API region/SFW mismatch; it works in production as well as development without adding a general API proxy.

DuckDuckGo autocomplete is **not guaranteed by Safe Search** ([official deployment guidance](https://duckduckgo.com/duckduckgo-help-pages/get-duckduckgo/deploy-duckduckgo)). Both regions apply a conservative adult-term filter; this is not a complete moderation system. CN additionally requires a nonempty `cn_allowlist` of reviewed exact phrases and returns only their intersection with real upstream results. Leave CN disabled if reliable review is unavailable; locale cannot bypass this policy. INTL suggestions stay SFW regardless of the saved content preference. In F5 verification, enabled local CN YAML and a working explicit proxy returned `[]` for `furry` because none of the eight reviewed technical phrases matched the upstream results. A separate INTL YAML returned eight furry phrases; Chrome displayed and submitted a selected phrase to Google. The CN list was not expanded.

CN alone renders small company/filing text with a centered · separator: slightly translucent light text in Light and dark text in Dark, without text shadow, stroke or filter. There is no fill, card, padding or backdrop blur. INTL renders no footer. Public filing metadata is explicit in `apps/web/src/start/filing.config.ts`, with no environment lookup or Go configuration change. Both numbers remain `null`; local review shows non-link pending labels. Before publication, disable `showPending`; populate only this site's approved ICP number and the police platform's issued number, local icon and official query URL. The ICP target is fixed to `https://beian.miit.gov.cn/`. No old-domain registration or invented number/icon is used.

`GET /api/v1/start/backgrounds` returns the reviewed catalog with live object HEAD metadata. It never proxies image bytes. After hydration, a Start server function fetches metadata from this deployment's `WEB_API_BASE_URL` (default `http://127.0.0.1:17321`; Compose uses `http://api:8080`). It forwards no user cookies and rejects a mismatched API region/SFW policy. There is no alternate-region fallback. The browser fetches the first background directly from the public CDN: decoded WebP first, then decoded AVIF fades in over it; failures retain the preview or theme. The theme/dots remain the SSR/loading fallback. B3 retains this loader and both image objects unchanged, without a background selector or user-data access. Both regions may use these reviewed public SFW assets.

Private YAML `storage` contains `endpoint`, `region`, `bucket`, `access_key_id`, `secret_access_key`, `public_base_url`; all six are required when enabled and URLs must be HTTPS. `storage: null` returns an empty catalog and preserves the theme-only mode. AWS SDK for Go v2 uses static credentials from that YAML, without loading AWS profiles, environment credentials or .env. The concrete adapter uses S3-compatible configuration; COS operations and replication are deferred.

Development-only import: `bin/api background-import <file.avif-or.webp> --config config.yaml`. It derives `start/backgrounds/<full-sha256>.<extension>`, checks HEAD, and uses atomic `If-None-Match: *` writes. Existing keys are never overwritten; both new and existing objects must pass R2 HEAD/GET plus public CDN GET type/length/hash checks. Import first checks that the YAML environment is `development`. Curated public keys/dimensions/hashes live in `services/api/internal/handlers/backgrounds.json`; credentials and source/preview image files do not belong in Git. Only import into a separately verified development Bucket/CDN; legacy production objects are read-only.

Focused checks: `task build:web`, `pnpm --filter @gofurry/web typecheck`, then `pnpm --filter @gofurry/web exec playwright test start.spec.ts start-suggestions.spec.ts --workers=1`. Repeat with `WEB_REGION=INTL`. Cases cover URL-authoritative SSR language/noindex, pre-JS system colors, theme persistence/reveal, dialog focus and complete exits, content-preference boundaries, first-frame navigation placement, custom clearing, 320/375/390/768px categories and suggestion IME/races/selection. External navigation is intercepted locally. Run `task check:hmr` after StyleX changes. Full CI, unrelated Go tests and Docker builds are unnecessary for these refinements.

Live checks, with the configured API running: `pnpm --filter @gofurry/web exec playwright test --config playwright.background.config.ts`. Enable suggestions explicitly in private YAML (CN needs reviewed golang phrases; INTL tests real furry results). The loopback Web test process on 17322 verifies Go/CDN responses, resize coverage, stable image identity/request counts, WebP-to-AVIF decoding, failure fallback and actual suggestion submission. It captures desktop Light/Dark settings and 320/390px settings/navigation. Set `PLAYWRIGHT_CHANNEL=chrome`. INTL requires its own API `--config` YAML plus test-only `START_API_URL` and `WEB_REGION=INTL`; no alternate-region fallback is used. Normal CI requires no private storage credentials. See [B2 evidence](docs/v0.1-b2.md) for the unchanged asset import.

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
