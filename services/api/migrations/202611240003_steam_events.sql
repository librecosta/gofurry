-- +goose Up
ALTER TABLE collection_current DROP CONSTRAINT collection_current_payload_budget,
 ADD CONSTRAINT collection_current_payload_budget CHECK (octet_length(data::text)<=CASE
 WHEN capability='steam.events' AND output_schema=1 AND scope#>>'{dimensions,section}'='events' THEN 524288
 WHEN capability='steam.details' AND output_schema=3 AND scope#>>'{dimensions,section}'='rich_details' THEN 524288
 WHEN capability='steam.assets' OR (capability='steam.details' AND output_schema IN (2,3) AND scope#>>'{dimensions,section}'='media') THEN 40960
 WHEN capability='steam.details' THEN 32768 ELSE 8192 END);

CREATE TABLE game_steam_events (
 id uuid PRIMARY KEY,
 game_source_id uuid NOT NULL REFERENCES game_sources(id),
 collection_source_id uuid NOT NULL REFERENCES collection_sources(id),
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 target_id uuid NOT NULL,
 namespace text NOT NULL,
 scope_key text NOT NULL CHECK (scope_key ~ '^[0-9a-f]{64}$'),
 requested_locale text NOT NULL CHECK (requested_locale IN ('en','zh-cn')),
 egress_region text NOT NULL CHECK (length(egress_region) BETWEEN 1 AND 35),
 observation_id uuid NOT NULL,
 content_hash text NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
 observed_at timestamptz NOT NULL,
 confirmed_at timestamptz NOT NULL,
 event_gid text NOT NULL CHECK (event_gid ~ '^[1-9][0-9]{0,19}$'),
 announcement_gid text NOT NULL CHECK (announcement_gid ~ '^[1-9][0-9]{0,19}$'),
 language integer NOT NULL CHECK (language>=-1),
 headline text NOT NULL CHECK (octet_length(headline) BETWEEN 1 AND 2048),
 published_at timestamptz,
 updated_at timestamptz,
 eligible boolean NOT NULL,
 hidden boolean NOT NULL DEFAULT false,
 revision integer NOT NULL DEFAULT 1 CHECK (revision>0),
 body_hash text NOT NULL CHECK (body_hash ~ '^[0-9a-f]{64}$'),
 metadata jsonb NOT NULL CHECK (jsonb_typeof(metadata)='object' AND octet_length(metadata::text)<=24576),
 metrics jsonb NOT NULL CHECK (jsonb_typeof(metrics)='object' AND octet_length(metrics::text)<=512),
 UNIQUE (game_source_id,egress_region,event_gid,language)
);
CREATE INDEX game_steam_events_news ON game_steam_events (game_source_id,egress_region,published_at DESC,announcement_gid DESC)
 WHERE published_at IS NOT NULL;
CREATE INDEX game_steam_events_ops ON game_steam_events (game_source_id,id DESC);

CREATE TABLE game_steam_event_versions (
 event_id uuid NOT NULL REFERENCES game_steam_events(id) ON DELETE CASCADE,
 body_hash text NOT NULL CHECK (body_hash ~ '^[0-9a-f]{64}$'),
 headline text NOT NULL CHECK (octet_length(headline) BETWEEN 1 AND 2048),
 raw_body text NOT NULL CHECK (octet_length(raw_body) BETWEEN 1 AND 131072),
 html text NOT NULL CHECK (octet_length(html)<=262144),
 plain_text text NOT NULL CHECK (octet_length(plain_text)<=262144),
 summary text NOT NULL CHECK (length(summary)<=180),
 observation_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 updated_at timestamptz,
 PRIMARY KEY(event_id,body_hash)
);

-- +goose Down
DROP TABLE game_steam_event_versions;
DROP TABLE game_steam_events;
ALTER TABLE collection_current DROP CONSTRAINT collection_current_payload_budget,
 ADD CONSTRAINT collection_current_payload_budget CHECK (octet_length(data::text)<=CASE
 WHEN capability='steam.details' AND output_schema=3 AND scope#>>'{dimensions,section}'='rich_details' THEN 524288
 WHEN capability='steam.assets' OR (capability='steam.details' AND output_schema IN (2,3) AND scope#>>'{dimensions,section}'='media') THEN 40960
 WHEN capability='steam.details' THEN 32768 ELSE 8192 END);
