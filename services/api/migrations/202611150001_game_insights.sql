-- +goose Up
CREATE TABLE game_player_daily (
 game_source_id uuid NOT NULL,
 egress_region text NOT NULL,
 utc_day date NOT NULL,
 samples jsonb NOT NULL CHECK (jsonb_typeof(samples)='object' AND octet_length(samples::text)<=8192),
 PRIMARY KEY (game_source_id,egress_region,utc_day)
);

CREATE TABLE game_price_observations (
 game_source_id uuid NOT NULL,
 binding_id uuid NOT NULL,
 observation_id uuid NOT NULL,
 egress_region text NOT NULL,
 store_country text NOT NULL,
 currency text NOT NULL,
 locale text NOT NULL CHECK (locale IN ('en','zh-cn')),
 utc_day date NOT NULL,
 observed_at timestamptz NOT NULL,
 state text NOT NULL,
 original bigint,
 final bigint,
 discount integer,
 PRIMARY KEY (game_source_id,egress_region,store_country,currency,locale,utc_day,observed_at,observation_id),
 CHECK ((store_country,currency) IN (('cn','CNY'),('us','USD'),('hk','HKD'))),
 CHECK ((state='free' AND original IS NULL AND final IS NULL AND discount IS NULL)
     OR (state IN ('priced','zero_price') AND original IS NOT NULL AND final IS NOT NULL AND discount IS NOT NULL
        AND original>=final AND final>=0 AND discount BETWEEN 0 AND 100 AND (state='zero_price')=(final=0)))
);

CREATE INDEX game_collection_members_reverse ON game_collection_members (game_id,collection_id);
CREATE INDEX game_channel_genres_lookup ON game_channel_steam USING gin (genres) WHERE section='details';

