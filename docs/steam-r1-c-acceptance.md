# Steam R1-C acceptance

Verified on 2026-11-23 in Asia/Shanghai. GoFurry started from
`develop` at `53e89db`. Harvest contracts were read at R1-A
`57b262e` and R1-B `ead956e`.

## Source samples

The private AppDetails captures were run offline through the existing
Harvest v1/v2 provider. The inputs were the previously verified R0-A
archives. The StoreBrowse payloads came from the persisted R1-B
Worker results. The GoFurry test delivered the normalized payloads
through a loopback Harvest protocol fixture and used versioned local
Plans and effective source bindings. This verification issued zero
Steam requests.

| Game | Details v2 market | Movies | Screenshots | StoreBrowse en/US assets |
| --- | --- | ---: | ---: | ---: |
| Paws, AppID 434000 | en/HK | 2 | 12 | 9 |
| Stray, AppID 1332010 | en/CN | 3 | 8 | 15 |

The test compares the full persisted structures against the input.
It checks decimal IDs, ordered arrays, full and thumbnail screenshot
URLs, base images, all supplied playback formats and the asset role,
slot and identity fields. The real Movie samples have HLS and DASH
sources. The public direct-file projection leaves their Movie URL
array empty. Both StoreBrowse samples have distinct Library Capsule
and Hero roles, including applicable 2x rows. Both omit Library Logo.

## Native Consumer and editorial tests

- Windows Go 1.27.4 and PostgreSQL 18.6 ran the versioned Offer,
  bounded wire and native media tests against the isolated
  `gofurry_r1c_media_test` database.
- One AppID exercised English and Simplified Chinese, US/CN/HK market
  scopes, and independent actual exits. Assets confirmed empty affected
  the matching source. Truncated media, an invalid known payload and
  an empty reset preserved unrelated evidence. A genuine scope
  withdrawal retired its source exits while retaining the publication.
- Snapshot and incremental pages committed with the cursor before
  ACK. Unknown versions, mismatched AppIDs and an injected
  PostgreSQL write failure left the cursor in place. A same-result
  confirmation retained its Observation. HTTP 410 reset changed epochs
  and upstream 503 left the editorial and public snapshots readable.
- Native selection tests exercised one cover, 8/4 limits, preserved
  original order, custom editorial ordering, equal URLs with distinct
  roles, cross-game rejection, legacy edits, revision handling,
  stale Preview Token, Publish and Publish Changes. The existing
  `TestCatalogPostgreSQL` passed on isolated CN and INTL databases.
- The native GoFurry executable listened on two loopback ports with
  an explicit private YAML and consumed one fixture event through its
  real Consumer lifecycle. Readiness, Management listing and public
  profile returned HTTP 200. An observer confirmed the cursor was
  committed before ACK. Management returned 401 without auth, 403
  for developer writes, 409 for a stale revision and 400 for a
  caller-supplied URL. An authorized retained selection returned 200
  and produced one audit receipt. The executable shut down cleanly.
- The built Web rendered the canonical published Paws URL against the
  loopback API with Harvest disabled. The SSR response retained the
  original Public ID and Slug, editorial text and selected screenshot.

| Local operation | Measurement |
| --- | ---: |
| Two games, Plan setup and ten-item snapshot activation | 172 ms |
| Five-event page including local commit and ACK | 123 ms |
| Mixed-origin selection draft write | 5.0 ms |
| Compact published media snapshot for one locale | 6,003 bytes |
| 100 scope rows, 13-row keyset SQL page median | 0.55 ms |
| 1,000 scope rows, 13-row keyset SQL page median | 1.00 ms |
| 10,000 scope rows, 13-row keyset SQL page median | 0.53 ms |

Each page measurement used 25 reads after a cursor inside a rolled-back
fixture transaction. These Windows loopback timings cover the database
query and decode. They do not establish production latency.

## Repository checks

`task sql:check` and the native API vet, tests and build passed. The
repository checks ran on a clean staged copy because the local checkout
contains an ignored diagnostic Go file under `services/api/.local`.
The Web build and typecheck passed. The first full Web test run had
87 passes, three opt-in weather skips and one responsive quick-access
assertion with a 0.015625 px difference. That test passed alone and
the full Playwright rerun with five workers finished with 88 passes
and the same three skips. The explicit local SSR check above exercised
the real Game profile path separately.

## Shared development migration

The configured database was `gofurry_cn_dev` on PostgreSQL 18.6.
Both Goose steps used the existing development YAML with writers
quiescent. Each pre-migration custom-format backup was restored to a
separate loopback database and checked by version and row fingerprints
before applying the migration to the configured target.

`202611230001` installs versioned Plans and source/draft/public media
storage. `202611230002` makes existing current market quote selection
version-aware. Legacy columns in 14 tables retained their row counts
and hashes, including 31 games, 62 legacy draft media rows, 62
published localization snapshots, 124 source metadata rows, 158
Plans and 414 collection current rows. All existing Plans still carry
schema 1. The shared database remains on its existing schedules and
contains zero R1-C draft or published selections.

## Reproduce and handoff

```powershell
task sql:check
task check
go -C services/api test ./internal/collection -run '^TestSteam(OfferVersions|MediaWireValidation|MediaNative)$' -count=1 -v -args -steam-media-db-config <isolated-yaml> -steam-media-samples <private-samples>
```

The native fixture uses an empty, explicitly migrated loopback
database named `gofurry_r1c_media_test`. Input names follow
`<app_id>-media-v1.json`, `<app_id>-media-v2.json`,
`<app_id>-details-v2.json`, `<app_id>-price-v2.json` and
`<app_id>-assets-v1.json`. StoreBrowse samples wrap the normalized
payload in `data` with its original `observed_at`.
Private samples, backup archives and verification logs are ignored.

R1-D can use the private Management fields and selector endpoints
with the existing revision and publication flows. StoreBrowse
omissions and truncation are valid candidate states. Playback
negotiation, visual ranking and source health belong to the next
editorial phase. Steam Prefill retains its v1 policy.

[Projection design](steam-r1-c-media-projection-selection.md) ·
[Management contract](steam-r1-c-media-contract.md)
