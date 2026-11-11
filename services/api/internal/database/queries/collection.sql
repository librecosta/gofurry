-- name: CollectionSiteAddress :one
SELECT a.id,a.site_id,a.url,a.kind FROM site_addresses a JOIN sites s ON s.id=a.site_id
WHERE s.public_id=$1 AND s.status='published' AND a.active AND a.url=$2
FOR UPDATE OF s;

-- name: CollectionSteamSource :one
SELECT p.id,p.game_id,p.external_key FROM game_sources p JOIN games g ON g.id=p.game_id
WHERE g.public_id=$1 AND g.status='published' AND p.active AND p.platform_code='steam' AND p.external_key=$2
FOR UPDATE OF g;

-- name: SaveCollectionSource :one
INSERT INTO collection_sources (id,namespace,provider,resource_type,external_key) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (namespace,provider,resource_type,external_key) DO UPDATE SET external_key=excluded.external_key
RETURNING *;

-- name: AddCollectionSiteBinding :one
INSERT INTO collection_bindings (id,source_id,resource_type,site_id,address_id,purpose) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (source_id,address_id) WHERE effective_until IS NULL AND address_id IS NOT NULL DO UPDATE SET purpose=excluded.purpose
RETURNING *;

-- name: AddCollectionGameBinding :one
INSERT INTO collection_bindings (id,source_id,resource_type,game_id,game_source_id,purpose) VALUES ($1,$2,'app',$3,$4,'steam')
ON CONFLICT (source_id,game_source_id) WHERE effective_until IS NULL AND game_source_id IS NOT NULL DO UPDATE SET purpose=excluded.purpose
RETURNING *;

-- name: CloseCollectionBinding :execrows
UPDATE collection_bindings b SET effective_until=transaction_timestamp()
FROM collection_sources s WHERE b.id=$1 AND b.source_id=s.id AND s.namespace=$2 AND b.effective_until IS NULL;

-- name: CloseRemovedSiteBindings :exec
UPDATE collection_bindings b SET effective_until=transaction_timestamp()
WHERE b.site_id=$1 AND b.effective_until IS NULL AND NOT EXISTS (
 SELECT 1 FROM site_addresses a WHERE a.id=b.address_id AND a.active AND (b.resource_type='domain' OR a.kind=b.purpose)
);

-- name: CloseRemovedGameBindings :exec
UPDATE collection_bindings b SET effective_until=transaction_timestamp()
WHERE b.game_id=$1 AND b.effective_until IS NULL
AND NOT EXISTS (SELECT 1 FROM game_sources g WHERE g.id=b.game_source_id AND g.active);

-- name: OpenCollectionBinding :one
SELECT b.id,b.source_id,b.resource_type,s.external_key FROM collection_bindings b
JOIN collection_sources s ON s.id=b.source_id
WHERE b.id=$1 AND s.namespace=$2 AND b.effective_until IS NULL FOR UPDATE OF b;

-- name: SaveCollectionPlan :one
INSERT INTO collection_plans (id,source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy)
VALUES ($1,$2,$3,1,$4,$5,$6,$7,$8)
ON CONFLICT (source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy)
DO UPDATE SET checked_at=NULL RETURNING *;

-- name: AttachCollectionPlan :exec
INSERT INTO collection_plan_bindings (plan_id,binding_id,source_id) VALUES ($1,$2,$3)
ON CONFLICT (plan_id,binding_id) DO NOTHING;

-- name: PendingCollectionSources :many
SELECT s.* FROM collection_sources s WHERE s.namespace=$1 AND s.target_id IS NULL
AND EXISTS (SELECT 1 FROM collection_bindings b WHERE b.source_id=s.id AND b.effective_until IS NULL)
ORDER BY s.checked_at NULLS FIRST,s.id LIMIT 8;

-- name: TouchCollectionSource :exec
UPDATE collection_sources SET checked_at=transaction_timestamp() WHERE id=$1;

