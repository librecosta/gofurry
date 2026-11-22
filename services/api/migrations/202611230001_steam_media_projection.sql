-- +goose Up
ALTER TABLE collection_plans ADD COLUMN output_schema integer NOT NULL DEFAULT 1
 CHECK (output_schema=1 OR (capability='steam.details' AND output_schema=2)),
 ADD CONSTRAINT collection_plans_assets_interval CHECK (capability<>'steam.assets' OR
  (interval_seconds>=21600 AND (interval_override_seconds=0 OR interval_override_seconds>=21600)));

-- +goose StatementBegin
DO $$
DECLARE old_key name;
BEGIN
 SELECT conname INTO STRICT old_key FROM pg_constraint
 WHERE conrelid='collection_plans'::regclass AND contype='u'
 AND pg_get_constraintdef(oid)='UNIQUE (source_id, capability, request_schema, params, scope, interval_seconds, anchor_at, policy)';
 EXECUTE format('ALTER TABLE collection_plans DROP CONSTRAINT %I',old_key);
END $$;
-- +goose StatementEnd

ALTER TABLE collection_plans ADD CONSTRAINT collection_plans_versioned_identity
 UNIQUE (source_id,capability,request_schema,output_schema,params,scope,interval_seconds,anchor_at,policy);
ALTER TABLE collection_current ADD COLUMN execution_egress_region text NOT NULL DEFAULT ''
 CHECK (execution_egress_region='' OR execution_egress_region ~ '^[a-z0-9][a-z0-9_-]{0,34}$'),
 DROP CONSTRAINT collection_current_output_schema_check,
 ADD CONSTRAINT collection_current_output_schema_check CHECK (output_schema=1 OR (capability='steam.details' AND output_schema=2));
ALTER TABLE collection_current DROP CONSTRAINT collection_current_pkey,
 ADD PRIMARY KEY (generation,source_id,capability,output_schema,scope_key,execution_egress_region);
-- PostgreSQL renders additional JSONB whitespace. The Go wire limit is 32 KiB.
ALTER TABLE collection_current DROP CONSTRAINT collection_current_check,
 ADD CONSTRAINT collection_current_payload_budget CHECK (octet_length(data::text)<=CASE
  WHEN capability='steam.assets' OR (capability='steam.details' AND output_schema=2 AND scope#>>'{dimensions,section}'='media') THEN 40960
  WHEN capability='steam.details' THEN 32768 ELSE 8192 END);

CREATE TABLE game_source_media (
 id uuid NOT NULL UNIQUE DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 game_source_id uuid NOT NULL REFERENCES game_sources(id),
 collection_source_id uuid NOT NULL REFERENCES collection_sources(id),
 binding_id uuid NOT NULL,
 target_id uuid NOT NULL CHECK (uuid_extract_version(target_id)=7),
 namespace text NOT NULL CHECK (length(namespace) BETWEEN 1 AND 100),
 capability text NOT NULL,
 output_schema integer NOT NULL,
 locale text NOT NULL CHECK (locale IN ('en','zh-cn')),
 store_country text NOT NULL CHECK (store_country IN ('cn','us','hk')),
 egress_region text NOT NULL CHECK (egress_region ~ '^[a-z0-9][a-z0-9_-]{0,34}$'),
 requested_egress_region text NOT NULL CHECK (requested_egress_region ~ '^[a-z0-9][a-z0-9_-]{0,34}$'),
 scope_key text NOT NULL CHECK (scope_key ~ '^[0-9a-f]{64}$'),
 epoch uuid NOT NULL,
 position bigint NOT NULL CHECK (position>0),
 event_at timestamptz NOT NULL,
 state text NOT NULL CHECK (state IN ('present','confirmed_empty','withdrawn')),
 observation_id uuid CHECK (uuid_extract_version(observation_id)=7),
 content_hash text CHECK (content_hash ~ '^[0-9a-f]{64}$'),
 ruleset text NOT NULL CHECK (length(ruleset) BETWEEN 1 AND 96),
 observed_at timestamptz NOT NULL,
 confirmed_at timestamptz NOT NULL,
 payload jsonb,
 FOREIGN KEY (binding_id,collection_source_id) REFERENCES collection_bindings(id,source_id),
 PRIMARY KEY (game_source_id,namespace,capability,output_schema,scope_key,egress_region),
 CHECK ((capability='steam.details' AND output_schema=2 AND state<>'confirmed_empty')
     OR (capability='steam.assets' AND output_schema=1)),
 CHECK ((state='withdrawn' AND payload IS NULL AND observation_id IS NULL AND content_hash IS NULL)
     OR (state<>'withdrawn' AND observation_id IS NOT NULL AND content_hash IS NOT NULL
         AND payload IS NOT NULL AND jsonb_typeof(payload)='object' AND octet_length(payload::text)<=40960))
);
CREATE INDEX game_source_media_observation ON game_source_media (game_source_id,observation_id) WHERE state='present';
CREATE INDEX game_source_media_page ON game_source_media (game_source_id,id) WHERE state IN ('present','confirmed_empty');

-- This table is independent of the legacy replace-all media edit.
CREATE TABLE game_draft_media_v2 (
 game_id uuid NOT NULL REFERENCES games(id),
 locale text NOT NULL CHECK (locale IN ('en','zh')),
 media jsonb NOT NULL CONSTRAINT game_draft_media_v2_contract CHECK (
  jsonb_typeof(media)='object' AND media @> '{"version":2}' AND octet_length(media::text)<=131072),
 PRIMARY KEY (game_id,locale)
);
ALTER TABLE game_published_localizations ADD COLUMN media_v2 jsonb CHECK (
 media_v2 IS NULL OR (jsonb_typeof(media_v2)='object' AND media_v2 @> '{"version":2}' AND octet_length(media_v2::text)<=131072));

-- +goose Down
-- Rollback is only lossless before high-fidelity plans or media are used.
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM collection_plans WHERE output_schema<>1 OR capability='steam.assets')
 OR EXISTS (SELECT 1 FROM collection_current WHERE output_schema<>1 OR capability='steam.assets')
 OR EXISTS (SELECT 1 FROM game_source_media)
 OR EXISTS (SELECT 1 FROM game_draft_media_v2)
 OR EXISTS (SELECT 1 FROM game_published_localizations WHERE media_v2 IS NOT NULL) THEN
  RAISE EXCEPTION 'Steam media migration contains data';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE game_published_localizations DROP COLUMN media_v2;
DROP TABLE game_draft_media_v2, game_source_media;
ALTER TABLE collection_current DROP CONSTRAINT collection_current_payload_budget,
 DROP CONSTRAINT collection_current_pkey,
 DROP COLUMN execution_egress_region,
 DROP CONSTRAINT collection_current_output_schema_check,
 ADD CONSTRAINT collection_current_output_schema_check CHECK (output_schema=1),
 ADD CONSTRAINT collection_current_check CHECK (octet_length(data::text)<=CASE WHEN capability='steam.details' THEN 32768 ELSE 8192 END),
 ADD PRIMARY KEY (generation,source_id,capability,output_schema,scope_key);
ALTER TABLE collection_plans DROP CONSTRAINT collection_plans_versioned_identity, DROP CONSTRAINT collection_plans_assets_interval,
 DROP COLUMN output_schema, ADD UNIQUE (source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy);
