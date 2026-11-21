-- +goose Up
ALTER TABLE collection_plan_bindings ADD COLUMN enabled boolean NOT NULL DEFAULT true;

UPDATE collection_plan_bindings b SET enabled=false
FROM collection_plans p WHERE p.id=b.plan_id AND p.remote_enabled=false;

CREATE TABLE steam_editorial_prefills (
 namespace text NOT NULL,
 app_id text NOT NULL CHECK (app_id ~ '^[1-9][0-9]{0,9}$'),
 locale text NOT NULL CHECK (locale IN ('en','zh-cn')),
 generation uuid NOT NULL CHECK (uuid_extract_version(generation)=7),
 started_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 submission_code text NOT NULL DEFAULT '',
 job_id uuid,
 target_id uuid,
 text_observation_id uuid,
 media_observation_id uuid,
 name text NOT NULL DEFAULT '' CHECK (length(name)<=160),
 summary text NOT NULL DEFAULT '' CHECK (length(summary)<=2000),
 facts jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(facts)='object' AND octet_length(facts::text)<=8192),
 media jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(media)='object' AND octet_length(media::text)<=16384),
 PRIMARY KEY (namespace,app_id,locale),
 CHECK ((job_id IS NULL)=(target_id IS NULL)),
 CHECK (submission_code IN ('','rate_limited','harvest_unavailable','submission_unconfirmed'))
);
CREATE INDEX steam_editorial_prefills_rate_idx ON steam_editorial_prefills (namespace,started_at DESC);