-- name: SaveCollectionTarget :exec
UPDATE collection_sources SET target_id=$2 WHERE id=$1 AND target_id IS NULL;

-- name: PendingCollectionPlans :many
SELECT p.*,s.target_id,s.namespace,s.provider,s.resource_type,s.external_key,
 EXISTS (SELECT 1 FROM collection_plan_bindings m JOIN collection_bindings b ON b.id=m.binding_id
         WHERE m.plan_id=p.id AND b.effective_until IS NULL) AS desired
FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id
WHERE s.namespace=$1 AND s.target_id IS NOT NULL AND
 (p.checked_at IS NULL OR p.checked_at < transaction_timestamp() - CASE WHEN p.schedule_id IS NULL THEN interval '30 seconds' ELSE interval '30 minutes' END OR
  (p.schedule_id IS NOT NULL AND p.remote_enabled IS DISTINCT FROM (EXISTS (SELECT 1 FROM collection_plan_bindings m JOIN collection_bindings b ON b.id=m.binding_id
         WHERE m.plan_id=p.id AND b.effective_until IS NULL))))
ORDER BY p.checked_at NULLS FIRST,p.id LIMIT 8;

-- name: TouchCollectionPlan :exec
UPDATE collection_plans SET checked_at=transaction_timestamp() WHERE id=$1;

-- name: MarkCollectionPost :execrows
UPDATE collection_plans SET post_started=true WHERE id=$1 AND NOT post_started AND schedule_id IS NULL;

-- name: ClearCollectionPost :exec
UPDATE collection_plans SET post_started=false WHERE id=$1 AND schedule_id IS NULL;

-- name: SaveCollectionSchedule :exec
UPDATE collection_plans SET schedule_id=$2,remote_revision=$3,remote_enabled=$4,checked_at=transaction_timestamp()
WHERE id=$1 AND (schedule_id IS NULL OR schedule_id=$2);

-- name: InitCollectionConsumer :exec
INSERT INTO collection_consumer (namespace,consumer_id) VALUES ($1,$2) ON CONFLICT DO NOTHING;

-- name: GetCollectionConsumer :one
SELECT * FROM collection_consumer WHERE namespace=$1;

-- name: SaveCollectionCursor :exec
UPDATE collection_consumer SET cursor=$2,ack_pending=true WHERE namespace=$1;

-- name: ActivateCollectionGeneration :exec
UPDATE collection_consumer SET generation=$2,epoch=$3,cursor=$4,reset_ack=$5,ack_pending=true,sweep_after='',sweep_at=transaction_timestamp()
WHERE namespace=$1;

-- name: MarkCollectionAck :exec
UPDATE collection_consumer SET ack_pending=false,reset_ack=NULL WHERE namespace=$1 AND cursor=$2;

-- name: SaveCollectionSweep :exec
UPDATE collection_consumer SET sweep_after=$2,sweep_at=$3 WHERE namespace=$1;

-- name: AddCollectionGap :exec
INSERT INTO collection_gaps (id,namespace,prior_epoch,prior_cursor) VALUES ($1,$2,$3,$4)
ON CONFLICT (namespace,prior_epoch,prior_cursor) DO NOTHING;

-- name: CollectionResultSource :one
SELECT s.* FROM collection_sources s WHERE s.target_id=$1 AND s.namespace=$2 AND EXISTS (
 SELECT 1 FROM collection_plans p JOIN collection_plan_bindings m ON m.plan_id=p.id
 JOIN collection_bindings b ON b.id=m.binding_id
 WHERE p.source_id=s.id AND p.capability=$3 AND p.scope=$4
 AND m.attached_at<=sqlc.arg(evidence_at)::timestamptz
 AND (p.capability<>'web.dns' OR (p.params->'types') ? sqlc.arg(rrtype)::text)
 AND b.effective_from<=sqlc.arg(evidence_at)::timestamptz
 AND (b.effective_until IS NULL OR b.effective_until>sqlc.arg(evidence_at)::timestamptz)
);

