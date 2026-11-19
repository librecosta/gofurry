-- name: PreviewGamePublication :one
SELECT coalesce(jsonb_agg(jsonb_build_object('locale',p.locale,'name',p.name,'summary',p.summary,
 'text_source_id',p.source_id,'text_observation_id',p.observation_id,
 'text_source',CASE WHEN ts.id IS NOT NULL THEN jsonb_build_object('platform_code',ts.platform_code,'external_key',ts.external_key,'source_url',ts.source_url) END,
 'media_source_id',ms.id,'media_source',CASE WHEN ms.id IS NOT NULL THEN jsonb_build_object('platform_code',ms.platform_code,'external_key',ms.external_key,'source_url',ms.source_url) END,
 'media_observed_at',m.observed_at,'media_store_country',coalesce(m.store_country,''),'media',jsonb_build_object('header',m.payload->>'header','capsule',m.payload->>'capsule',
 'screenshots',m.payload->'screenshots','movies',m.payload->'movies')) ORDER BY p.locale),'[]')::jsonb AS presentation
FROM games g CROSS JOIN LATERAL game_draft_text(g.id) p
LEFT JOIN game_sources ts ON ts.id=p.source_id
LEFT JOIN LATERAL (
 SELECT s.id,m.payload,m.observed_at,m.store_country FROM game_sources s JOIN game_source_metadata m ON m.game_source_id=s.id
 WHERE s.game_id=g.id AND s.active AND m.section='media' AND m.payload IS NOT NULL
 ORDER BY (s.id=p.source_id) DESC NULLS LAST,(s.id=g.intake_display_source) DESC NULLS LAST,
  (m.locale=CASE p.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END) DESC,m.observed_at DESC,s.id,m.locale,m.store_country,m.egress_region
 LIMIT 1
) m ON true
LEFT JOIN game_sources ms ON ms.id=m.id
WHERE g.id=$1;

-- name: ReadManagementPublishedGame :one
SELECT coalesce(jsonb_agg(jsonb_build_object('locale',p.locale,'name',p.name,'summary',p.summary,
 'text_source_id',p.text_source_id,'text_observation_id',p.text_observation_id,
 'text_source',CASE WHEN ts.id IS NOT NULL THEN jsonb_build_object('platform_code',ts.platform_code,'external_key',ts.external_key,'source_url',ts.source_url) END,
 'media_source_id',p.media_source_id,'media_source',CASE WHEN ms.id IS NOT NULL THEN jsonb_build_object('platform_code',ms.platform_code,'external_key',ms.external_key,'source_url',ms.source_url) END,
 'media_observed_at',p.media_observed_at,'media_store_country',p.media_store_country,'media',p.media,'cover_url',p.cover_url) ORDER BY p.locale),'[]')::jsonb AS presentation
FROM game_published_localizations p
LEFT JOIN game_sources ts ON ts.id=p.text_source_id
LEFT JOIN game_sources ms ON ms.id=p.media_source_id
WHERE p.game_id=$1;

-- name: PublishReviewedGame :one
UPDATE games SET status='published',content_rating='sfw',revision=revision+1,
 published_at=transaction_timestamp(),first_published_at=coalesce(first_published_at,published_at,transaction_timestamp()),
 updated_at=transaction_timestamp()
WHERE id=$1 AND status='draft' RETURNING *;

-- name: ClearGamePublicationSnapshot :exec
DELETE FROM game_published_localizations WHERE game_id=$1;

-- name: SaveGamePublicationLocale :exec
INSERT INTO game_published_localizations
 (game_id,locale,name,summary,text_source_id,text_observation_id,media_source_id,media,media_observed_at,media_store_country,cover_url,revision)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: SetDraftGameRating :one
UPDATE games SET content_rating=$2,revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 AND status='draft' RETURNING *;
