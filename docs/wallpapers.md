# Official wallpapers

Each deployment owns `start_wallpapers` and `wallpaper_audit_logs`; no Admin database or identity integration. UUIDv7 IDs, bound pgx/sqlc queries and explicit Goose migrations are authoritative. Configure one private YAML; optional `database: null` and `management: null` keep other APIs independent. Never inherit PG*/DATABASE_URL credentials.

`task migrate ACTION=status|up CONFIG=<absolute YAML>` maintains only the configured deployment. Status is read-only; serve never migrates. `task sql:generate` / `task sql:check` use pinned sqlc. CN/INTL need physically separate databases and credentials.

Management is a separate App, never registered on Public, and requires database, storage, a distinct `127.0.0.1`/`::1` address and exactly 32 crypto/rand bytes as canonical unpadded base64url in `management.token`. No CORS, Cookie authentication, arbitrary object operations or forwarded-client trust. All routes require Bearer plus `X-LC-Actor-ID/Name/Role`, UUID `X-Request-ID`, matching `X-LC-Region/Environment`. Actor claims are audit snapshots from the authenticated Admin service, not separate proof of a human identity or permission. Scope is limited to official wallpapers; Admin capability enforcement arrives in C3.

`GET /internal/v1/readyz` checks database and schema. Public readiness remains about the public lifecycle, so optional wallpaper infrastructure does not disable search/weather. Both Apps share one pool and Service; shutdown drains both before closing shared resources. Management has a 120/minute global budget, 16 active connections, bounded I/O and 90-second operation deadlines. Public retains its existing 1 MiB body limit.