-- name: CollectionCurrent :one
SELECT * FROM collection_current WHERE generation=$1 AND source_id=$2 AND capability=$3 AND output_schema=$4 AND scope_key=$5;

-- name: PutCollectionCurrent :exec
INSERT INTO collection_current (generation,source_id,capability,output_schema,scope_key,scope,position,state,
 observation_id,observed_at,received_at,confirmed_at,content_hash,evidence,ruleset,data,facts)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
ON CONFLICT (generation,source_id,capability,output_schema,scope_key) DO UPDATE SET
 scope=excluded.scope,position=excluded.position,state=excluded.state,observation_id=excluded.observation_id,
 observed_at=excluded.observed_at,received_at=excluded.received_at,confirmed_at=excluded.confirmed_at,
 content_hash=excluded.content_hash,evidence=excluded.evidence,ruleset=excluded.ruleset,data=excluded.data,facts=excluded.facts
WHERE collection_current.position<excluded.position OR (collection_current.position=excluded.position
 AND collection_current.observation_id=excluded.observation_id
 AND excluded.confirmed_at>COALESCE(collection_current.confirmed_at,'-infinity'));

-- name: DeleteOldCollectionRows :execrows
DELETE FROM collection_current WHERE ctid IN (
 SELECT c.ctid FROM collection_current c JOIN collection_sources s ON s.id=c.source_id
 WHERE s.namespace=sqlc.arg(namespace) AND c.generation IS DISTINCT FROM sqlc.narg(generation)::uuid
 LIMIT 2048
);

-- name: SiteCollectionBindingsAt :many
SELECT DISTINCT ON (b.site_id) b.id,b.site_id,greatest(b.effective_from,m.attached_at)::timestamptz AS attributable_since FROM collection_bindings b
JOIN collection_plan_bindings m ON m.binding_id=b.id JOIN collection_plans p ON p.id=m.plan_id
WHERE b.source_id=$1 AND b.site_id IS NOT NULL AND p.capability=$2 AND p.scope=$3
 AND m.attached_at<=sqlc.arg(evidence_at)::timestamptz
 AND (p.capability<>'web.dns' OR (p.params->'types') ? sqlc.arg(rrtype)::text)
 AND b.effective_from<=sqlc.arg(evidence_at)::timestamptz
 AND (b.effective_until IS NULL OR b.effective_until>sqlc.arg(evidence_at)::timestamptz)
ORDER BY b.site_id,b.effective_from DESC,b.id DESC;

-- name: AddSiteEvidenceChange :exec
INSERT INTO site_evidence_changes (id,site_id,binding_id,target_id,epoch,position,capability,output_schema,scope_key,scope,
 fact_key,prior_value,value,ruleset,prior_ruleset,source_observation_id,content_hash,evidence,first_observed_at,source_observed_at,received_at,event_kind)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
ON CONFLICT (binding_id,epoch,position,fact_key) DO NOTHING;

-- name: ConfirmCollectionCurrent :execrows
UPDATE collection_current SET confirmed_at=sqlc.arg(confirmed_at)
WHERE generation=sqlc.arg(generation) AND source_id=sqlc.arg(source_id) AND capability=sqlc.arg(capability)
 AND output_schema=sqlc.arg(output_schema) AND scope_key=sqlc.arg(scope_key)
 AND position=sqlc.arg(position) AND observation_id=sqlc.arg(observation_id)
 AND confirmed_at<sqlc.arg(confirmed_at);

-- name: PutSiteDailyCoverage :exec
INSERT INTO site_daily_coverage (site_id,source_id,binding_id,capability,output_schema,scope_key,utc_day,confirmed_at,evidence_state,observation_id)
VALUES (sqlc.arg(site_id),sqlc.arg(source_id),sqlc.arg(binding_id),sqlc.arg(capability),sqlc.arg(output_schema),
 sqlc.arg(scope_key),(sqlc.arg(confirmed_at)::timestamptz AT TIME ZONE 'UTC')::date,sqlc.arg(confirmed_at),
 sqlc.arg(evidence_state),sqlc.arg(observation_id))
