-- name: CreateGame :one
INSERT INTO games (id,public_id,slug,intake_managed) VALUES ($1,$2,$3,$4)
ON CONFLICT (public_id) DO NOTHING RETURNING *;

-- name: LockGame :one
SELECT * FROM games WHERE id=$1 FOR UPDATE;

-- name: UpdateGame :one
UPDATE games SET slug=$2,revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: ChangeGameStatus :one
UPDATE games SET status=sqlc.arg(new_status),revision=revision+1,updated_at=transaction_timestamp(),
 published_at=CASE WHEN sqlc.arg(new_status)::text='published' THEN transaction_timestamp() ELSE published_at END,
 archived_at=CASE WHEN sqlc.arg(new_status)::text='archived' THEN transaction_timestamp() ELSE archived_at END
WHERE id=sqlc.arg(id) RETURNING *;

-- name: ClearGameLocalizations :exec
DELETE FROM game_localizations WHERE game_id=$1;

-- name: AddGameLocalization :exec
INSERT INTO game_localizations (game_id,locale,name,summary) VALUES ($1,$2,$3,$4);

-- name: DisableGameSources :exec
UPDATE game_sources SET active=false WHERE game_id=$1 AND active;

-- name: CanPublishGame :one
SELECT EXISTS(
 SELECT 1 FROM games g CROSS JOIN LATERAL (SELECT game_presentation(g.id,g.intake_display_source) AS profile OFFSET 0) p
 WHERE g.id=sqlc.arg(id) AND jsonb_array_length(p.profile->'localizations')>0
   AND (NOT g.intake_managed OR (p.profile->>'approved')::boolean)
)::boolean;

-- name: SaveGameSource :one
INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (platform_code,external_key) DO UPDATE SET source_url=excluded.source_url,active=true
WHERE game_sources.game_id=excluded.game_id RETURNING id;

-- name: PublishedGame :one
SELECT e.public_id,e.slug,(p.profile->'localizations')::jsonb AS localizations,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('platform_code',x.platform_code,'external_key',x.external_key,'source_url',x.source_url) ORDER BY x.platform_code COLLATE "C",x.external_key COLLATE "C")
 FROM game_sources x WHERE x.game_id=e.id AND x.active),'[]'::jsonb)::jsonb AS sources
FROM games e CROSS JOIN LATERAL (SELECT game_presentation(e.id,e.intake_display_source) AS profile OFFSET 0) p
WHERE e.public_id=$1 AND e.status='published' AND (NOT e.intake_managed OR (p.profile->>'approved')::boolean);

-- name: PublishedGamePage :many
SELECT e.public_id,e.slug,(p.profile->'localizations')::jsonb AS localizations
FROM games e CROSS JOIN LATERAL (SELECT game_presentation(e.id,e.intake_display_source) AS profile OFFSET 0) p
WHERE e.status='published' AND (NOT e.intake_managed OR (p.profile->>'approved')::boolean) AND e.public_id>sqlc.arg(after_public_id)
ORDER BY e.public_id LIMIT sqlc.arg(page_size);
