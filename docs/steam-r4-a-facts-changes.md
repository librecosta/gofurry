# STEAM-R4-A canonical facts and change intelligence

## Legacy contract and sources

The legacy audit used Game Collector V2 at `27d4c1db`. The US/English canonical source, typed release windows and language normalization come from decision 0005. Decisions 0007 and 0008 distinguish observed history from missing data. The legacy Game Change Registry provides the six version-1 detector keys and their event codes.

| Legacy capability | GoFurry R4 source and contract |
| --- | --- |
| Canonical release state and windows | Harvest `steam.details:1:3:steam_v3` rich details with `steam-go` v1.3.12 `NormalizeReleaseDate` |
| First available and release history | Evidence-backed review, independent minimum available observation and normalized UTC-day changes |
| Canonical languages | Steam SDK language definitions, original supported-languages text, source order and optional support columns |
| Daily free and platform states | Explicit presence-aware AppDetails v3 fields, one effective fact per field and UTC day |
| Daily prices and discounts | Approved Details v1/v3 Price Segment written into the existing price table, with bounded daily reads |
| Player peak, average and coverage | Existing half-hour samples, daily API and tracking-specific 30-day quality |
| Facts, Metrics and Changes | Existing Consumer and collection bindings, regional PostgreSQL and six compiled versioned detectors |

Harvest retains the same offers, transport, quota, Raw Evidence, request count and Schedule behavior. No Harvest code or schema change was needed. The existing v3 price shape already carries its actual currency. R4 accepts that currency from verified v3 source rows. Legacy v1 price validation still requires the configured market currency.

## Fact semantics and scope

The tracking identity is the effective-dated Steam row in `collection_bindings`. Observation AppID, source, Target, namespace, capability and version, language, country and exit are checked against the approved plan and binding. A corrected AppID uses its own period. Historical records keep the prior period. Archived games and inactive bindings leave the current read path.

Each available daily fact retains the observation ID, actual observation UTC day and time, confirmed time, market, locale, exit and normalizer version. Latest known values are independent across fields. Canonical automatic release, languages, free and platform facts use US/en. Other validated scopes remain separately recorded. With no US/en evidence the new field stays unknown and existing market release text remains available through the legacy Profile. A candidate with an omitted field keeps the last valid value. Canonical fields older than seven days are marked stale. An authoritative scope withdrawal can mark that value unavailable. A timed-out or failed job produces no new fact. The existing collection diagnostics remain the source for individual job failures.

Release has three independent facts. **Official Release** preserves raw text, known availability, day/month/quarter/year/TBA/none/unknown precision and available calendar endpoints. **First Observed Available** is the earliest retained observation with explicit available status. **First Available** requires a qualifying same-AppID Steam announcement reviewed by an authorized operator. It stays unknown without historical evidence, even when the official date is exact. Corrections append a new review with the previous revision, interval, evidence, reason and actor.

Languages retain Steam identity, API/Web codes, tier, vendor order and unknown names present alongside identified languages. Interface and subtitles can remain unknown. Empty, unrecognized and oversized sets leave the previous valid set in place. The fact limit is 64 KiB of encoded language JSON. Rich descriptions and the R2 publication path are independent.

Prices record actual valid observations and their day, currency, amounts, discount, confirmation, locale and actual v3 exit. Daily reads select the final valid observation of the day and its sample count. Missing days have no row. The latest quote carries its observation time and becomes stale after 48 hours. Free and zero price are distinct. Player samples continue to use the existing half-hour storage and UTC range queries. The new quality summary counts valid current-period samples, observed and missing days, and last observation, scoped by exit.

## Projection and recovery

Incremental Consumer writes and cursor changes commit together. Reset copies minimized rich facts and price inputs during staging. Activation writes durable facts and the new cursor in one transaction and clears its staging rows. An interrupted reset leaves its candidates private. Bounded maintenance removes staging rows older than two days.

The queue uses binding and UTC day. A changed daily value can enqueue the affected day and its successor. Projection operates on closed UTC days. Each pass has a 12-second budget, at most 32 binding-days, and bounded pruning. The selected day is locked. Event replacement, six applicable version checkpoints and dequeue commit atomically. Failures leave the day pending and apply a five-minute retry delay. The status includes the oldest pending day when computing lag. Upstream Through represents the latest closed UTC day against the available committed Consumer inputs. Late valid inputs can reopen a previously processed day. An upstream gap carries no successful observation.

| Detector v1 | Codes |
| --- | --- |
| `free_game_transition` | `game_became_free`, `game_became_paid` |
| `windows_support_transition` | `windows_support_added`, `windows_support_removed` |
| `mac_support_transition` | `mac_support_added`, `mac_support_removed` |
| `linux_support_transition` | `linux_support_added`, `linux_support_removed` |
| `game_release_transition` | `game_became_available`, `game_availability_withdrawn`, `game_release_plan_changed` |
| `game_price_transition` | State, currency, increase/decrease and discount started/changed/ended |

Adjacent valid days must belong to the same binding, market, locale and exit. The first observation is a baseline. A missing day breaks comparison. Changed release text alone has no event if the known date semantics match. A currency switch suppresses price and discount arithmetic. Currency is compared only for quotes with amounts. Each event contains Before, After, the day of detection, source observation time, and independent materialization time. UUIDv7 IDs are retained when the natural event key survives recomputation.

A bounded rebuild covers one tracking period and at most seven retained closed UTC days per request. Dry run computes the same rules without writing facts, events or checkpoints. Both use the existing Management receipt. The ordinary history window is 400 UTC days including the prior input day. Pruning preserves the latest fact per field and scope. First observed availability and review evidence survive daily retention.

## HTTP and rollout

Public Profile adds optional `steam_facts` and `GET /api/v1/games/:publicID/changes` provides a bounded keyset page. Both use the regional Published/SFW boundary. Public history contains semantic values, market, day and quality. Private Management exposes provenance and detector diagnostics. Draft and Published Snapshot remain governed by the existing editorial APIs.

GoFurry explicitly applies migration `202611240004` after R3 `202611240003`. Admin requires no schema change. The native API starts against the applied schema and validates the registry when Harvest Consumer is configured. Upgrade isolated/production regions separately. If application rollback is necessary, stop R4 writers and roll back compatible binaries while retaining the additive regional database structures. Any database restore requires the region's approved backup procedure.
