-- +goose Up
CREATE TABLE steam_game_daily_facts (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 day date NOT NULL,
 locale text NOT NULL CHECK (locale IN ('en','zh-cn')),
 market text NOT NULL CHECK (market IN ('us','cn','hk')),
 egress text NOT NULL CHECK (length(egress) BETWEEN 1 AND 64),
 kind text NOT NULL CHECK (kind IN ('release','languages','free','windows','mac','linux')),
 value jsonb NOT NULL CHECK (octet_length(value::text)<=98304),
 observation_id uuid NOT NULL CHECK (uuid_extract_version(observation_id)=7),
 observed_at timestamptz NOT NULL,
 confirmed_at timestamptz NOT NULL,
 normalizer integer NOT NULL CHECK (normalizer>0),
 UNIQUE (binding_id,locale,market,egress,kind,day),
 CHECK (day=(observed_at AT TIME ZONE 'UTC')::date AND observed_at<=confirmed_at)
);
CREATE INDEX steam_daily_latest ON steam_game_daily_facts (binding_id,market,locale,kind,day DESC,observed_at DESC);
CREATE INDEX steam_price_tracking_day ON game_price_observations (binding_id,utc_day);
CREATE INDEX steam_price_tracking_scope ON game_price_observations
 (binding_id,egress_region,store_country,utc_day DESC,observed_at DESC,observation_id DESC);
ALTER TABLE game_price_observations
 ADD COLUMN confirmed_at timestamptz,
 ADD COLUMN output_schema integer NOT NULL DEFAULT 1,
 DROP CONSTRAINT game_price_observations_check,
 ADD CONSTRAINT steam_price_confirmation_time CHECK (confirmed_at>=observed_at),
 ADD CONSTRAINT steam_price_market_currency CHECK (
  (output_schema=1 AND (store_country,currency) IN (('cn','CNY'),('us','USD'),('hk','HKD')))
  OR (output_schema=3 AND store_country IN ('cn','us','hk') AND currency ~ '^[A-Z]{3}$'));

CREATE TABLE steam_fact_reset_inputs (
 generation uuid NOT NULL,
 source_id uuid NOT NULL,
 capability text NOT NULL,
 output_schema integer NOT NULL,
 scope_key text NOT NULL,
 scope jsonb NOT NULL,
 state text NOT NULL CHECK (state='present'),
 ruleset text NOT NULL,
 observation_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 confirmed_at timestamptz NOT NULL,
 execution_egress_region text NOT NULL,
 data jsonb NOT NULL CHECK (octet_length(data::text)<=393216),
 recorded_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 PRIMARY KEY (generation,source_id,scope_key,output_schema,execution_egress_region,observation_id)
);
CREATE INDEX steam_fact_reset_expiration ON steam_fact_reset_inputs (recorded_at);

CREATE VIEW steam_fact_inputs AS
SELECT generation,source_id,capability,output_schema,scope_key,scope,state,ruleset,observation_id,observed_at,confirmed_at,execution_egress_region,data
FROM collection_current
UNION ALL
SELECT generation,source_id,capability,output_schema,scope_key,scope,state,ruleset,observation_id,observed_at,confirmed_at,execution_egress_region,data
FROM steam_fact_reset_inputs;

CREATE TABLE steam_game_availability_evidence (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 binding_id uuid NOT NULL UNIQUE REFERENCES collection_bindings(id),
 observed_at timestamptz NOT NULL,
 observation_id uuid NOT NULL CHECK (uuid_extract_version(observation_id)=7)
);

CREATE TABLE steam_first_available_reviews (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 revision bigint NOT NULL CHECK (revision>0),
 window_start date NOT NULL,
 window_end date NOT NULL CHECK (window_end>=window_start),
 evidence_url text NOT NULL CHECK (length(evidence_url) BETWEEN 1 AND 512),
 reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 512),
 actor_id uuid NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE (binding_id,revision)
);

