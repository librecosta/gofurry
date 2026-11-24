# STEAM-R2-A · Full game details acceptance

**Result · PASS for the isolated end-to-end release path**

**Date · 2026-11-24, Asia/Shanghai**

| Repository | Starting develop | R2 implementation |
| --- | --- | --- |
| Harvest | `ead956eb` | `29aae69b` |
| GoFurry | `03cd366` | The develop commit containing this report |
| Admin | `46a2d77` | `a82f1425` |

The legacy field audit used Collector V2 `dev` at `27d4c1db`. Harvest's `docs/steam-r2-a-details-v3.md` contains the field-by-field mapping and budgets. Admin's `docs/steam-r2-a-details-editor.md` defines the editor and gateway. Testing used native Windows Go, Node, pnpm and Edge with PostgreSQL 18 and private YAML. Isolated CN and INTL databases were separate. Archived AppDetails bodies and local logs stayed in the ignored workspace.

## Contract and ownership

- The new Harvest Offer is `steam.details:1:3:steam_v3`. It delivers compatible details and price, Media v2, and a separate rich details Segment. An explicit version and approved Namespace are required. v1, v2, Assets v1, News, Players and existing plan versions remain available. Current regional quotes accept v3 under the existing source, market, freshness and binding checks. Existing historical price sampling retains its v1 policy.
- GoFurry accepts the v3 scope only with a matching verified AppID, exact plan version, source binding, Store locale, market and requested/actual egress. The durable source projection stores the Observation, Target, Namespace, scope, hash, ruleset and confirmation. Consumer cursor activation and source projection use the same transaction. Replay and 410 recovery re-read through Harvest HTTP.
- Management lists up to 48 candidate summaries and lazily fetches one candidate's source text, reviewed HTML and plain-text alternatives. The shared GoFurry sanitizer parses Steam HTML as HTML and uses the SDK for BBCode. It preserves headings, lists, emphasis, line breaks and safe HTTPS links. Embedded script, media, foreign elements and event attributes are removed. Inline images require official Steam CDN URLs. Links carry `rel` and `referrerpolicy` protections. Editorial replacements are plain text escaped by GoFurry.
- The independent details draft is keyed by Game and editorial locale. Adoption checks the candidate's Observation under the game lock, advances the existing revision and writes an operation receipt. Preview Token includes rich details. Publish and Publish Changes freeze the reviewed details in `game_published_localizations.rich_details`. No collection update can rewrite a draft or publication.
- Public Profile exposes optional `published_details` from the existing SFW published snapshots. It omits the internal Origin and diagnostics. It prefers the requested published locale and marks alternate published locale fallback. Unpublished games and drafts stay private. The existing gallery, News, player and price surfaces remain in use.

## Real sources and field coverage

| AppID and Steam scope | Original bytes | Serialized source bodies, detailed/about/short | GoFurry JSONB logical/column bytes |
| --- | ---: | ---: | ---: |
| Paws 434000, en/HK | 16,934 | 1,803 / 1,803 / 212 | 7,269 / 2,611 |
| Stray 1332010, en/CN | 15,663 | 995 / 995 / 145 | 7,081 / 2,635 |
| Hades 1145360, en/US | 21,692 | 3,551 / 3,551 / 155 | 11,645 / 5,265 |

Paws and Stray came from previously attributed Steam archives verified byte-for-byte. Hades was also collected directly from Steam by a native v3 Job. That Job issued exactly one Permit and saved four successful Observations and a 21,692-byte raw archive. The GoFurry Consumer read that actual Observation from Harvest HTTP. An Admin Publish Changes operation adopted it, and the stored publication references the same Observation and attributed live Attempt. No extra request was made by the Worker.

All three sources had a detailed description, About the Game, short description, website, support email, Windows/macOS minimum requirements, ratings, a required age of zero and full controller support. Paws and Hades provided support URLs. Paws and Stray provided Linux minimum requirements. Stray supplied minimum and recommended requirements on all three platforms. Hades supplied no Linux requirement and marked one language for full audio. The language counts were 9, 16 and 11. The samples supplied no nonempty content descriptor IDs/notes. Synthetic contract cases cover descriptor values, missing/empty data, unknown languages and support flags, malformed optional fields and truncation. Interface and subtitle states remain unknown from AppDetails. A direct Stray zh-CN/CN attempt timed out after one Permit, with all Segments marked unavailable. Stray en/CN and the published-locale fallback were verified independently.

