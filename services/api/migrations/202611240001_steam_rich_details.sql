-- +goose Up
ALTER TABLE collection_plans DROP CONSTRAINT collection_plans_check2,
 ADD CONSTRAINT collection_plans_output_schema_check CHECK (output_schema=1 OR (capability='steam.details' AND output_schema IN (2,3)));
ALTER TABLE collection_current DROP CONSTRAINT collection_current_output_schema_check,
 ADD CONSTRAINT collection_current_output_schema_check CHECK (output_schema=1 OR (capability='steam.details' AND output_schema IN (2,3))),
 DROP CONSTRAINT collection_current_payload_budget,
 ADD CONSTRAINT collection_current_payload_budget CHECK (octet_length(data::text)<=CASE
 WHEN capability='steam.details' AND output_schema=3 AND scope#>>'{dimensions,section}'='rich_details' THEN 524288
 WHEN capability='steam.assets' OR (capability='steam.details' AND output_schema IN (2,3) AND scope#>>'{dimensions,section}'='media') THEN 40960
 WHEN capability='steam.details' THEN 32768 ELSE 8192 END);

-- Reuse the existing durable Steam source projection and its binding and replay rules.
ALTER TABLE game_source_media DROP CONSTRAINT game_source_media_check,
 DROP CONSTRAINT game_source_media_check1,
 ADD CONSTRAINT game_steam_projection_version CHECK (
 (capability='steam.details' AND output_schema IN (2,3) AND state<>'confirmed_empty')
 OR (capability='steam.assets' AND output_schema=1)),
 ADD CONSTRAINT game_steam_projection_payload CHECK (
 (state='withdrawn' AND payload IS NULL AND observation_id IS NULL AND content_hash IS NULL)
 OR (state<>'withdrawn' AND observation_id IS NOT NULL AND content_hash IS NOT NULL
 AND payload IS NOT NULL AND jsonb_typeof(payload)='object'
 AND octet_length(payload::text)<=CASE WHEN ruleset='gofurry-steam-rich-3' AND output_schema=3 THEN 524288 ELSE 40960 END));

CREATE TABLE game_draft_rich_details (
 game_id uuid NOT NULL REFERENCES games(id),
 locale text NOT NULL CHECK (locale IN ('en','zh')),
 details jsonb NOT NULL CHECK (jsonb_typeof(details)='object' AND octet_length(details::text)<=524288),
 PRIMARY KEY (game_id,locale)
);
ALTER TABLE game_published_localizations ADD COLUMN rich_details jsonb CHECK (
 rich_details IS NULL OR (jsonb_typeof(rich_details)='object' AND octet_length(rich_details::text)<=524288));

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM collection_plans WHERE output_schema=3)
 OR EXISTS (SELECT 1 FROM collection_current WHERE output_schema=3)
 OR EXISTS (SELECT 1 FROM game_source_media WHERE output_schema=3)
 OR EXISTS (SELECT 1 FROM game_draft_rich_details)
 OR EXISTS (SELECT 1 FROM game_published_localizations WHERE rich_details IS NOT NULL) THEN
  RAISE EXCEPTION 'Steam v3 migration contains data';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE game_published_localizations DROP COLUMN rich_details;
DROP TABLE game_draft_rich_details;
ALTER TABLE game_source_media DROP CONSTRAINT game_steam_projection_version,
 DROP CONSTRAINT game_steam_projection_payload,
 ADD CONSTRAINT game_source_media_check CHECK ((capability='steam.details' AND output_schema=2 AND state<>'confirmed_empty')
 OR (capability='steam.assets' AND output_schema=1)),
 ADD CONSTRAINT game_source_media_check1 CHECK (
 (state='withdrawn' AND payload IS NULL AND observation_id IS NULL AND content_hash IS NULL)
 OR (state<>'withdrawn' AND observation_id IS NOT NULL AND content_hash IS NOT NULL
 AND payload IS NOT NULL AND jsonb_typeof(payload)='object' AND octet_length(payload::text)<=40960));
ALTER TABLE collection_current DROP CONSTRAINT collection_current_output_schema_check,
 ADD CONSTRAINT collection_current_output_schema_check CHECK (output_schema=1 OR (capability='steam.details' AND output_schema=2)),
 DROP CONSTRAINT collection_current_payload_budget,
 ADD CONSTRAINT collection_current_payload_budget CHECK (octet_length(data::text)<=CASE
 WHEN capability='steam.assets' OR (capability='steam.details' AND output_schema=2 AND scope#>>'{dimensions,section}'='media') THEN 40960
 WHEN capability='steam.details' THEN 32768 ELSE 8192 END);
ALTER TABLE collection_plans DROP CONSTRAINT collection_plans_output_schema_check,
 ADD CONSTRAINT collection_plans_check2 CHECK (output_schema=1 OR (capability='steam.details' AND output_schema=2));