ON CONFLICT (site_id,source_id,capability,output_schema,scope_key,utc_day) DO UPDATE SET
 binding_id=excluded.binding_id,confirmed_at=excluded.confirmed_at,evidence_state=excluded.evidence_state,observation_id=excluded.observation_id
WHERE site_daily_coverage.confirmed_at<excluded.confirmed_at;

-- name: PutGameEvidenceNews :exec
INSERT INTO game_evidence_news (source_id,scope_key,gid,published_at,observed_at,observation_id,value)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (source_id,scope_key,gid) DO UPDATE SET
 published_at=excluded.published_at,observed_at=excluded.observed_at,observation_id=excluded.observation_id,value=excluded.value
WHERE game_evidence_news.observed_at<excluded.observed_at;

-- name: BoundGameEvidenceNews :exec
DELETE FROM game_evidence_news n WHERE n.source_id=$1 AND n.scope_key=$2 AND n.gid IN (
 SELECT older.gid FROM game_evidence_news older WHERE older.source_id=$1 AND older.scope_key=$2
 ORDER BY older.published_at DESC,older.gid DESC OFFSET 200
);

-- name: PutGameEvidencePlayers :exec
INSERT INTO game_evidence_players (source_id,scope_key,bucket,observed_at,observation_id,players)
VALUES (sqlc.arg(source_id),sqlc.arg(scope_key),date_bin('30 minutes',sqlc.arg(observed_at)::timestamptz,'2000-01-01 UTC'),
 sqlc.arg(observed_at),sqlc.arg(observation_id),sqlc.arg(players))
ON CONFLICT (source_id,scope_key,bucket) DO UPDATE SET observed_at=excluded.observed_at,observation_id=excluded.observation_id,players=excluded.players
WHERE game_evidence_players.observed_at<excluded.observed_at;

-- name: PruneGameEvidence :exec
WITH old_players AS (
 DELETE FROM game_evidence_players WHERE ctid IN
 (SELECT ctid FROM game_evidence_players WHERE bucket < transaction_timestamp() - interval '30 days' ORDER BY bucket LIMIT 512)
)
DELETE FROM game_evidence_news WHERE ctid IN
(SELECT ctid FROM game_evidence_news WHERE published_at < transaction_timestamp() - interval '180 days' ORDER BY published_at LIMIT 512);

-- name: TryCollectionLock :one
SELECT pg_try_advisory_lock(hashtextextended('gofurry/harvest/'||sqlc.arg(namespace)::text,0));

-- name: ReleaseCollectionLock :one
SELECT pg_advisory_unlock(hashtextextended('gofurry/harvest/'||sqlc.arg(namespace)::text,0));


-- name: ActivateSiteCoverage :exec
INSERT INTO site_daily_coverage (site_id,source_id,binding_id,capability,output_schema,scope_key,utc_day,confirmed_at,evidence_state,observation_id)
SELECT DISTINCT ON (b.site_id,c.source_id,c.capability,c.output_schema,c.scope_key)
 b.site_id,c.source_id,b.id,c.capability,c.output_schema,c.scope_key,(c.confirmed_at AT TIME ZONE 'UTC')::date,
 c.confirmed_at,c.state,c.observation_id
FROM collection_current c
JOIN collection_bindings b ON b.source_id=c.source_id
JOIN collection_plan_bindings m ON m.binding_id=b.id
JOIN collection_plans p ON p.id=m.plan_id AND p.capability=c.capability
WHERE c.generation=$1 AND b.site_id IS NOT NULL AND c.state IN ('present','confirmed_empty') AND c.observation_id IS NOT NULL
 AND p.scope=c.scope-'namespace'-'dimensions'
 AND (c.capability<>'web.dns' OR (p.params->'types') ? (c.scope->'dimensions'->>'rrtype'))
 AND c.confirmed_at>=greatest(b.effective_from,m.attached_at)
 AND (b.effective_until IS NULL OR b.effective_until>c.confirmed_at)