Harvest bounds the rich Segment at 384 KiB with separate per-field budgets. A synthetic 64 KiB introduction survives the encode/normalize path. Oversize fields are omitted with a diagnostic, preserving the other valid facts. The common 32 KiB Media v2 budget remains unchanged. Source normalization in GoFurry stores bounded JSONB. The quoted JSONB sizes are PostgreSQL measurements, including per-row compression.

## Native end-to-end tests

| Scenario | Result |
| --- | --- |
| Harvest Worker, Permit, Raw Evidence and v1-only Worker fence | PASS for real Steam response replay on three apps and a direct Hades Job |
| Native HTTP Consumer and regional persistence | PASS across CN, HK and US source markets with original Observation IDs |
| Replay and epoch/cursor reset | PASS after a genuine HTTP 410, with source rows and published documents conserved |
| Admin editing and promotion | PASS for Stray Publish Changes, Hades initial Publish and Hades live-source Publish Changes |
| Revision, CSRF and receipt | PASS for stale-revision 409, missing CSRF 403 and Developer write rejection. Three real write receipts matched three successful Admin operations. |
| Publication boundary | PASS for draft isolation, Preview Token changes, private Origin stripping and stable readback after bindings were closed |
| Regional API and SSR | PASS for distinct databases, cross-region 404, published/SFW checks, zh/en and published-language fallback |
| R1 public media | PASS for real Steam Paws and Stray HLS in both native Edge HLS and HLS.js/MSE. Manifests and media segments succeeded, playback time advanced and resources were released. No first-page stream requests. |

Public JSON and SSR sizes are from native local HTTP, with the same existing R1 media fields where applicable.

| Scenario | Management Game, bytes | Selected Candidate, bytes | Public Profile, bytes | SSR HTML, bytes |
| --- | ---: | ---: | ---: | ---: |
| Stray Publish Changes | 144,279 | 18,649 | 13,123 | 31,700 |
| Hades initial Publish | 8,022 | 27,886 | 10,426 | 24,971 |
| Hades direct-source Publish Changes | 29,157 | 27,891 | 8,156 | 23,228 |

The browser loads details candidate data on selection. On Edge, time from opening playback to progress was 5,988/3,041 ms for Paws native HLS/MSE and 8,119/3,049 ms for Stray. The corresponding stream request counts were 19/6 and 18/7. The values are local test observations, with no production percentile interpretation.

## Native checks and migration

- Harvest `task check` passed. The explicit native Raw Evidence regression and the v3 Worker/API/PostgreSQL test passed on separately migrated databases.
- An isolated GoFurry `task check` run passed (88 passed, 3 skipped). A second full run on the final source had one intermittent mobile Start animation timing failure (87 passed, 3 skipped); the unchanged test then passed three consecutive serial runs. The original working directory still has a pre-existing ignored diagnostic Go file that triggers the toolchain JSON rule. That file was preserved. SQLC diff, native Go, production Web build, TypeScript, HMR and all 10 focused gallery/details Playwright tests passed. The focused project included no-JavaScript published rich details, safe links and visible unknown language support.
- Admin `task check:api`, `task check:web`, `task check:embed` and focused Playwright passed. Four unrelated or opt-in legacy scenarios in the focused Admin suite were skipped. The real operator workflow was covered separately by the native browser run above.
- Development `harvest_dev` advanced 13 → 14. The restore verification preserved 404 Attempts, 1,288 Observations, 159 Schedules and four existing evidence/attribution rows.
- Development `gofurry_cn_dev` advanced `202611230002` → `202611240001` → `202611240002`. Explicit migration followed isolated CN/INTL execution and local restore verification for each backup. Row counts and ordered fingerprints confirmed that 31 Games, 62 published localizations, 414 current collection rows and 158 plans retained their content. The new v3 quote path returned two attributed current quotes in the isolated test. Admin required no migration.

## Rollout and remaining boundaries

Apply Harvest and GoFurry migrations explicitly, upgrade the Harvest server before v3 Workers, and upgrade GoFurry and Admin before enabling a pilot Namespace. Production services and schedules were untouched. Production egress and localized Steam latency remain rollout checks. The direct Chinese Steam response timed out in this run. The archive replay, true English/US direct chain, independent source markets, and Chinese publication fallback are verified.

R2-A's collection, storage, editing, approval and public presentation path passed. R3-A can use the same source, draft and approval boundaries for Store Events as a separate task.
