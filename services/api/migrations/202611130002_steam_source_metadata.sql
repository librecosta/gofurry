-- +goose Up
-- Steam details and media can exceed the compact Web payload budget.
ALTER TABLE collection_current DROP CONSTRAINT collection_current_data_check;
ALTER TABLE collection_current ADD CHECK (
 octet_length(data::text)<=CASE WHEN capability='steam.details' THEN 32768 ELSE 8192 END
);

CREATE TABLE game_source_metadata (
 game_source_id uuid NOT NULL CHECK (uuid_extract_version(game_source_id)=7),
 locale text NOT NULL CHECK (locale IN ('en','zh-cn')),
 store_country text NOT NULL CHECK (store_country IN ('cn','us','hk')),
 egress_region text NOT NULL CHECK (egress_region ~ '^[a-z0-9][a-z0-9_-]{0,34}$'),
 section text NOT NULL CHECK (section IN ('details','media')),
 binding_id uuid NOT NULL CHECK (uuid_extract_version(binding_id)=7),
 scope_key text NOT NULL CHECK (scope_key ~ '^[0-9a-f]{64}$'),
 epoch uuid NOT NULL CHECK (uuid_extract_version(epoch)=7),
 position bigint NOT NULL CHECK (position>0),
 event_at timestamptz NOT NULL,
 state text NOT NULL CHECK (state IN ('present','withdrawn')),
 output_schema integer NOT NULL CHECK (output_schema=1),
 ruleset text NOT NULL CHECK (ruleset IN ('gofurry-evidence-1','gofurry-steam-1')),
 observation_id uuid CHECK (uuid_extract_version(observation_id)=7),
 content_hash text CHECK (content_hash ~ '^[0-9a-f]{64}$'),
 observed_at timestamptz,
 confirmed_at timestamptz,
 payload jsonb,
 PRIMARY KEY (game_source_id,locale,store_country,egress_region,section),
 CHECK (state<>'present' OR payload IS NOT NULL),
 CHECK ((payload IS NOT NULL AND observation_id IS NOT NULL AND content_hash IS NOT NULL AND observed_at IS NOT NULL AND confirmed_at IS NOT NULL)
  OR (payload IS NULL AND observation_id IS NULL AND content_hash IS NULL AND observed_at IS NULL AND confirmed_at IS NULL)),
 CHECK (payload IS NULL OR (jsonb_typeof(payload)='object' AND octet_length(payload::text)<=32768 AND
  CASE section WHEN 'details' THEN
   jsonb_typeof(payload->'name')='string' AND length(payload->>'name')>0 AND
   payload-ARRAY['name','type','description','supported_languages','developers','publishers','platforms','categories','genres','release','localized']='{}'::jsonb
  ELSE payload-ARRAY['header','capsule','screenshots','movies']='{}'::jsonb AND payload<>'{}'::jsonb END))
);

ALTER TABLE game_approved_presentations ADD COLUMN steam_store_country text
 CHECK (steam_store_country IN ('cn','us','hk'));
ALTER TABLE game_approved_presentations ADD COLUMN steam_egress_region text
 CHECK (steam_egress_region ~ '^[a-z0-9][a-z0-9_-]{0,34}$');
UPDATE game_approved_presentations a SET steam_store_country='us',steam_egress_region=i.egress_region
FROM game_sources s JOIN game_intake_candidates i ON i.platform_code='steam' AND i.external_key=s.external_key AND i.game_source_id=s.id
WHERE a.locale='en' AND a.game_source_id=s.id AND a.game_id=s.game_id AND a.observation_id=i.profile_observation_id
 AND i.egress_region~'^[a-z0-9][a-z0-9_-]{0,34}$';

-- Keep the existing count cap per Steam news scope. News has no freshness
-- requirement and is no longer pruned by age.
DROP INDEX game_evidence_news_prune;