CREATE TABLE steam_change_registry (
 detector text NOT NULL,
 version integer NOT NULL CHECK (version>0),
 source_contract text NOT NULL,
 policy text NOT NULL,
 codes text[] NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 PRIMARY KEY (detector,version)
);
INSERT INTO steam_change_registry(detector,version,source_contract,policy,codes) VALUES
 ('free_game_transition',1,'steam.details/3','adjacent_utc_day',ARRAY['game_became_free','game_became_paid']),
 ('windows_support_transition',1,'steam.details/3','adjacent_utc_day',ARRAY['windows_support_added','windows_support_removed']),
 ('mac_support_transition',1,'steam.details/3','adjacent_utc_day',ARRAY['mac_support_added','mac_support_removed']),
 ('linux_support_transition',1,'steam.details/3','adjacent_utc_day',ARRAY['linux_support_added','linux_support_removed']),
 ('game_release_transition',1,'steam.details/3','adjacent_utc_day',ARRAY['game_became_available','game_availability_withdrawn','game_release_plan_changed']),
 ('game_price_transition',1,'steam.price_daily/1','adjacent_utc_day',ARRAY['game_price_state_changed','game_price_currency_changed','game_price_decreased','game_price_increased','game_discount_started','game_discount_ended','game_discount_changed']);

CREATE TABLE steam_change_days (
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 day date NOT NULL,
 failures integer NOT NULL DEFAULT 0,
 retry_after timestamptz,
 PRIMARY KEY (binding_id,day)
);
CREATE INDEX steam_change_due ON steam_change_days (day,binding_id);
CREATE TABLE steam_change_checkpoints (
 detector text NOT NULL,
 version integer NOT NULL,
 source_start date NOT NULL,
 processed_through date NOT NULL,
 upstream_through date NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 PRIMARY KEY (detector,version),
 FOREIGN KEY (detector,version) REFERENCES steam_change_registry(detector,version),
 CHECK (source_start<=processed_through AND processed_through<=upstream_through)
);
CREATE TABLE steam_game_changes (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 day date NOT NULL,
 detector text NOT NULL,
 version integer NOT NULL,
 code text NOT NULL,
 market text NOT NULL,
 locale text NOT NULL,
 egress text NOT NULL,
 before_value jsonb NOT NULL,
 after_value jsonb NOT NULL,
 observation_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 materialized_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 FOREIGN KEY (detector,version) REFERENCES steam_change_registry(detector,version),
 UNIQUE (binding_id,day,detector,version,code,market,locale,egress)
);
CREATE INDEX steam_change_page ON steam_game_changes (binding_id,day DESC,id DESC);
CREATE INDEX steam_change_retention ON steam_game_changes (day,id);
CREATE INDEX steam_fact_retention ON steam_game_daily_facts (day,id);

-- +goose StatementBegin
CREATE FUNCTION queue_steam_change_day() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d date;
BEGIN
 IF TG_TABLE_NAME='game_price_observations' THEN
  d := NEW.utc_day;
 ELSE
  d := NEW.day;
 END IF;
 IF d >= (transaction_timestamp() AT TIME ZONE 'UTC')::date-400 THEN
  INSERT INTO steam_change_days(binding_id,day) VALUES (NEW.binding_id,d),(NEW.binding_id,d+1)
  ON CONFLICT (binding_id,day) DO UPDATE SET day=excluded.day;
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER queue_steam_facts AFTER INSERT OR UPDATE OF value,observation_id ON steam_game_daily_facts
 FOR EACH ROW EXECUTE FUNCTION queue_steam_change_day();
