-- +goose Up
ALTER TABLE games ADD COLUMN draft_slug text CHECK (
 draft_slug IS NULL OR (status IN ('published','archived') AND draft_slug<>slug
 AND length(draft_slug) BETWEEN 1 AND 80 AND draft_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'));

ALTER TABLE game_localizations ADD COLUMN text_source_id uuid, ADD COLUMN text_observation_id uuid,
 ADD CHECK ((text_source_id IS NULL)=(text_observation_id IS NULL));
ALTER TABLE game_published_localizations ADD COLUMN media_observation_id uuid;

CREATE TABLE game_draft_media (
 game_id uuid NOT NULL,
 locale text NOT NULL CHECK (locale IN ('en','zh')),
 media jsonb NOT NULL CHECK (
  jsonb_typeof(media)='object' AND octet_length(media::text)<=32768
  AND media-ARRAY['header','capsule','screenshots','movies']='{}'
  AND (NOT media?'header' OR jsonb_typeof(media->'header')='string')
  AND (NOT media?'capsule' OR jsonb_typeof(media->'capsule')='string')
  AND CASE WHEN media?'screenshots' THEN jsonb_typeof(media->'screenshots')='array'
   AND NOT jsonb_path_exists(media, '$.screenshots[*] ? (@.type() != "string")')
   AND jsonb_path_exists(media, '$.screenshots ? (@.size() <= 8)') ELSE true END
  AND CASE WHEN media?'movies' THEN jsonb_typeof(media->'movies')='array'
   AND NOT jsonb_path_exists(media, '$.movies[*] ? (@.type() != "string")')
   AND jsonb_path_exists(media, '$.movies ? (@.size() <= 4)') ELSE true END),
 cover_url text NOT NULL CHECK (cover_url='' OR (length(cover_url)<=2048 AND cover_url LIKE 'https://%')),
 media_source_id uuid,
 media_observation_id uuid,
 media_observed_at timestamptz,
 media_store_country text NOT NULL CHECK (media_store_country='' OR media_store_country ~ '^[a-zA-Z]{2}$'),
 PRIMARY KEY (game_id,locale),
 CHECK (media_observation_id IS NULL OR (media_source_id IS NOT NULL AND media_observed_at IS NOT NULL)),
 CHECK (media_source_id IS NOT NULL OR (media_observed_at IS NULL AND media_store_country=''))
);

INSERT INTO game_localizations (game_id,locale,name,summary,text_source_id,text_observation_id)
SELECT p.game_id,p.locale,p.name,p.summary,
 CASE WHEN p.text_observation_id IS NOT NULL THEN p.text_source_id END,
 CASE WHEN p.text_source_id IS NOT NULL THEN p.text_observation_id END
FROM game_published_localizations p JOIN games g ON g.id=p.game_id AND g.status='published'
ON CONFLICT (game_id,locale) DO NOTHING;

-- R1-A had no media Observation ID in the snapshot. Keep its known attribution.
INSERT INTO game_draft_media (game_id,locale,media,cover_url,media_source_id,media_observed_at,media_store_country)
SELECT p.game_id,p.locale,p.media,p.cover_url,p.media_source_id,p.media_observed_at,p.media_store_country
FROM game_published_localizations p JOIN games g ON g.id=p.game_id AND g.status='published'
WHERE p.media<>'{}' OR p.cover_url<>'';

INSERT INTO game_localizations (game_id,locale,name,summary,text_source_id,text_observation_id)
SELECT DISTINCT ON (g.id,i.profile->>'locale') g.id,i.profile->>'locale',left(btrim(i.profile->>'name'),160),
 left(btrim(coalesce(i.profile->>'summary','')),2000),s.id,i.profile_observation_id
FROM games g JOIN game_intake_candidates i ON i.game_id=g.id AND i.state='accepted'
JOIN game_sources s ON s.id=i.game_source_id AND s.game_id=g.id AND s.active
WHERE g.status='draft' AND i.profile_observation_id IS NOT NULL
 AND i.profile->>'locale' IN ('en','zh') AND btrim(i.profile->>'name')<>''
 AND NOT EXISTS (SELECT 1 FROM game_localizations l WHERE l.game_id=g.id)
ORDER BY g.id,i.profile->>'locale',(s.id=g.intake_display_source) DESC NULLS LAST,i.profile_at DESC NULLS LAST,i.id;

ALTER TABLE catalog_publication_decisions DROP CONSTRAINT catalog_publication_decisions_action_check,
 ADD CHECK (action IN ('publish','publish_changes','unpublish','archive','restore','block','clear_block','rating'));
DROP FUNCTION game_draft_text(uuid);
