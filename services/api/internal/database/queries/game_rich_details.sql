-- name: ListGameRichDetails :many
SELECT m.id,m.observation_id,m.locale,m.store_country,m.egress_region,m.observed_at,m.confirmed_at
FROM game_sources s JOIN game_source_media m ON m.game_source_id=s.id
JOIN collection_bindings b ON b.id=m.binding_id AND b.game_source_id=s.id AND b.game_id=s.game_id
WHERE s.game_id=$1 AND s.active AND b.effective_until IS NULL AND m.state='present'
 AND m.output_schema=3 AND m.ruleset='gofurry-steam-rich-3'
ORDER BY m.confirmed_at DESC,m.id DESC LIMIT 48;

-- name: GetGameRichDetails :one
SELECT m.* FROM game_sources s JOIN game_source_media m ON m.game_source_id=s.id
JOIN collection_bindings b ON b.id=m.binding_id AND b.game_source_id=s.id AND b.game_id=s.game_id
WHERE s.game_id=sqlc.arg(game_id) AND s.active AND b.effective_until IS NULL AND m.state='present'
 AND m.id=sqlc.arg(candidate_id) AND m.output_schema=3 AND m.ruleset='gofurry-steam-rich-3';

-- name: CompareGameRichEnglish :one
SELECT en.payload FROM game_source_media zh JOIN game_source_media en
 ON en.game_source_id=zh.game_source_id AND en.collection_source_id=zh.collection_source_id
 AND en.binding_id=zh.binding_id AND en.locale='en' AND en.store_country=zh.store_country
 AND en.egress_region=zh.egress_region AND en.requested_egress_region=zh.requested_egress_region
 AND en.capability=zh.capability AND en.output_schema=zh.output_schema AND en.ruleset=zh.ruleset
WHERE zh.id=$1 AND en.state='present'
ORDER BY en.confirmed_at DESC,en.id DESC LIMIT 1;

-- name: SaveGameDraftRichDetails :exec
INSERT INTO game_draft_rich_details (game_id,locale,details) VALUES ($1,$2,$3)
ON CONFLICT (game_id,locale) DO UPDATE SET details=excluded.details;

-- name: ClearGameDraftRichDetails :exec
DELETE FROM game_draft_rich_details WHERE game_id=$1 AND locale=$2;

-- name: DeleteOrphanGameDraftRichDetails :exec
DELETE FROM game_draft_rich_details d WHERE d.game_id=$1 AND NOT EXISTS (
 SELECT 1 FROM game_localizations l WHERE l.game_id=d.game_id AND l.locale=d.locale);
