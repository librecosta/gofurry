-- name: SteamCanonicalInputs :many
SELECT DISTINCT ON (b.id,c.observation_id,c.execution_egress_region)
 b.id AS binding_id,c.observation_id,c.observed_at,c.confirmed_at,
 (c.scope->>'locale')::text AS locale,(c.scope->>'region')::text AS market,c.execution_egress_region AS egress,
 jsonb_strip_nulls(jsonb_build_object('release',c.data->'release','platforms',c.data->'platforms',
 'is_free',c.data->'is_free','supported_languages',c.data->'supported_languages','languages',c.data->'languages',
 'invalid',c.data->'invalid','truncated',c.data->'truncated'))::jsonb AS payload
FROM steam_fact_inputs c JOIN collection_sources src ON src.id=c.source_id
JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam'
JOIN game_sources gs ON gs.id=b.game_source_id AND gs.game_id=b.game_id AND gs.platform_code='steam' AND gs.external_key=src.external_key
JOIN games g ON g.id=b.game_id AND g.status<>'archived'
JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=c.capability
 AND p.output_schema=c.output_schema AND p.scope=c.scope-'namespace'-'dimensions'
WHERE c.generation=sqlc.arg(generation)::uuid AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id))
 AND (sqlc.arg(scope_key)::text='' OR c.scope_key=sqlc.arg(scope_key))
 AND src.namespace=c.scope->>'namespace' AND src.provider='steam' AND src.target_id IS NOT NULL
 AND c.capability='steam.details' AND c.output_schema=3 AND c.scope->'dimensions'='{"section":"rich_details"}'
 AND c.ruleset='gofurry-steam-rich-3' AND c.state='present' AND c.observation_id IS NOT NULL
 AND c.observed_at<=transaction_timestamp()
 AND c.observed_at>=greatest(b.effective_from,m.attached_at) AND c.observed_at<=c.confirmed_at
 AND (b.effective_until IS NULL OR c.observed_at<b.effective_until)
 AND c.scope->>'locale' IN ('en','zh-cn') AND c.scope->>'region' IN ('us','cn','hk')
 AND c.execution_egress_region<>'' AND c.data->>'app_id'=gs.external_key
 AND (b.id,c.observation_id,c.execution_egress_region)>(sqlc.arg(after_binding)::uuid,sqlc.arg(after_observation)::uuid,sqlc.arg(after_egress)::text)
ORDER BY b.id,c.observation_id,c.execution_egress_region,c.confirmed_at DESC LIMIT 64;

-- name: StageSteamFactInput :exec
INSERT INTO steam_fact_reset_inputs (generation,source_id,capability,output_schema,scope_key,scope,state,ruleset,observation_id,observed_at,confirmed_at,execution_egress_region,data)
SELECT c.generation,c.source_id,c.capability,c.output_schema,c.scope_key,c.scope,c.state,c.ruleset,c.observation_id,c.observed_at,c.confirmed_at,c.execution_egress_region,
 CASE WHEN c.scope#>>'{dimensions,section}'='rich_details' THEN
  jsonb_strip_nulls(jsonb_build_object('app_id',c.data->'app_id','release',c.data->'release','platforms',c.data->'platforms',
    'is_free',c.data->'is_free','supported_languages',c.data->'supported_languages','languages',c.data->'languages'))
 ELSE c.data END
FROM collection_current c
WHERE c.generation=sqlc.arg(generation)::uuid AND c.source_id=sqlc.arg(source_id)::uuid
 AND c.output_schema=sqlc.arg(output_schema)::int AND c.scope_key=sqlc.arg(scope_key)::text
 AND c.execution_egress_region=sqlc.arg(egress)::text AND c.observation_id=sqlc.arg(observation_id)::uuid
 AND c.state='present' AND c.confirmed_at IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM collection_consumer a WHERE a.generation=c.generation)
