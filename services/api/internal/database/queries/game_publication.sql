-- name: PreviewGamePublication :one
SELECT coalesce(jsonb_agg(jsonb_build_object('locale',p.locale,'name',p.name,'summary',p.summary,
 'text_source_id',p.text_source_id,'text_observation_id',p.text_observation_id,
 'text_source',CASE WHEN ts.id IS NOT NULL THEN jsonb_build_object('platform_code',ts.platform_code,'external_key',ts.external_key,'source_url',ts.source_url) END,
 'media_source_id',m.media_source_id,'media_observation_id',m.media_observation_id,
 'media_source',CASE WHEN ms.id IS NOT NULL THEN jsonb_build_object('platform_code',ms.platform_code,'external_key',ms.external_key,'source_url',ms.source_url) END,
 'media_observed_at',m.media_observed_at,'media_store_country',coalesce(m.media_store_country,''),
 'media',coalesce(m.media,'{}'),'cover_url',coalesce(m.cover_url,''),'media_v2',v.media,'rich_details',rd.details) ORDER BY p.locale),'[]')::jsonb AS presentation
FROM game_localizations p
LEFT JOIN game_sources ts ON ts.id=p.text_source_id
LEFT JOIN game_draft_media m ON m.game_id=p.game_id AND m.locale=p.locale
LEFT JOIN game_draft_media_v2 v ON v.game_id=p.game_id AND v.locale=p.locale
LEFT JOIN game_draft_rich_details rd ON rd.game_id=p.game_id AND rd.locale=p.locale
LEFT JOIN game_sources ms ON ms.id=m.media_source_id
WHERE p.game_id=$1;

-- name: ReadManagementPublishedGame :one
SELECT coalesce(jsonb_agg(jsonb_build_object('locale',p.locale,'name',p.name,'summary',p.summary,
 'text_source_id',p.text_source_id,'text_observation_id',p.text_observation_id,
 'text_source',CASE WHEN ts.id IS NOT NULL THEN jsonb_build_object('platform_code',ts.platform_code,'external_key',ts.external_key,'source_url',ts.source_url) END,
 'media_source_id',p.media_source_id,'media_observation_id',p.media_observation_id,
 'media_source',CASE WHEN ms.id IS NOT NULL THEN jsonb_build_object('platform_code',ms.platform_code,'external_key',ms.external_key,'source_url',ms.source_url) END,
 'media_observed_at',p.media_observed_at,'media_store_country',p.media_store_country,'media',p.media,'cover_url',p.cover_url,
 'media_v2',p.media_v2,'rich_details',p.rich_details,'publication_revision',p.revision) ORDER BY p.locale),'[]')::jsonb AS presentation
FROM game_published_localizations p
LEFT JOIN game_sources ts ON ts.id=p.text_source_id
LEFT JOIN game_sources ms ON ms.id=p.media_source_id
WHERE p.game_id=$1;

-- name: PublishReviewedGame :one
UPDATE games SET status='published',content_rating='sfw',revision=revision+1,
 published_at=transaction_timestamp(),first_published_at=coalesce(first_published_at,published_at,transaction_timestamp()),
 updated_at=transaction_timestamp()
WHERE id=$1 AND status='draft' RETURNING *;

-- name: PublishReviewedGameChanges :one
UPDATE games SET slug=coalesce(draft_slug,slug),draft_slug=NULL,revision=revision+1,
 published_at=transaction_timestamp(),updated_at=transaction_timestamp()
WHERE id=$1 AND status='published' AND content_rating='sfw' RETURNING *;

-- name: ClearGamePublicationSnapshot :exec
DELETE FROM game_published_localizations WHERE game_id=$1;

-- name: SaveGamePublicationLocale :exec
INSERT INTO game_published_localizations
 (game_id,locale,name,summary,text_source_id,text_observation_id,media_source_id,media_observation_id,media,media_observed_at,media_store_country,cover_url,revision,media_v2,rich_details)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15);

-- name: SetDraftGameRating :one
UPDATE games SET content_rating=$2,revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 AND status='draft' RETURNING *;

-- name: ReadGameDraftMedia :many
SELECT locale,media,cover_url,media_source_id,media_observation_id,media_observed_at,media_store_country
FROM game_draft_media WHERE game_id=$1 ORDER BY locale;

-- name: ClearGameDraftMedia :exec
DELETE FROM game_draft_media WHERE game_id=$1;

-- name: SaveGameDraftMedia :exec
INSERT INTO game_draft_media (game_id,locale,media,cover_url,media_source_id,media_observation_id,media_observed_at,media_store_country)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8);

-- name: ManagementGameSourceCandidates :one
SELECT coalesce(jsonb_agg(to_jsonb(offers) ORDER BY observed_at DESC,source_id,observation_id,locale,section,store_country),'[]')::jsonb AS candidates
FROM (
 SELECT s.id AS source_id,m.observation_id,s.platform_code,s.external_key,
  CASE m.locale WHEN 'zh-cn' THEN 'zh' ELSE 'en' END::text AS locale,m.section,m.store_country,m.observed_at,
  CASE WHEN m.section='details' THEN left(btrim(m.payload->>'name'),160) ELSE '' END::text AS name,
  CASE WHEN m.section='details' THEN left(btrim(coalesce(m.payload->>'description','')),2000) ELSE '' END::text AS summary,
  CASE WHEN m.section='media' THEN m.payload ELSE '{}' END::jsonb AS media
 FROM game_sources s JOIN game_source_metadata m ON m.game_source_id=s.id
 WHERE s.game_id=sqlc.arg(game_id) AND s.active AND m.state='present' AND m.observation_id IS NOT NULL
  AND (m.section='media' OR (m.payload->>'type'='game' AND (m.locale='en' OR m.payload->>'localized'='true')))
 UNION ALL
 SELECT s.id,i.profile_observation_id,s.platform_code,s.external_key,i.profile->>'locale','details',
  CASE WHEN i.profile_scope_key<>'' AND i.profile->>'locale'='en' AND i.platform_code='steam' THEN 'us' ELSE '' END,i.profile_at,
  left(btrim(i.profile->>'name'),160),left(btrim(coalesce(i.profile->>'summary','')),2000),'{}'::jsonb
 FROM game_sources s JOIN game_intake_candidates i ON i.game_source_id=s.id AND i.game_id=s.game_id
 WHERE s.game_id=sqlc.arg(game_id) AND s.active AND i.state='accepted' AND i.profile_observation_id IS NOT NULL
  AND i.profile_at IS NOT NULL AND i.profile->>'locale' IN ('en','zh') AND btrim(i.profile->>'name')<>''
 ORDER BY observed_at DESC,source_id,observation_id,locale,section,store_country LIMIT 48
) offers;
