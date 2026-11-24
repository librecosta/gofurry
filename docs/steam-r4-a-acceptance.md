# STEAM-R4-A acceptance

## Native environment

The implementation started from remote-matched `develop` heads Harvest `526a7b3`, GoFurry `f68db9f` and Admin `bda31ab`. Harvest remains at `526a7b3`. Admin delivered `7a0a096`. The containing GoFurry commit delivers the Consumer, database, API and Public Web changes.

Tests used native Windows Go 1.27.4, Node 24.22.1, pnpm 12.8.1, Edge and loopback PostgreSQL 18. The GoFurry CN and INTL test databases were independent disposable clones. The final migration upgraded each from `202611240003` to `202611240004`. The isolated Harvest R3 database was at v15. Read-only probes reported shared `harvest_dev` at v14 and `gofurry_cn_dev` at `202611240002`. A shared INTL migration status was not verified. Those development databases received no migration. No production resources were used.

## Harvest observations and replay

| App | Input | Scope replayed | Result |
| --- | --- | --- | --- |
| Hades 1145360 | Previously captured real Steam v3 Observation | US / English, actual CN egress | Available, exact official 2020-09-17, 11 language entries, First Available unknown, USD quote |
| Paws 434000 | Native v3 projection fixture backed by archived Steam payload | HK / English, actual CN egress | Six scoped daily facts and HKD quote |
| Stray 1332010 | Native v3 projection fixture backed by archived Steam payload | CN / English, actual CN egress | Six scoped daily facts and CNY quote |

The HK and CN fixture markets refer to isolated projections and are distinct from claims about fresh upstream market capture. The real Hades capture retained its original source market. R4 restarted an isolated native Harvest Server and replayed its durable Observations over authenticated HTTP. It issued zero new Steam collection requests. A real HTTP 410 and subsequent Snapshot Reset restored the Consumer cursor on both replay streams. The first run retained six facts from one Observation, and the combined run retained 24 facts from four Observations. Four version-3 quotes persisted with the correct actual exit, including two US and one each CN and HK. The single-day sources created zero price or release change events.

## Semantic and operating checks

- Go tests exercise day, month, quarter, year, TBA, none, unknown, date-format equivalence, absent support flags, language/audio interpretation, and currency-only changes.
- The rollback-only PostgreSQL test writes an approved tracking-period fixture. It verifies UTC daily comparisons, seven changes from a valid two-day sequence, stable event UUIDs on repeat projection, a currency-only price transition, and absence of cross-period, cross-market and cross-egress events. A dry run leaves events untouched. Independent player quality includes an observed zero and excludes another tracking period and exit. First Available correction preserves prior evidence and rejects a stale review revision. Draft change rows have no public visibility.
- A native reconcile probe forced an event INSERT failure in a disposable database. The transaction left zero events and retained the queued day for retry. After the test cleared the injected fault and advanced the retry deadline, the running service wrote one event, advanced all six checkpoints and dequeued the day on its next tick. Completed reset staging was empty.
- Edge used the native Admin Embed and private gateway for Operator and Developer sessions. Public requests without a session received 401. Developer reads succeeded and writes were forbidden. A write missing CSRF received 403. The Operator read facts and the six-detector status and used the real preview action. GoFurry returned a matching Request ID and receipt. The durable rebuild was exercised in PostgreSQL transactions.
- CN Profile and JS-off SSR showed the new Hades facts alongside the existing R2 published details. The independent INTL API returned 404 for the CN-only game. The CS2 R3 list still contained Chinese and English articles and the full article remained readable. An independent CN/INTL legacy Playwright run passed all three game-profile cases, including old price/player semantics, a gap-aware chart, legacy news pagination and bilingual SSR.
- Paws and Stray were played from their adopted R1 snapshots in native Edge HLS and HLS.js/MSE. Both obtained real manifests and segments and advanced playback time. Closing stopped both paths. The MSE runs revoked their owned object URLs. The prior fallback, SFW, independent region and CDN-failure assertions also passed.

## Local measurements

| Check | Observation |
| --- | --- |
| Hades extended Profile serialized sample | 9,241 bytes in the first replay |
| Hades CN initial SSR, JS off | 28,480 bytes |
| Admin Game Facts read | 7,466 bytes and 17.5 ms in one final local request |
| Paws and Stray initial Web JS | 462,734 bytes before opening media |
| Gallery and HLS.js JS assets | 17,119 and 331,491 bytes |
| Real HLS, Paws native / MSE | 19 / 7 stream requests, 7.47 / 3.12 seconds to playback proof |
| Real HLS, Stray native / MSE | 13 / 7 stream requests, 7.52 / 5.98 seconds to playback proof |
| Synthetic 10-game / 100-game / 1,000-game rollback probe | 85 / 122 / 601 ms for connection, inserts, queue creation, summary and rollback |
| Synthetic daily fact composite bytes | 9,917 / 99,197 / 991,997 bytes for 80 / 800 / 8,000 rows |
| Registry / closed-day checkpoints on first-day replay | 6 / 0 before the observed UTC day closed |

The synthetic measure covers storage and queue intake. It excludes full background catch-up time and production load. Single-run HTTP and CDN timings are local observations.

## Test suites and limits

Harvest `task check` passed. Admin `task check:api`, `task check:web` and `task check:embed` passed. Admin Playwright passed 33 tests with 24 opt-in cases skipped. The distinct Edge flow above used the real local services.

GoFurry SQLC diff, native Go, Web typecheck and native local integrations were exercised. The original worktree's `task check` stops on the existing ignored `services/api/.local/ops-data-60/diagnose.go` because it imports the old JSON package. The file was preserved. The clean isolated source copy passed `task check` with 88 browser tests passed and three opt-in tests skipped. The earlier isolated run hit an unrelated Start suggestions timing assertion. The final rebuild lock ordering was also retested in the affected Go packages and against PostgreSQL.

An early harness import restarted the old R2 isolated fixture and refreshed its generated run files before it was redirected to R4-owned clones. The reported acceptance uses the dedicated R4 copies. No old-site history was imported and no batch Steam requests or production schedules were created. A single historic US/en Hades sample cannot prove its first playable date. Additional upstream locales, alternate browser/CDN paths and production deployment/sizing remain outside this native acceptance evidence.
