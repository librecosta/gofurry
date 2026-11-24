# STEAM-R3-A Store Events acceptance

**Result: PASS in isolated native acceptance.** Production promotion remains a separate operation.

## Baseline and delivery

All three `develop` baselines matched their remote heads before implementation.

| Repository | Baseline | R3 delivery |
| --- | --- | --- |
| Harvest | `29aae69` | `526a7b3` |
| GoFurry | `febdad0` | `dff206b` |
| Admin | `a82f142` | `bda31ab` |

The legacy comparison used Game Collector V2 at `27d4c1db`. The new Store Events adapter preserves separate event and announcement GIDs, topic and clan identities when provided, event type and name, official URL, body, source language, publication/visibility, post/update/start/end/created/modified times, tags, comments and votes. Harvest preserves the original response under Raw Evidence. GoFurry maintains the raw article body and its reviewed HTML, plain text and summary. Controllers use the same single Storefront request budget as AppDetails.

## Real Steam proof

Windows native source probes returned HTTP 200 for English and Chinese requests for CS2 (730) and Terraria (105600). CS2 supplied both languages. The Chinese request returned two Steam language-6 articles containing Chinese text and two language-0 English fallbacks. Terraria returned English for both requests, with bodies up to 42,491 bytes. Terraria was used for source-shape and size evidence. The native full chain used CS2.

| Native CS2 Worker case | Raw bytes | Persisted Observation bytes | Articles | Permits on successful Attempt |
| --- | ---: | ---: | ---: | ---: |
| English `0` | 21,331 | 9,471 | 4 English | 1 |
| Simplified Chinese `6_0` | 21,316 | 9,447 | 2 Chinese + 2 English | 1 |

Both archives matched SHA-256 in PostgreSQL and linked to their successful Attempts. The first bounded live batch also observed three upstream timeouts and quota deferrals. A separate one-Permit English retry succeeded. Replay and browser checks used persisted responses. The Empty/Unavailable distinction, foreign AppID isolation and over-budget article handling also have synthetic tests.

The Chinese-scope Observation entered GoFurry through authenticated Harvest HTTP. Four linked articles and four body versions were persisted in CN. Announcements retain their Steam-declared language and the original requested locale. Admin exposes a separate script signal based on reviewed plain text. Han characters supply a script signal only. English fallback stays labeled English. A real HTTP 410 and Snapshot Reset restored the cursor with the same article and version counts.

## Persistence, moderation and public reading

- The Consumer validates the requested locale, actual egress, capability/schema, AppID, independent GIDs, scope and approved source binding before saving. Snapshot activation and incremental projections save articles and the cursor together. Batch reads load up to 16 eligible source rows at a time. A failed, empty or missing source leaves the existing articles in place.
- PostgreSQL keeps a current row per GameSource, actual exit, event and Steam-declared language. A body/headline digest identifies a version, with up to ten distinct retained versions. The rollback-only native test verified that increasing votes creates zero body versions, missing votes preserve the known value, and editing the body creates one version with independent provenance.
- The existing GoFurry rich HTML allowlist reviews the SDK-converted BBCode/HTML. Tests exercised headings, lists, block quotes, official lazy Steam images, HTTPS links, scripts, event attributes, external images and iframes. Video markup becomes safe outbound links where the SDK can resolve it.
- In native Windows Edge, the embedded Admin read and rendered actual Chinese CS2 content. An unauthenticated read returned 401, a Developer read succeeded and its write returned 403, and a write missing CSRF returned 403. An Operator hid the article, which returned 404 publicly, then restored it, which returned 200. The stale revision returned 409. The two successful operations each generated both a GoFurry receipt and an Admin audit result. Hide and restore apply to the selected language variant.
- Profile and insight lists continue to use the existing paged news summary contract. Verified Store Events add an article ID, summary and actual declared locale. Matching official brief identities are deduplicated. Full content is available from `GET /api/v1/games/:publicID/news/:article` after the existing public/SFW checks. The website requests one full article when the reader follows the article link, retaining the Game Profile canonical URL and SSR. Edge rendered the article with JavaScript disabled. Public JSON contains no raw body or internal provenance.
- The independent INTL API returned 404 for the CN-only article. The CN/INTL native game-profile suite passed all three cases, including the legacy six-plus-two News keyset, SSR, price semantics and Players. A separate real CDN regression passed Paws and Stray in native HLS and MSE, including teardown and previous SFW/region fences.

## Native checks and measurements

- Harvest `task check` passed. PostgreSQL 18 native evidence, versioned Offers, Assets and v3 Rich Details tests passed on independent databases. The Store Events package tests verified one SDK GET, exact evidence, distinct GIDs, optional zero counters, language fallback and partial-batch handling.
- GoFurry passed API tests, SQLC diff, Web build/typecheck and the native versioning test. A clean isolated source copy passed `task check` with 88 browser tests passed and three opt-in tests skipped. The ordinary worktree's existing ignored diagnostic Go file triggers the toolchain check and was kept. Later scoped changes were exercised by the native Consumer/browser replay and the SQLC/Go checks.
- Admin passed native API, typecheck/build and Embed checks. Default Playwright reported 33 passed and 24 opt-in cases skipped. The real browser/Management operations ran separately on the isolated databases.

| Measurement | Local result |
| --- | ---: |
| CS2 Public Profile with summaries | 2,471 bytes |
| Public English / Chinese article samples | 1,618 / 2,453 bytes |
| Admin article detail sample | 5,671 bytes |
| Selected article SSR document | 18,324 bytes |
| CN four raw bodies / reviewed HTML | 6,305 / 6,741 bytes logical |
| CN PostgreSQL raw plus HTML column storage | 10,794 bytes |
| Local Profile GET samples | 15–125 ms |
| Real HLS click-to-play samples | 2.9–9.0 s, 6–19 stream requests |

These are individual isolated measurements. They include warm/cold runtime and CDN variation.

## Migration and operating limits

Harvest v15 and GoFurry `202611240003` were explicitly migrated on isolated PostgreSQL 18. The GoFurry migration passed both a cloned CN/INTL upgrade and clean CN/INTL creation. Admin has no R3 migration. Shared Harvest reported v14 by a read-only check. The shared GoFurry migration level was not independently verified in this run. Shared databases and production were untouched.

For promotion, verify target and backups, apply both migrations explicitly, and upgrade Harvest server before Workers. Upgrade the regional GoFurry Consumer/API and compatible Admin before enabling an authorized Preview Namespace. Verify permissions, quotas and real egress separately per deployment. Steam Events are a bounded recent window. The article source language is a Steam declaration, and the body script signal is not a language classifier. Terraria's Chinese request fell back to English. No prior-site history, production schedule, or deployment was started with this change.

R3 acceptance is complete. The next independent implementation is STEAM-R4-A.