-- +goose StatementBegin
CREATE FUNCTION game_insight_owners(src_id uuid, cap text, evidence_at timestamptz)
RETURNS TABLE (game_source_id uuid, binding_id uuid, scope jsonb, namespace text, app_id text)
LANGUAGE sql STABLE AS $$
 SELECT s.id,b.id,p.scope,src.namespace,src.external_key
 FROM collection_sources src
 JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam'
 JOIN game_sources s ON s.id=b.game_source_id AND s.game_id=b.game_id AND s.platform_code='steam' AND s.external_key=src.external_key
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=cap AND p.request_schema=1
 WHERE src.id=src_id AND src.target_id IS NOT NULL AND src.provider='steam' AND src.resource_type='app'
   AND evidence_at>=greatest(b.effective_from,m.attached_at) AND (b.effective_until IS NULL OR evidence_at<b.effective_until)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION game_insight_scope_hash(ns text, region text, section text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT AS $$
 SELECT encode(sha256(convert_to('{"namespace":'||to_json(ns)::text||',"egress_region":'||to_json(region)::text||
  ',"dimensions":{"section":'||to_json(section)::text||'}}','UTF8')),'hex')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION keep_game_player_sample(sample game_evidence_players) RETURNS void LANGUAGE sql AS $$
 INSERT INTO game_player_daily AS day (game_source_id,egress_region,utc_day,samples)
 SELECT DISTINCT ON (o.game_source_id,o.scope->>'egress_region') o.game_source_id,o.scope->>'egress_region',
  (sample.bucket AT TIME ZONE 'UTC')::date,
  jsonb_build_object(
   (extract(hour FROM sample.bucket AT TIME ZONE 'UTC')::int*2+floor(extract(minute FROM sample.bucket AT TIME ZONE 'UTC')/30)::int)::text,
   jsonb_build_array(sample.players,sample.observed_at,sample.observation_id,o.binding_id))
 FROM game_insight_owners(sample.source_id,'steam.players',sample.observed_at) o
 WHERE sample.scope_key=game_insight_scope_hash(o.namespace,o.scope->>'egress_region','players')
 ORDER BY o.game_source_id,o.scope->>'egress_region',o.binding_id
 ON CONFLICT (game_source_id,egress_region,utc_day) DO UPDATE SET samples=day.samples||excluded.samples
 WHERE NOT EXISTS (SELECT FROM jsonb_each(excluded.samples) point
  WHERE (day.samples->point.key->>1)::timestamptz >= (point.value->>1)::timestamptz)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION track_game_player_day() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM keep_game_player_sample(NEW);
 RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER track_game_player_day AFTER INSERT OR UPDATE ON game_evidence_players
 FOR EACH ROW EXECUTE FUNCTION track_game_player_day();

-- +goose StatementBegin
CREATE FUNCTION save_game_price_observations(gen uuid, src_filter uuid, scope_filter text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE point record;
BEGIN
 FOR point IN
  SELECT DISTINCT ON (o.game_source_id,c.scope_key,c.observation_id)
   o.game_source_id,o.binding_id,c.observation_id,c.scope->>'egress_region' AS egress,
   c.scope->>'region' AS country,c.scope->>'locale' AS locale,
   CASE c.scope->>'region' WHEN 'cn' THEN 'CNY' WHEN 'us' THEN 'USD' WHEN 'hk' THEN 'HKD' END AS currency,
   (c.observed_at AT TIME ZONE 'UTC')::date AS utc_day,c.observed_at,c.data->>'state' AS state,
   c.data#>>'{amounts,initial}' AS original,c.data#>>'{amounts,final}' AS final,c.data#>>'{amounts,discount_percent}' AS discount
  FROM collection_current c CROSS JOIN LATERAL game_insight_owners(c.source_id,c.capability,c.observed_at) o
  WHERE c.generation=gen AND (src_filter IS NULL OR c.source_id=src_filter) AND (scope_filter='' OR c.scope_key=scope_filter)
   AND c.capability='steam.details' AND c.output_schema=1 AND c.state='present'
   AND c.observation_id IS NOT NULL AND c.observed_at<=c.confirmed_at
   AND c.scope->'dimensions'='{"section":"price"}' AND c.scope->>'namespace'=o.namespace
   AND c.scope-'namespace'-'dimensions'=o.scope AND c.scope->>'region' IN ('cn','us','hk') AND c.scope->>'locale' IN ('en','zh-cn')
   AND c.data->>'app_id'=o.app_id AND c.data->>'tier'='observed_storefront'
   AND c.data->>'source_url'='https://store.steampowered.com/api/appdetails'
   AND (c.data->>'state'='free' OR (c.data->>'state' IN ('priced','zero_price')
     AND (c.scope->>'region',c.data#>>'{amounts,currency}') IN (('cn','CNY'),('us','USD'),('hk','HKD'))
     AND c.data#>>'{amounts,initial}' ~ '^[0-9]{1,15}$' AND c.data#>>'{amounts,final}' ~ '^[0-9]{1,15}$'
     AND c.data#>>'{amounts,discount_percent}' ~ '^[0-9]{1,3}$'))
  ORDER BY o.game_source_id,c.scope_key,c.observation_id,o.binding_id
 LOOP
  INSERT INTO game_price_observations
  VALUES (point.game_source_id,point.binding_id,point.observation_id,point.egress,point.country,point.currency,point.locale,
   point.utc_day,point.observed_at,point.state,point.original::bigint,point.final::bigint,point.discount::integer)
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

SELECT count(keep_game_player_sample(e)) FROM game_evidence_players e;
SELECT count(save_game_price_observations(generation,NULL,'')) FROM collection_consumer WHERE generation IS NOT NULL;

-- +goose Down
DROP TRIGGER track_game_player_day ON game_evidence_players;
DROP FUNCTION track_game_player_day();
DROP FUNCTION keep_game_player_sample(game_evidence_players);
DROP FUNCTION save_game_price_observations(uuid,uuid,text);
DROP FUNCTION game_insight_scope_hash(text,text,text);
DROP FUNCTION game_insight_owners(uuid,text,timestamptz);
DROP INDEX game_channel_genres_lookup;
DROP INDEX game_collection_members_reverse;
DROP TABLE game_price_observations;
DROP TABLE game_player_daily;
