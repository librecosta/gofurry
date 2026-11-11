-- +goose Up
ALTER TABLE site_addresses ADD UNIQUE (id,site_id);
ALTER TABLE game_sources ADD UNIQUE (id,game_id);

CREATE TABLE collection_sources (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 namespace text NOT NULL,
 provider text NOT NULL,
 resource_type text NOT NULL,
 external_key text NOT NULL CHECK (length(external_key) BETWEEN 1 AND 512),
 target_id uuid UNIQUE,
 checked_at timestamptz,
 UNIQUE (namespace,provider,resource_type,external_key),
 UNIQUE (id,resource_type),
 CHECK ((provider='web' AND resource_type IN ('origin','domain')) OR (provider='steam' AND resource_type='app'))
);
CREATE TABLE collection_bindings (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 source_id uuid NOT NULL,
 resource_type text NOT NULL,
 site_id uuid REFERENCES sites(id) ON DELETE RESTRICT,
 game_id uuid REFERENCES games(id) ON DELETE RESTRICT,
 address_id uuid,
 game_source_id uuid,
 purpose text NOT NULL,
 effective_from timestamptz NOT NULL DEFAULT transaction_timestamp(),
 effective_until timestamptz CHECK (effective_until>=effective_from),
 FOREIGN KEY (source_id,resource_type) REFERENCES collection_sources(id,resource_type),
 FOREIGN KEY (address_id,site_id) REFERENCES site_addresses(id,site_id),
 FOREIGN KEY (game_source_id,game_id) REFERENCES game_sources(id,game_id),
 UNIQUE (id,source_id),
 CHECK (
  (resource_type IN ('domain','origin') AND site_id IS NOT NULL AND address_id IS NOT NULL AND game_id IS NULL AND game_source_id IS NULL
   AND ((resource_type='domain' AND purpose='domain') OR (resource_type='origin' AND purpose IN ('primary','alternate'))))
  OR (resource_type='app' AND game_id IS NOT NULL AND game_source_id IS NOT NULL AND site_id IS NULL AND address_id IS NULL AND purpose='steam')
 )
);
CREATE UNIQUE INDEX collection_open_site_address ON collection_bindings (source_id,address_id) WHERE effective_until IS NULL AND address_id IS NOT NULL;
CREATE UNIQUE INDEX collection_open_game_source ON collection_bindings (source_id,game_source_id) WHERE effective_until IS NULL AND game_source_id IS NOT NULL;
CREATE INDEX collection_binding_site ON collection_bindings (site_id) WHERE effective_until IS NULL;
CREATE INDEX collection_binding_game ON collection_bindings (game_id) WHERE effective_until IS NULL;
CREATE INDEX collection_binding_period ON collection_bindings (source_id,effective_from,effective_until);

CREATE TABLE collection_plans (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 source_id uuid NOT NULL REFERENCES collection_sources(id),
 capability text NOT NULL,
 request_schema integer NOT NULL CHECK (request_schema=1),
 params jsonb NOT NULL CHECK (jsonb_typeof(params)='object' AND octet_length(params::text)<=512),
 scope jsonb NOT NULL CHECK (jsonb_typeof(scope)='object' AND octet_length(scope::text)<=256),
 interval_seconds integer NOT NULL CHECK (interval_seconds BETWEEN 900 AND 2592000),
 anchor_at timestamptz NOT NULL,
 policy text NOT NULL CHECK (policy IN ('state_refresh','point_in_time')),
 schedule_id uuid UNIQUE,
 remote_revision bigint,
 remote_enabled boolean,
 post_started boolean NOT NULL DEFAULT false,
 checked_at timestamptz,
 UNIQUE (source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy),
 UNIQUE (id,source_id),
 CHECK ((schedule_id IS NULL AND remote_revision IS NULL AND remote_enabled IS NULL) OR
        (schedule_id IS NOT NULL AND remote_revision>0 AND remote_enabled IS NOT NULL))
);
CREATE TABLE collection_plan_bindings (
 plan_id uuid NOT NULL,
 binding_id uuid NOT NULL,
 source_id uuid NOT NULL,
 attached_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 PRIMARY KEY (plan_id,binding_id),
 FOREIGN KEY (plan_id,source_id) REFERENCES collection_plans(id,source_id),
 FOREIGN KEY (binding_id,source_id) REFERENCES collection_bindings(id,source_id)
);
CREATE INDEX collection_plan_membership ON collection_plan_bindings(binding_id);
CREATE INDEX collection_plans_check ON collection_plans(checked_at NULLS FIRST,id);