ON CONFLICT (generation,source_id,scope_key,output_schema,execution_egress_region,observation_id)
DO UPDATE SET confirmed_at=greatest(steam_fact_reset_inputs.confirmed_at,excluded.confirmed_at);

-- name: ClearSteamFactResetInputs :exec
DELETE FROM steam_fact_reset_inputs WHERE generation=$1;

-- name: PutSteamDailyFact :exec
INSERT INTO steam_game_daily_facts AS old (binding_id,day,locale,market,egress,kind,value,observation_id,observed_at,confirmed_at,normalizer)
SELECT sqlc.arg(binding_id),(sqlc.arg(observed_at)::timestamptz AT TIME ZONE 'UTC')::date,sqlc.arg(locale),
 sqlc.arg(market),sqlc.arg(egress),sqlc.arg(kind),sqlc.arg(value),sqlc.arg(observation_id),sqlc.arg(observed_at),sqlc.arg(confirmed_at),1
WHERE sqlc.arg(observed_at)>=(transaction_timestamp() AT TIME ZONE 'UTC')::date-400
 AND sqlc.arg(observed_at)<=transaction_timestamp()
ON CONFLICT (binding_id,locale,market,egress,kind,day) DO UPDATE SET
 value=excluded.value,observation_id=excluded.observation_id,observed_at=excluded.observed_at,confirmed_at=excluded.confirmed_at
WHERE (excluded.observed_at,excluded.observation_id,excluded.confirmed_at)>(old.observed_at,old.observation_id,old.confirmed_at);

-- name: PutSteamFirstObserved :exec
INSERT INTO steam_game_availability_evidence AS old(binding_id,observed_at,observation_id) VALUES ($1,$2,$3)
ON CONFLICT (binding_id) DO UPDATE SET observed_at=excluded.observed_at,observation_id=excluded.observation_id
WHERE (excluded.observed_at,excluded.observation_id)<(old.observed_at,old.observation_id);

-- name: SteamDetectorRegistry :many
SELECT * FROM steam_change_registry ORDER BY detector,version;

-- name: NextSteamChangeDay :one
SELECT * FROM steam_change_days WHERE day<(transaction_timestamp() AT TIME ZONE 'UTC')::date
 AND (retry_after IS NULL OR retry_after<=transaction_timestamp())
ORDER BY day,binding_id LIMIT 1 FOR UPDATE;

-- name: LockSteamRebuildDay :exec
INSERT INTO steam_change_days (binding_id,day) VALUES ($1,$2)
ON CONFLICT (binding_id,day) DO UPDATE SET day=excluded.day;

-- name: ClearSteamChangeDay :exec
DELETE FROM steam_change_days WHERE binding_id=$1 AND day=$2;

-- name: SteamDailyPairs :many
SELECT a.kind,a.locale,a.market,a.egress,b.value AS before_value,a.value AS after_value,a.observation_id,a.observed_at
FROM steam_game_daily_facts a JOIN steam_game_daily_facts b ON a.binding_id=b.binding_id
 AND a.locale=b.locale AND a.market=b.market AND a.egress=b.egress AND a.kind=b.kind AND b.day=a.day-1
WHERE a.binding_id=sqlc.arg(binding_id)::uuid AND a.day=sqlc.arg(day)::date
 AND a.kind<>'languages' AND a.locale='en' AND a.market='us'
ORDER BY a.kind,a.locale,a.market,a.egress;

-- name: SteamPriceDailyPairInputs :many
WITH scope AS (
 SELECT DISTINCT ON (p.utc_day,p.store_country,p.locale,p.egress_region) p.*,
 count(*) OVER (PARTITION BY p.utc_day,p.store_country,p.locale,p.egress_region)::integer AS samples
 FROM game_price_observations p
 WHERE p.binding_id=sqlc.arg(binding_id)::uuid AND p.utc_day IN (sqlc.arg(day)::date,sqlc.arg(day)::date-1)
  AND p.observed_at<=transaction_timestamp()
 ORDER BY p.utc_day,p.store_country,p.locale,p.egress_region,p.observed_at DESC,p.observation_id DESC
)
SELECT utc_day,store_country,locale,egress_region,currency,state,original,final,discount,samples,observation_id,observed_at,confirmed_at
FROM scope ORDER BY utc_day,store_country,locale,egress_region;