CREATE TRIGGER queue_steam_price AFTER INSERT ON game_price_observations FOR EACH ROW EXECUTE FUNCTION queue_steam_change_day();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION save_game_price_observations(gen uuid, src_filter uuid, scope_filter text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE point record;
BEGIN
 FOR point IN
  SELECT DISTINCT ON (o.game_source_id,c.scope_key,c.observation_id,c.execution_egress_region)
   o.game_source_id,o.binding_id,c.observation_id,c.output_schema,
   CASE WHEN c.output_schema=3 THEN c.execution_egress_region ELSE c.scope->>'egress_region' END AS egress,
   c.scope->>'region' AS country,c.scope->>'locale' AS locale,
   coalesce(c.data#>>'{amounts,currency}',CASE c.scope->>'region' WHEN 'cn' THEN 'CNY' WHEN 'us' THEN 'USD' WHEN 'hk' THEN 'HKD' END) AS currency,
   (c.observed_at AT TIME ZONE 'UTC')::date AS utc_day,c.observed_at,c.confirmed_at,c.data->>'state' AS state,
   c.data#>>'{amounts,initial}' AS original,c.data#>>'{amounts,final}' AS final,c.data#>>'{amounts,discount_percent}' AS discount
  FROM steam_fact_inputs c CROSS JOIN LATERAL (
   SELECT s.id AS game_source_id,b.id AS binding_id,p.scope,src.namespace,src.external_key AS app_id
   FROM collection_sources src
   JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam'
   JOIN games g ON g.id=b.game_id
   JOIN game_sources s ON s.id=b.game_source_id AND s.game_id=b.game_id AND s.platform_code='steam' AND s.external_key=src.external_key
   JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
   JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=c.capability AND p.output_schema=c.output_schema
   WHERE src.id=c.source_id AND src.target_id IS NOT NULL AND src.provider='steam' AND src.resource_type='app'
    AND (c.output_schema=1 OR g.status<>'archived')
    AND c.observed_at>=greatest(b.effective_from,m.attached_at)
    AND (b.effective_until IS NULL OR c.observed_at<b.effective_until)
  ) o
  WHERE c.generation=gen AND (src_filter IS NULL OR c.source_id=src_filter) AND (scope_filter='' OR c.scope_key=scope_filter)
   AND c.capability='steam.details' AND c.output_schema IN (1,3) AND c.state='present'
   AND c.observation_id IS NOT NULL AND c.observed_at<=c.confirmed_at
   AND (c.output_schema=1 OR c.execution_egress_region<>'')
   AND c.scope->'dimensions'='{"section":"price"}' AND c.scope->>'namespace'=o.namespace
   AND c.scope-'namespace'-'dimensions'=o.scope
   AND c.scope->>'region' IN ('cn','us','hk') AND c.scope->>'locale' IN ('en','zh-cn')
   AND c.data->>'app_id'=o.app_id AND c.data->>'tier'='observed_storefront'
   AND c.data->>'source_url'='https://store.steampowered.com/api/appdetails'
   AND (c.data->>'state'='free' OR (c.data->>'state' IN ('priced','zero_price')
     AND (c.output_schema=1 AND (c.scope->>'region',c.data#>>'{amounts,currency}') IN (('cn','CNY'),('us','USD'),('hk','HKD'))
       OR c.output_schema=3 AND c.data#>>'{amounts,currency}' ~ '^[A-Z]{3}$')
     AND c.data#>>'{amounts,initial}' ~ '^[0-9]{1,15}$' AND c.data#>>'{amounts,final}' ~ '^[0-9]{1,15}$'
     AND c.data#>>'{amounts,discount_percent}' ~ '^[0-9]{1,3}$'))
  ORDER BY o.game_source_id,c.scope_key,c.observation_id,c.execution_egress_region,o.binding_id
 LOOP
  INSERT INTO game_price_observations (game_source_id,binding_id,observation_id,egress_region,store_country,currency,locale,utc_day,observed_at,state,original,final,discount,confirmed_at,output_schema)
  VALUES (point.game_source_id,point.binding_id,point.observation_id,point.egress,point.country,point.currency,point.locale,
   point.utc_day,point.observed_at,point.state,point.original::bigint,point.final::bigint,point.discount::integer,point.confirmed_at,point.output_schema)
  ON CONFLICT DO NOTHING;
  IF FOUND THEN
   -- Retain at most the latest 96 genuine samples per UTC day and store scope.
   DELETE FROM game_price_observations p WHERE p.game_source_id=point.game_source_id AND p.egress_region=point.egress
    AND p.store_country=point.country AND p.currency=point.currency AND p.locale=point.locale AND p.utc_day=point.utc_day
    AND (p.observed_at,p.observation_id) IN (
     SELECT old.observed_at,old.observation_id FROM game_price_observations old
     WHERE old.game_source_id=p.game_source_id AND old.egress_region=p.egress_region AND old.store_country=p.store_country
      AND old.currency=p.currency AND old.locale=p.locale AND old.utc_day=p.utc_day
     ORDER BY old.observed_at DESC,old.observation_id DESC OFFSET 96);
  END IF;
 END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'R4 facts require a forward repair or a verified restore'; END $$;