CREATE TABLE collection_consumer (
 namespace text PRIMARY KEY,
 consumer_id uuid NOT NULL,
 generation uuid,
 epoch uuid,
 cursor text NOT NULL DEFAULT '' CHECK (length(cursor)<=1024),
 ack_pending boolean NOT NULL DEFAULT false,
 reset_ack uuid,
 sweep_after text NOT NULL DEFAULT '' CHECK (length(sweep_after)<=1024),
 sweep_at timestamptz NOT NULL DEFAULT '-infinity',
 CHECK ((generation IS NULL AND epoch IS NULL AND cursor='') OR (generation IS NOT NULL AND epoch IS NOT NULL AND cursor<>''))
);
CREATE TABLE collection_gaps (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 namespace text NOT NULL REFERENCES collection_consumer(namespace),
 prior_epoch uuid NOT NULL,
 prior_cursor text NOT NULL CHECK (length(prior_cursor)<=1024),
 detected_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE (namespace,prior_epoch,prior_cursor)
);
CREATE TABLE collection_current (
 source_id uuid NOT NULL REFERENCES collection_sources(id),
 generation uuid NOT NULL,
 capability text NOT NULL,
 output_schema integer NOT NULL CHECK (output_schema=1),
 scope_key text NOT NULL CHECK (scope_key ~ '^[0-9a-f]{64}$'),
 scope jsonb NOT NULL CHECK (octet_length(scope::text)<=512),
 position bigint NOT NULL CHECK (position>0),
 state text NOT NULL CHECK (state IN ('present','confirmed_empty','withdrawn')),
 observation_id uuid,
 observed_at timestamptz NOT NULL,
 received_at timestamptz NOT NULL,
 confirmed_at timestamptz,
 content_hash text NOT NULL CHECK (content_hash='' OR content_hash ~ '^[0-9a-f]{64}$'),
 evidence text NOT NULL,
 ruleset text NOT NULL,
 data jsonb NOT NULL CHECK (octet_length(data::text)<=8192),
 facts jsonb NOT NULL CHECK (octet_length(facts::text)<=8192),
 PRIMARY KEY (generation,source_id,capability,output_schema,scope_key)
);

CREATE TABLE site_evidence_changes (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 site_id uuid NOT NULL REFERENCES sites(id),
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 target_id uuid NOT NULL,
 epoch uuid NOT NULL,
 position bigint NOT NULL,
 capability text NOT NULL,
 output_schema integer NOT NULL,
 scope_key text NOT NULL,
 scope jsonb NOT NULL CHECK (octet_length(scope::text)<=512),
 fact_key text NOT NULL CHECK (length(fact_key)<=96),
 event_kind text NOT NULL CHECK (event_kind IN ('baseline','change','withdrawn')),
 prior_value jsonb CHECK (octet_length(prior_value::text)<=6144),
 value jsonb CHECK (octet_length(value::text)<=6144),
 ruleset text NOT NULL,
 prior_ruleset text NOT NULL,
 source_observation_id uuid,
 content_hash text NOT NULL,
 evidence text NOT NULL,
 first_observed_at timestamptz NOT NULL,
 source_observed_at timestamptz NOT NULL,
 received_at timestamptz NOT NULL,
 UNIQUE (binding_id,epoch,position,fact_key)
);
CREATE INDEX site_evidence_timeline ON site_evidence_changes (site_id,first_observed_at,id);
CREATE INDEX site_evidence_attribution ON site_evidence_changes (binding_id,capability,scope_key,fact_key);
CREATE TABLE site_daily_coverage (
 site_id uuid NOT NULL REFERENCES sites(id),
 source_id uuid NOT NULL REFERENCES collection_sources(id),
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 capability text NOT NULL,
 output_schema integer NOT NULL,
 scope_key text NOT NULL,
 utc_day date NOT NULL,
 confirmed_at timestamptz NOT NULL,
 evidence_state text NOT NULL CHECK (evidence_state IN ('present','confirmed_empty')),
 observation_id uuid NOT NULL,
 PRIMARY KEY (site_id,source_id,capability,output_schema,scope_key,utc_day)
);
CREATE INDEX site_daily_coverage_timeline ON site_daily_coverage (site_id,utc_day,source_id,capability,output_schema,scope_key);

CREATE TABLE game_evidence_news (
 source_id uuid NOT NULL REFERENCES collection_sources(id),
 scope_key text NOT NULL,
 gid text NOT NULL CHECK (gid ~ '^[0-9]{1,20}$'),
 published_at timestamptz NOT NULL,
 observed_at timestamptz NOT NULL,
 observation_id uuid NOT NULL,
 value jsonb NOT NULL CHECK (octet_length(value::text)<=2048),
 PRIMARY KEY (source_id,scope_key,gid)
);
CREATE INDEX game_evidence_news_recent ON game_evidence_news (source_id,scope_key,published_at DESC,gid DESC);
CREATE INDEX game_evidence_news_prune ON game_evidence_news (published_at);
CREATE TABLE game_evidence_players (
 source_id uuid NOT NULL REFERENCES collection_sources(id),
 scope_key text NOT NULL,
 bucket timestamptz NOT NULL,
 observed_at timestamptz NOT NULL,
 observation_id uuid NOT NULL,
 players bigint NOT NULL CHECK (players>=0),
 PRIMARY KEY (source_id,scope_key,bucket)
);
CREATE INDEX game_evidence_players_prune ON game_evidence_players(bucket);

-- +goose Down
DROP TABLE game_evidence_players;
DROP TABLE game_evidence_news;
DROP TABLE site_daily_coverage;
DROP TABLE site_evidence_changes;
DROP TABLE collection_current;
DROP TABLE collection_gaps;
DROP TABLE collection_consumer;
DROP TABLE collection_plan_bindings;
DROP TABLE collection_plans;
DROP TABLE collection_bindings;
DROP TABLE collection_sources;
ALTER TABLE game_sources DROP CONSTRAINT game_sources_id_game_id_key;
ALTER TABLE site_addresses DROP CONSTRAINT site_addresses_id_site_id_key;