-- name: PutSteamChange :one
INSERT INTO steam_game_changes AS old(binding_id,day,detector,version,code,market,locale,egress,before_value,after_value,observation_id,observed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (binding_id,day,detector,version,code,market,locale,egress) DO UPDATE SET
 before_value=excluded.before_value,after_value=excluded.after_value,observation_id=excluded.observation_id,observed_at=excluded.observed_at,
 materialized_at=CASE WHEN (old.before_value,old.after_value,old.observation_id,old.observed_at)
 IS DISTINCT FROM (excluded.before_value,excluded.after_value,excluded.observation_id,excluded.observed_at) THEN transaction_timestamp() ELSE old.materialized_at END
RETURNING id;

-- name: RemoveSupersededSteamChanges :exec
DELETE FROM steam_game_changes WHERE binding_id=sqlc.arg(binding_id)::uuid AND day=sqlc.arg(day)::date
 AND detector=ANY(sqlc.arg(detectors)::text[]) AND NOT(id=ANY(sqlc.arg(kept)::uuid[]));

-- name: AdvanceSteamCheckpoint :exec
INSERT INTO steam_change_checkpoints AS old(detector,version,source_start,processed_through,upstream_through)
VALUES ($1,$2,$3,$3,(transaction_timestamp() AT TIME ZONE 'UTC')::date-1)
ON CONFLICT (detector,version) DO UPDATE SET
 source_start=least(old.source_start,excluded.source_start),processed_through=greatest(old.processed_through,excluded.processed_through),
 upstream_through=greatest(old.upstream_through,excluded.upstream_through),updated_at=transaction_timestamp();

-- name: PruneSteamFactHistory :exec
WITH retired_inputs AS (
 DELETE FROM steam_fact_reset_inputs WHERE ctid IN (
  SELECT ctid FROM steam_fact_reset_inputs WHERE recorded_at<transaction_timestamp()-interval '2 days'
  ORDER BY recorded_at LIMIT 256
 )
), changes AS (
 DELETE FROM steam_game_changes WHERE id IN (SELECT id FROM steam_game_changes
 WHERE day<(transaction_timestamp() AT TIME ZONE 'UTC')::date-399 ORDER BY day,id LIMIT 256)
)
DELETE FROM steam_game_daily_facts WHERE id IN (SELECT x.id FROM steam_game_daily_facts x
 WHERE x.day<(transaction_timestamp() AT TIME ZONE 'UTC')::date-400
 AND EXISTS (SELECT 1 FROM steam_game_daily_facts n WHERE n.binding_id=x.binding_id AND n.locale=x.locale AND n.market=x.market
 AND n.kind=x.kind AND n.egress=x.egress AND n.day>x.day)
 ORDER BY x.day,x.id LIMIT 256);

-- name: SteamCheckpointStatus :many
SELECT r.detector,r.version,r.source_contract,r.policy,r.codes,r.enabled,
 c.source_start,c.processed_through,c.upstream_through,c.updated_at,
 coalesce((SELECT min(day)::text FROM steam_change_days WHERE day<(transaction_timestamp() AT TIME ZONE 'UTC')::date),'')::text AS oldest_pending
FROM steam_change_registry r LEFT JOIN steam_change_checkpoints c USING(detector,version)
ORDER BY r.detector,r.version;

-- name: DelaySteamChangeDay :exec
UPDATE steam_change_days SET failures=failures+1,retry_after=transaction_timestamp()+interval '5 minutes'
WHERE binding_id=$1 AND day=$2;

-- name: SteamPendingStatus :one
SELECT count(*) FILTER(WHERE day<(transaction_timestamp() AT TIME ZONE 'UTC')::date)::bigint AS pending,
 count(*) FILTER(WHERE failures>0)::bigint AS failed,max(retry_after)::timestamptz AS retry_after
FROM steam_change_days;

-- name: SteamManagementIdentity :one
SELECT id FROM games WHERE public_id=$1;

-- name: CurrentSteamFactBinding :one
SELECT b.id,s.external_key FROM game_sources s JOIN collection_bindings b ON b.game_source_id=s.id
 JOIN games g ON g.id=s.game_id AND b.game_id=g.id
 JOIN collection_sources src ON src.id=b.source_id AND src.provider='steam' AND src.external_key=s.external_key
WHERE g.public_id=$1 AND g.status<>'archived' AND s.platform_code='steam' AND s.active AND b.purpose='steam' AND b.effective_until IS NULL
 AND b.effective_from<=transaction_timestamp()
ORDER BY (s.id=g.intake_display_source) DESC NULLS LAST,b.effective_from DESC,b.id DESC LIMIT 1;

-- name: CurrentSteamDailyFacts :many
SELECT p.kind,p.value,p.locale,p.market,p.egress,p.day,p.observed_at,p.confirmed_at,p.observation_id,p.normalizer
FROM (VALUES ('release'),('languages'),('free'),('windows'),('mac'),('linux')) AS kinds(kind)
CROSS JOIN LATERAL (
 SELECT d.* FROM steam_game_daily_facts d WHERE d.binding_id=$1 AND d.market='us' AND d.locale='en' AND d.kind=kinds.kind
 ORDER BY d.day DESC,d.observed_at DESC,d.observation_id DESC LIMIT 1
) p ORDER BY p.kind;

-- name: RecentSteamPriceDaily :many
WITH latest AS (
 SELECT DISTINCT ON (p.store_country,p.egress_region,p.utc_day)
 p.utc_day,p.store_country,p.locale,p.egress_region,p.currency,p.state,p.original,p.final,p.discount,p.observation_id,p.observed_at,p.confirmed_at,
 count(*) OVER (PARTITION BY p.store_country,p.egress_region,p.utc_day,p.locale)::integer AS samples
 FROM game_price_observations p
 WHERE p.binding_id=sqlc.arg(binding_id)::uuid AND (sqlc.arg(market)::text='' OR p.store_country=sqlc.arg(market))
  AND p.egress_region=sqlc.arg(egress)::text AND p.observed_at<=transaction_timestamp()
  AND p.utc_day>=sqlc.arg(from_day)::date
 ORDER BY p.store_country,p.egress_region,p.utc_day,(p.locale=sqlc.arg(locale)::text) DESC,p.observed_at DESC,p.observation_id DESC
)
SELECT * FROM latest ORDER BY utc_day DESC,store_country LIMIT sqlc.arg(take_rows)::integer;

-- name: CurrentSteamMarketPrices :many
SELECT p.utc_day,p.store_country,p.locale,p.egress_region,p.currency,p.state,p.original,p.final,p.discount,
 p.observation_id,p.observed_at,p.confirmed_at,(
  SELECT count(*)::int FROM game_price_observations d WHERE d.binding_id=p.binding_id AND d.egress_region=p.egress_region
   AND d.store_country=p.store_country AND d.utc_day=p.utc_day AND d.locale=p.locale AND d.observed_at<=transaction_timestamp()
 )::int AS samples
FROM (VALUES ('cn'),('us'),('hk')) AS markets(market)
CROSS JOIN LATERAL (
 SELECT p.* FROM game_price_observations p
 WHERE p.binding_id=sqlc.arg(binding_id)::uuid AND p.egress_region=sqlc.arg(egress)::text
  AND p.store_country=markets.market AND p.observed_at<=transaction_timestamp()
 ORDER BY p.utc_day DESC,(p.locale=sqlc.arg(locale)::text) DESC,p.observed_at DESC,p.observation_id DESC LIMIT 1
) p
WHERE sqlc.arg(market)::text='' OR markets.market=sqlc.arg(market)
ORDER BY p.store_country;

-- name: SteamCanonicalScopeWithdrawals :many
SELECT egress_region,max(event_at)::timestamptz AS event_at FROM game_source_media
WHERE binding_id=$1 AND capability='steam.details' AND output_schema=3 AND locale='en' AND store_country='us'
 AND ruleset='gofurry-steam-rich-3' AND state='withdrawn'
GROUP BY egress_region;

-- name: SteamFactsPlayerQuality :one
SELECT count(DISTINCT d.utc_day)::int AS days,count(*)::int AS samples,
  max((p.value->>1)::timestamptz)::timestamptz AS observed_at
FROM collection_bindings b JOIN game_player_daily d ON d.game_source_id=b.game_source_id
 CROSS JOIN LATERAL jsonb_each(d.samples) p
WHERE b.id=$1 AND d.egress_region=$2 AND p.value->>3=b.id::text
 AND d.utc_day BETWEEN (transaction_timestamp() AT TIME ZONE 'UTC')::date-29 AND (transaction_timestamp() AT TIME ZONE 'UTC')::date
 AND (p.value->>1)::timestamptz<=transaction_timestamp();

-- name: SteamFirstObservedAvailable :one
SELECT observed_at,observation_id FROM steam_game_availability_evidence WHERE binding_id=$1;

-- name: SteamFirstAvailableReview :one
SELECT * FROM steam_first_available_reviews WHERE binding_id=$1 ORDER BY revision DESC LIMIT 1;

-- name: RecordSteamFirstAvailable :exec
INSERT INTO steam_first_available_reviews(binding_id,revision,window_start,window_end,evidence_url,reason,actor_id)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: LockSteamGame :one
SELECT id,revision,status FROM games WHERE public_id=$1 FOR UPDATE;

-- name: ListSteamGameChanges :many
SELECT e.* FROM steam_game_changes e JOIN collection_bindings b ON b.id=e.binding_id
 JOIN games g ON g.id=b.game_id
WHERE g.public_id=sqlc.arg(public_id)::text
 AND (NOT sqlc.arg(published_only)::boolean OR (g.status='published' AND EXISTS (SELECT 1 FROM game_channel_public p WHERE p.id=g.id)))
 AND (sqlc.arg(market)::text='' OR e.market=sqlc.arg(market) OR sqlc.arg(published_only)::boolean AND e.detector<>'game_price_transition')
 AND (sqlc.arg(egress)::text='' OR e.egress=sqlc.arg(egress))
 AND (sqlc.arg(code)::text='' OR e.code=sqlc.arg(code))
 AND e.day BETWEEN sqlc.arg(from_day)::date AND sqlc.arg(through_day)::date
 AND (e.day,e.id)<(sqlc.arg(after_day)::date,sqlc.arg(after_id)::uuid)
ORDER BY e.day DESC,e.id DESC LIMIT 31;

-- name: SteamChangeWorkBounds :one
SELECT ((transaction_timestamp() AT TIME ZONE 'UTC')::date-399)::date AS first_day,
 ((transaction_timestamp() AT TIME ZONE 'UTC')::date-1)::date AS last_day;

-- name: CanRebuildSteamBinding :one
SELECT EXISTS (
 SELECT 1 FROM games g JOIN collection_bindings b ON b.game_id=g.id
 WHERE g.public_id=sqlc.arg(public_id) AND b.id=sqlc.arg(binding_id)
 AND b.purpose='steam' AND (b.effective_from AT TIME ZONE 'UTC')::date<=sqlc.arg(day)::date
 AND (b.effective_until IS NULL OR (b.effective_until AT TIME ZONE 'UTC')::date>=sqlc.arg(day)::date)
) AS allowed;
