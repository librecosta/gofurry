-- name: CreateGame :one
INSERT INTO games (id,public_id,slug) VALUES ($1,$2,$3)
ON CONFLICT (public_id) DO NOTHING RETURNING *;

-- name: LockGame :one
SELECT * FROM games WHERE id=$1 FOR UPDATE;

-- name: UpdateGameDraft :one
UPDATE games SET slug=CASE WHEN status='draft' THEN sqlc.arg(edit_slug)::text ELSE slug END,
 draft_slug=CASE WHEN status='published' THEN nullif(sqlc.arg(edit_slug),slug) END,
 revision=revision+1,updated_at=transaction_timestamp()
WHERE id=sqlc.arg(game_id) AND status IN ('draft','published') RETURNING *;

-- name: ChangeGameVisibility :one
UPDATE games SET status=sqlc.arg(new_status),content_rating=sqlc.arg(content_rating),
 slug=CASE WHEN sqlc.arg(new_status)::text='draft' THEN coalesce(draft_slug,slug) ELSE slug END,
 draft_slug=CASE WHEN sqlc.arg(new_status)::text='draft' THEN NULL ELSE draft_slug END,
 revision=revision+1,updated_at=transaction_timestamp(),
 archived_at=CASE WHEN sqlc.arg(new_status)::text='archived' THEN transaction_timestamp() ELSE NULL END
WHERE id=sqlc.arg(id) AND sqlc.arg(new_status)::text IN ('archived','draft') RETURNING *;

-- name: ClearGameLocalizations :exec
DELETE FROM game_localizations WHERE game_id=$1;

-- name: AddGameLocalization :exec
INSERT INTO game_localizations (game_id,locale,name,summary) VALUES ($1,$2,$3,$4);

-- name: SaveGameDraftText :exec
INSERT INTO game_localizations (game_id,locale,name,summary,text_source_id,text_observation_id)
VALUES ($1,$2,$3,$4,$5,$6);

-- name: AttributeNewGameDraftText :exec
UPDATE game_localizations SET text_source_id=$5,text_observation_id=$6
WHERE game_id=$1 AND locale=$2 AND name=$3 AND summary=$4
 AND text_source_id IS NULL AND text_observation_id IS NULL;

-- name: DisableGameSources :exec
UPDATE game_sources SET active=false WHERE game_id=$1 AND active;

-- name: SaveGameSource :one
INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (platform_code,external_key) DO UPDATE SET source_url=excluded.source_url,active=true
WHERE game_sources.game_id=excluded.game_id RETURNING id;

-- name: PublishedGame :one
SELECT e.public_id,e.slug,game_published_text(e.id)::jsonb AS localizations,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('platform_code',x.platform_code,'external_key',x.external_key,'source_url',x.source_url) ORDER BY x.platform_code COLLATE "C",x.external_key COLLATE "C")
 FROM game_sources x WHERE x.game_id=e.id AND x.active),'[]'::jsonb)::jsonb AS sources
FROM game_channel_public e
WHERE e.public_id=$1;

-- name: PublishedGamePage :many
SELECT e.public_id,e.slug,game_published_text(e.id)::jsonb AS localizations
FROM game_channel_public e
WHERE e.public_id>sqlc.arg(after_public_id)
ORDER BY e.public_id LIMIT sqlc.arg(page_size);