ORDER BY b.site_id,c.source_id,c.capability,c.output_schema,c.scope_key,b.effective_from DESC,b.id DESC
ON CONFLICT (site_id,source_id,capability,output_schema,scope_key,utc_day) DO UPDATE SET
 binding_id=excluded.binding_id,confirmed_at=excluded.confirmed_at,evidence_state=excluded.evidence_state,observation_id=excluded.observation_id
WHERE site_daily_coverage.confirmed_at<excluded.confirmed_at;

-- name: ActivateGameNews :exec
INSERT INTO game_evidence_news (source_id,scope_key,gid,published_at,observed_at,observation_id,value)
SELECT c.source_id,c.scope_key,item->>'gid',(item->>'published_at')::timestamptz,c.observed_at,c.observation_id,item
FROM collection_current c CROSS JOIN LATERAL jsonb_array_elements(c.data->'items') item
WHERE c.generation=$1 AND c.capability='steam.news' AND c.state='present'
ON CONFLICT (source_id,scope_key,gid) DO UPDATE SET
 published_at=excluded.published_at,observed_at=excluded.observed_at,observation_id=excluded.observation_id,value=excluded.value
WHERE game_evidence_news.observed_at<excluded.observed_at;

-- name: BoundActivatedGameNews :exec
DELETE FROM game_evidence_news n WHERE (n.source_id,n.scope_key,n.gid) IN (
 SELECT ranked.source_id,ranked.scope_key,ranked.gid FROM (
  SELECT b.source_id,b.scope_key,b.gid,row_number() OVER(PARTITION BY b.source_id,b.scope_key ORDER BY b.published_at DESC,b.gid DESC) AS rank
  FROM game_evidence_news b
  JOIN collection_current c ON c.source_id=b.source_id AND c.scope_key=b.scope_key
  WHERE c.generation=$1 AND c.capability='steam.news' AND c.state='present'
 ) ranked WHERE ranked.rank>200
);

-- name: ActivateGamePlayers :exec
INSERT INTO game_evidence_players (source_id,scope_key,bucket,observed_at,observation_id,players)
SELECT c.source_id,c.scope_key,date_bin('30 minutes',c.observed_at,'2000-01-01 UTC'),c.observed_at,c.observation_id,(c.data->>'player_count')::bigint
FROM collection_current c WHERE c.generation=$1 AND c.capability='steam.players' AND c.state='present'
 AND c.observed_at>=transaction_timestamp()-interval '30 days'
ON CONFLICT (source_id,scope_key,bucket) DO UPDATE SET observed_at=excluded.observed_at,observation_id=excluded.observation_id,players=excluded.players
WHERE game_evidence_players.observed_at<excluded.observed_at;

