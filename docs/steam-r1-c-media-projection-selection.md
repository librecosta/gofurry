# Steam R1-C media projection and selection

GoFurry owns its regional Steam media candidates and editorial selections.
The Consumer accepts the following bounded Harvest results.

| Capability | Output schema | Regional projection |
| --- | --- | --- |
| `steam.details` | 1 | Existing text, price and URL-array media |
| `steam.details` | 2 | Existing details and current price facts, plus structured AppDetails media |
| `steam.assets` | 1 | Structured StoreBrowse assets |

Other registered capabilities continue to use their schema 1 contracts.
The Consumer distinguishes unsupported schemas, malformed known payloads
and conflicting AppIDs. An invalid known media payload preserves last
known good while the stream advances. An unsupported schema, unknown
capability or conflicting new-version AppID fails the page before ACK.

## Durable projection

The Consumer validates the full source scope and its hash, known
capability and version, the bound Target, approved Plan version, and
effective binding interval. Steam AppID, market and language agree with
the registered collection source and its attached Plan. The regional
GameSource must be active and own the same Steam AppID. No title lookup is
used.

`game_source_media` keeps the latest verified row for each GameSource,
namespace, capability, output schema, scope hash and actual egress.
The Harvest scope carries the requested egress. The Observation also
reports the actual Attempt egress, so a route policy with multiple exits
keeps them separate. The row retains the Target, collection source,
binding, Observation ID, content hash, ruleset, state and time fields.
The binary Observation envelope is not replicated; the row contains
only the validated media contract and provenance. V1 remains in
`game_source_metadata`. Details v2 text follows the existing fact
normalizer. Current price reads can use either approved version.
Steam Prefill and the legacy durable text and price-history paths remain v1.

Increments write source candidates and cursor in one transaction before
ACK. Reset builds a generation from Snapshot and the change catch-up, then
commits source rows and the new cursor together before ACK. Epoch changes
compare source receipt time across epochs. Within an epoch, positions
fence replays. Confirmation of the same content retains the original
Observation and refreshes confirmation time. A reset with no row for a
scope keeps its last durable evidence. A known withdrawal covers the
declared scope and its actual exits. A verified `confirmed_empty`
updates only its corresponding StoreBrowse source. Truncation is
reported as received. Missing entries in a truncated result establish
no withdrawal.

Each high-fidelity payload is validated at its actual JSON size with
a 32 KiB ceiling. PostgreSQL has a 40 KiB text ceiling on these JSONB
columns to account for its object rendering. Existing browser-facing
and Web evidence budgets stay on their established limits. Only the
three Harvest stream reads can use the 4 MiB page budget.

## Editorial and publication

Management returns versioned candidate groups. For the same AppID,
market, language and relevant exit, prefer structured AppDetails media
when available and use the existing v1 candidate as the fallback.
StoreBrowse adds independent image roles. Neither URL equality nor
arrival order establishes identity across versions or roles.

The authenticated Management selection endpoint resolves the requested
IDs from durable verified candidate rows. It copies complete records
with their individual source receipts into `game_draft_media_v2`.
Selection is revision-controlled and audited. Existing valid picks can
be reduced or reordered after their candidate leaves the current list.
Source updates never write either draft or published selections.

Legacy changes still replace only legacy `game_draft_media`. The text
locale must exist to add a v2 selection. If a legacy edit later removes
that locale, the stored new selection remains readable in Management
and is excluded from that publication preview until the locale returns.

The existing SFW preview, Publish and Publish Changes flows include
v2 data in the token and atomically copy it into
`game_published_localizations.media_v2`. Published media retains its
selection revision and the localization snapshot has its publication
revision. The public Game Profile continues to return its existing
compatibility fields and reads the selected cover and direct-file
projection from the local snapshot. Catalog directory cards retain
their existing cover contract. The public API makes no Harvest request.

## Plans and migration

`202611230001_steam_media_projection.sql` adds the explicit
`collection_plans.output_schema`, defaulting old rows to 1, as part of
the existing Plan uniqueness key. New Details v2 and Assets v1 Plans
are opt-in. Assets Plan creation and updates enforce a minimum six-hour
interval. Server reconciliation compares the complete request, keeps
old Schedule IDs and allows Harvest to enforce its preview namespace
gate. The migration creates no Schedules and makes no outbound calls.

`202611230002_steam_versioned_quotes.sql` matches storefront price
quotes to the approved Plan's output version and selects one freshest
quote per market and locale. The legacy historical price writer
attributes only to its approved v1 Plans.

[Management and media contract](steam-r1-c-media-contract.md) ·
[Verification and rollout](steam-r1-c-acceptance.md)
