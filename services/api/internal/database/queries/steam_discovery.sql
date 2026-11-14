-- name: InitSteamDiscovery :exec
INSERT INTO steam_discovery_state(namespace) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: GetSteamDiscovery :one
SELECT * FROM steam_discovery_state WHERE namespace=$1;

-- name: LockSteamDiscovery :one
SELECT pg_try_advisory_lock(7748032881221203::bigint)::boolean;

-- name: UnlockSteamDiscovery :exec
SELECT pg_advisory_unlock(7748032881221203::bigint);

-- name: ConfigureSteamDiscovery :exec
UPDATE steam_discovery_state SET paused=sqlc.arg(paused),
 backfill=CASE WHEN sqlc.arg(start_backfill)::boolean AND window_started=0 THEN true ELSE backfill END,
 next_at=now() WHERE namespace=$1;

-- name: OpenSteamDiscoveryWindow :exec
UPDATE steam_discovery_state SET window_started=sqlc.arg(window_started),modified_since=sqlc.arg(modified_since),
 last_appid=0,egress_region=sqlc.arg(egress_region),page_limit=sqlc.arg(page_limit),last_code=''
WHERE namespace=$1 AND window_started=0;

-- name: SaveSteamDiscoveryTarget :exec
UPDATE steam_discovery_state SET target_id=$2 WHERE namespace=$1;

-- name: SaveSteamDiscoveryIntent :exec
UPDATE steam_discovery_state SET intent_id=$2,intent_at=now(),writer_binding=$3,
 jobs_today=CASE WHEN budget_day=$4 THEN jobs_today+1 ELSE 1 END,
 selected_today=CASE WHEN budget_day=$4 THEN selected_today ELSE 0 END,
 explored_today=CASE WHEN budget_day=$4 THEN explored_today ELSE 0 END,
 budget_day=$4
WHERE namespace=$1;

-- name: SaveSteamDiscoveryJob :exec
UPDATE steam_discovery_state SET job_id=$2,last_code='' WHERE namespace=$1;

-- name: RetrySteamDiscoveryPage :exec
UPDATE steam_discovery_state SET job_id=NULL,intent_id=NULL,intent_at=NULL,writer_binding='',
 next_at=now()+make_interval(secs=>sqlc.arg(delay)::integer),last_code=sqlc.arg(code) WHERE namespace=$1;

-- name: CommitSteamDiscoveryPage :exec
UPDATE steam_discovery_state SET
 pending=sqlc.arg(pending),last_appid=sqlc.arg(last_appid),
 watermark=CASE WHEN sqlc.arg(complete)::boolean THEN greatest(watermark,window_started) ELSE watermark END,
 window_started=CASE WHEN sqlc.arg(complete) THEN 0 ELSE window_started END,
 backfill=backfill AND NOT sqlc.arg(complete),windows=windows+CASE WHEN sqlc.arg(complete) THEN 1 ELSE 0 END,
 next_at=now()+make_interval(secs=>sqlc.arg(delay)::integer),
 intent_id=NULL,intent_at=NULL,job_id=NULL,writer_binding='',last_code='',
 selected_today=CASE WHEN budget_day=sqlc.arg(day) THEN selected_today ELSE 0 END+sqlc.arg(selected)::int,
 explored_today=CASE WHEN budget_day=sqlc.arg(day) THEN explored_today ELSE 0 END+sqlc.arg(explored)::int,
 jobs_today=CASE WHEN budget_day=sqlc.arg(day) THEN jobs_today ELSE 0 END,budget_day=sqlc.arg(day),
 pages=pages+1,rows_seen=rows_seen+sqlc.arg(rows_seen)::integer
WHERE namespace=$1;

-- name: SaveSteamDiscoveryQueue :exec
UPDATE steam_discovery_state SET pending=$2,admitted=admitted+sqlc.arg(admitted)::integer,
 admitted_today=CASE WHEN admission_day=sqlc.arg(day) THEN admitted_today ELSE 0 END+sqlc.arg(admitted)::integer,
 admission_day=sqlc.arg(day)
WHERE namespace=$1;

-- name: SteamDiscoveryAdmissions :one
SELECT (CASE WHEN admission_day=sqlc.arg(day)::bigint THEN admitted_today ELSE 0 END)::integer
FROM steam_discovery_state WHERE namespace=$1;

-- name: SteamDiscoveryKnown :many
SELECT keys.external_key::text AS external_key,(s.id IS NOT NULL OR coalesce(c.review<>'{}' OR c.discovery_modified>0,false))::boolean AS known,
 coalesce(c.discovery_modified,0)::bigint AS modified,coalesce(c.state,'')::text AS state,
 coalesce(c.reason,'')::text AS reason,(c.game_id IS NOT NULL)::boolean AS bound
FROM unnest(sqlc.arg(keys)::text[]) keys(external_key)
LEFT JOIN game_sources s ON s.platform_code='steam' AND s.external_key=keys.external_key AND s.active
LEFT JOIN game_intake_candidates c ON c.platform_code='steam' AND c.external_key=keys.external_key;

-- name: NoteSteamDiscoveryCandidate :exec
UPDATE game_intake_candidates SET discovery_modified=$2 WHERE id=$1 AND discovery_modified<$2;

-- name: ReopenSteamDiscoveryCandidate :execrows
UPDATE game_intake_candidates SET discovery_modified=$2,state='discovered',reason='',next_at=now(),
 job_id=NULL,job_started_at=NULL,job_writer_binding='',job_round=1,job_generation=job_generation+1,
 attempt_id=NULL,observation_id=NULL,observation_hash='',failures=0,updated_at=now()
WHERE id=$1 AND discovery_modified>0 AND discovery_modified<$2 AND game_id IS NULL
 AND state IN ('deferred','rejected') AND reason IN ('eligibility_uncertain','irrelevant_game','adult_game','job_failed','observation_unattributed');

-- name: PutSteamEligibility :exec
UPDATE game_intake_candidates SET automation=$2 WHERE id=$1;

-- name: WakeReviewedEligibility :exec
UPDATE game_intake_candidates SET state='discovered',next_at=now(),failures=0,updated_at=now()
WHERE id=$1 AND game_id IS NULL AND reason IN ('eligibility_uncertain','adult_game','irrelevant_game')
 AND state IN ('deferred','rejected');

-- name: ClaimSteamAIBudget :execrows
INSERT INTO steam_eligibility_ai_budget(utc_day,calls) VALUES ((statement_timestamp() AT TIME ZONE 'UTC')::date,1)
ON CONFLICT (utc_day) DO UPDATE SET calls=steam_eligibility_ai_budget.calls+1
WHERE steam_eligibility_ai_budget.calls<sqlc.arg(daily_limit)::integer;

-- name: PruneSteamAIBudget :exec
DELETE FROM steam_eligibility_ai_budget WHERE utc_day<(statement_timestamp() AT TIME ZONE 'UTC')::date-7;