-- name: ActivateSiteBaselines :exec
WITH bindings AS (
 SELECT DISTINCT ON (b.site_id,c.source_id,c.capability,c.output_schema,c.scope_key)
  b.id AS binding_id,b.site_id,s.target_id,c.capability,c.output_schema,c.scope_key,c.scope,c.position,
  c.observation_id,c.observed_at,c.received_at,c.content_hash,c.evidence,c.ruleset,c.facts,
  CASE WHEN c.observed_at>=greatest(b.effective_from,m.attached_at) THEN c.observed_at ELSE c.confirmed_at END AS first_observed_at
 FROM collection_current c JOIN collection_sources s ON s.id=c.source_id
 JOIN collection_bindings b ON b.source_id=c.source_id
 JOIN collection_plan_bindings m ON m.binding_id=b.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.capability=c.capability
 WHERE c.generation=sqlc.arg(generation) AND c.state IN ('present','confirmed_empty') AND b.site_id IS NOT NULL
  AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id)::uuid)
  AND (sqlc.narg(scope_key)::text IS NULL OR c.scope_key=sqlc.narg(scope_key)::text)
  AND p.scope=c.scope-'namespace'-'dimensions'
  AND (c.capability<>'web.dns' OR (p.params->'types') ? (c.scope->'dimensions'->>'rrtype'))
  AND c.confirmed_at>=greatest(b.effective_from,m.attached_at)
  AND (b.effective_until IS NULL OR b.effective_until>c.confirmed_at)
 ORDER BY b.site_id,c.source_id,c.capability,c.output_schema,c.scope_key,b.effective_from DESC,b.id DESC
)
INSERT INTO site_evidence_changes (id,site_id,binding_id,target_id,epoch,position,capability,output_schema,scope_key,scope,
 fact_key,event_kind,value,ruleset,prior_ruleset,source_observation_id,content_hash,evidence,first_observed_at,source_observed_at,received_at)
SELECT uuidv7(),b.site_id,b.binding_id,b.target_id,sqlc.arg(epoch),b.position,b.capability,b.output_schema,b.scope_key,b.scope,
 f.key,'baseline',f.value,b.ruleset,'',b.observation_id,b.content_hash,b.evidence,b.first_observed_at,b.observed_at,b.received_at
FROM bindings b CROSS JOIN LATERAL jsonb_each(b.facts) f
WHERE NOT EXISTS (SELECT 1 FROM site_evidence_changes e
 WHERE e.binding_id=b.binding_id AND e.capability=b.capability AND e.scope_key=b.scope_key AND e.fact_key=f.key)
ON CONFLICT (binding_id,epoch,position,fact_key) DO NOTHING;

-- name: DeleteRetiredCollectionCurrent :exec
DELETE FROM collection_current WHERE ctid IN (
 SELECT c.ctid FROM collection_current c JOIN collection_sources s ON s.id=c.source_id
 WHERE s.namespace=$1 AND COALESCE(c.confirmed_at,c.received_at)<transaction_timestamp()-interval '30 days'
 AND NOT EXISTS (
  SELECT 1 FROM collection_plans p JOIN collection_plan_bindings m ON m.plan_id=p.id
  JOIN collection_bindings b ON b.id=m.binding_id
  WHERE p.source_id=c.source_id AND p.capability=c.capability AND p.scope=c.scope-'namespace'-'dimensions'
   AND (c.capability<>'web.dns' OR (p.params->'types') ? (c.scope->'dimensions'->>'rrtype'))
   AND b.effective_until IS NULL
 )
 LIMIT 512
);

-- name: GetUnknownCollectionPlan :one
SELECT p.*,s.target_id FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id
WHERE p.id=$1 AND s.namespace=$2 AND p.post_started AND p.schedule_id IS NULL AND s.target_id IS NOT NULL;

-- name: ReadSiteEvidence :many
SELECT * FROM site_evidence_changes
WHERE site_id=sqlc.arg(site_id) AND first_observed_at>=sqlc.arg(from_time) AND first_observed_at<sqlc.arg(until_time)
 AND (first_observed_at,id)>(sqlc.arg(after_time),sqlc.arg(after_id))
ORDER BY first_observed_at,id LIMIT sqlc.arg(page_size);

-- name: ReadSiteCoverage :many
SELECT * FROM site_daily_coverage WHERE site_id=sqlc.arg(site_id) AND utc_day>=sqlc.arg(from_day) AND utc_day<sqlc.arg(until_day)
AND (sqlc.narg(after_day)::date IS NULL OR
 (utc_day,source_id,capability,output_schema,scope_key)>(sqlc.narg(after_day)::date,sqlc.arg(after_source),sqlc.arg(after_capability),sqlc.arg(after_schema),sqlc.arg(after_scope)))
ORDER BY utc_day,source_id,capability,output_schema,scope_key LIMIT sqlc.arg(page_size);
