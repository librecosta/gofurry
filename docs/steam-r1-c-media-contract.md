# Steam R1-C Management media contract

These fields extend the private GoFurry Management Game routes under
`/internal/v1/ops`. Requests use the existing Management gateway token,
region and authenticated Actor assertions. Owner and Operator roles
can save selections through the normal revision and audit rules.
The legacy JSON shapes remain supported.

## Read

`GET /games/{public_id}` adds these fields.

| Field | Meaning |
| --- | --- |
| `media_candidates_v2` | First 12 durable, currently bound candidate groups |
| `media_candidates_v2_next` | Optional last row UUIDv7, for the next candidate page |
| `draft_media_v2` | Independent editorial selection snapshots by locale |
| `published_media_v2` | Adopted snapshots by locale with publication revision |

Use `GET /games/{public_id}/media-candidates?after={cursor}` for
additional bounded pages. The response has `items` and optional
`next`. An unbound source is excluded from candidate queries.

Candidate groups describe the GameSource, collection source, binding,
Target, namespace, capability and output schema. They also include
locale, Store Country, actual and requested egress, canonical scope
key, Observation ID, content hash, ruleset, observed time and confirmed
time. Source locale uses `en` or `zh-cn`. Market codes are lowercase.
Each group carries one typed payload.

- `media` includes `app_id`, `truncated`, five optional AppDetails
  image slots, the ordered Movie array and the ordered Screenshot array.
  Movie IDs are lossless decimal strings. Each Movie retains Name,
  Thumbnail, Highlight if available, source Order and the independent
  `mp4_480`, `mp4_max`, `webm_480`, `webm_max`, `dash_av1`,
  `dash_h264` and `hls_h264` fields. Screenshot IDs are decimal
  strings and can be zero. Full and thumbnail URLs stay separate.
- `assets` includes `app_id`, `truncated` and the ordered
  StoreBrowse array. A row retains Asset ID, Type, Family, native Slot,
  canonical URL, locale, source surface, observed time and optional
  source path digest. The supported roles are the 17 Harvest
  StoreBrowse types, including distinct Library and 2x variants.
  Absence of Library Logo is valid. A `confirmed_empty` group has an
  empty array and `truncated=false`.

Legacy `source_candidates` still supplies v1 display candidates.
New and old candidates are grouped by source and market. Raw Steam JSON,
HTTP headers and credentials are outside this contract.

## Save a selection

`PATCH /games/{public_id}/media-selection` accepts
`{ "revision": number, "selections": [ … ] }`. Each item has the
draft `locale` (`en` or `zh`), optional `cover`, and optional
`images`, `screenshots` and `movies` arrays. Each pick uses this
same typed reference.

```json
{
  "game_source_id": "01900000-0000-7000-8000-000000000001",
  "observation_id": "01900000-0000-7000-8000-000000000002",
  "id": "9007199254740993"
}
```

Use Movie ID, Screenshot ID or StoreBrowse Asset ID for the relevant
group. For an AppDetails base image, the stable source-slot selector is
`header_url`, `capsule_url`, `capsule_v5_url`, `background_url`
or `background_raw_url`. The server resolves the role and URL.
Request fields are strict. The ordinary 16 KiB Management write limit
applies. A stale revision returns a conflict. The editor can select
from another approved source language or market for that draft locale.
The original language and market remain in the saved provenance.

A save replaces the v2 document across draft locales. Send
`selections: []` to clear it. It does not edit legacy URL arrays.
Each locale can adopt one cover, one image per official role, eight
screenshots and four movies. Logos and Community Icons are role
images. Order in request arrays controls display order. Original
Movie and Screenshot `order` values are retained. Reusing a stable
identity twice in one group is invalid. Two different image roles may
legitimately share a URL.

The server copies full selected objects into a typed version 2
document with a selection revision and individual `origin` records.
The originally adopted receipt is preserved for retained picks.
Publish and Publish Changes use the standard preview token and copy
the document to the published localization. Public compatibility
returns a direct MP4 or WebM URL only where present. HLS and DASH
remain typed sources in Management and stored snapshots.

## Create an explicit Plan

The existing Game Collection Plan create endpoint accepts
`output_schema`, `locale` and `store_country` for the Steam media
capabilities. Select schema 2 for `steam.details` explicitly.
`steam.assets` uses schema 1. Omission keeps existing schema 1
behavior. The CLI also exposes `--output-schema`. Harvest preview
authorization and the configured egress policy still govern execution.
