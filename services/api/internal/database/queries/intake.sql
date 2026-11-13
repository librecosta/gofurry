-- name: LockIntakeAdmission :exec
SELECT pg_advisory_xact_lock(7748032881221201::bigint);

-- name: TryIntakeDispatch :one
SELECT pg_try_advisory_xact_lock(7748032881221202::bigint)::boolean;

-- name: IntakeCandidate :one
SELECT * FROM game_intake_candidates WHERE platform_code=$1 AND external_key=$2;

-- name: InsertIntakeCandidate :one
INSERT INTO game_intake_candidates (id,platform_code,external_key,discovered_via)
SELECT $1,$2,$3,$4 WHERE (
 SELECT count(*) FROM game_intake_candidates WHERE state IN ('discovered','resolving','deferred')
)<10000
RETURNING *;

-- name: LockIntakeCandidate :one
SELECT * FROM game_intake_candidates WHERE platform_code=$1 AND external_key=$2 FOR UPDATE;

-- name: ClaimIntake :one
WITH due AS (
 SELECT i.id FROM game_intake_candidates i
 WHERE i.state IN ('discovered','deferred','resolving') AND i.next_at<=statement_timestamp()
   AND (i.namespace='' OR i.namespace=sqlc.arg(namespace))
   AND NOT EXISTS (SELECT 1 FROM game_intake_candidates busy WHERE busy.state='resolving' AND busy.lease_until>statement_timestamp())
 ORDER BY i.next_at,i.id LIMIT 1 FOR UPDATE OF i SKIP LOCKED
)
UPDATE game_intake_candidates i SET state='resolving',lease_id=sqlc.arg(lease_id),
 lease_until=statement_timestamp()+interval '45 seconds',next_at=statement_timestamp()+interval '45 seconds',
 namespace=CASE WHEN i.namespace='' AND i.platform_code='steam' THEN sqlc.arg(namespace) ELSE i.namespace END,
 egress_region=CASE WHEN i.egress_region='' AND i.platform_code='steam' THEN sqlc.arg(egress_region) ELSE i.egress_region END,
 updated_at=statement_timestamp()
FROM due WHERE i.id=due.id RETURNING i.*;

-- name: LockIntakeLease :one
SELECT * FROM game_intake_candidates WHERE id=$1 AND lease_id=$2 AND lease_until>statement_timestamp() FOR UPDATE;

-- name: SaveIntakeTarget :execrows
UPDATE game_intake_candidates SET target_id=$3, updated_at=statement_timestamp()
WHERE id=$1 AND lease_id=$2 AND lease_until>statement_timestamp() AND (target_id IS NULL OR target_id=$3);

-- name: StartIntakeJob :one
UPDATE game_intake_candidates SET job_started_at=coalesce(job_started_at,statement_timestamp()),
 job_writer_binding=sqlc.arg(writer_binding)
WHERE id=sqlc.arg(id) AND lease_id=sqlc.arg(lease_id) AND lease_until>statement_timestamp()
 AND (job_writer_binding='' OR job_writer_binding=sqlc.arg(writer_binding)) RETURNING job_started_at;

-- name: SaveIntakeJob :execrows
UPDATE game_intake_candidates SET job_id=$3, updated_at=statement_timestamp()
WHERE id=$1 AND lease_id=$2 AND lease_until>statement_timestamp() AND (job_id IS NULL OR job_id=$3);

-- name: RestartIntakeJob :execrows
UPDATE game_intake_candidates SET job_round=2,job_id=NULL,job_started_at=NULL,job_writer_binding='',
 state='deferred',reason='job_retry',next_at=statement_timestamp()+interval '5 minutes',
 lease_id=NULL,lease_until=NULL,failures=0,updated_at=statement_timestamp()
WHERE id=$1 AND lease_id=$2 AND job_id=$3 AND lease_until>statement_timestamp() AND job_round=1;

-- name: ReleaseIntake :execrows
UPDATE game_intake_candidates SET state=sqlc.arg(state),reason=sqlc.arg(reason),
 lease_id=NULL,lease_until=NULL,
 next_at=CASE WHEN sqlc.arg(delay_seconds)::integer>0 THEN statement_timestamp()+make_interval(secs=>sqlc.arg(delay_seconds)::integer) END,
 failures=sqlc.arg(failures),updated_at=statement_timestamp(),decided_at=statement_timestamp(),decision_version=1
WHERE id=sqlc.arg(id) AND lease_id=sqlc.arg(lease_id) AND lease_until>statement_timestamp();

-- name: SaveIntakeDecision :exec
UPDATE game_intake_candidates SET state='accepted',reason=sqlc.arg(reason),
 game_id=sqlc.arg(game_id),game_source_id=sqlc.arg(game_source_id),relation_attached=sqlc.arg(relation_attached),
 attempt_id=sqlc.arg(attempt_id),observation_id=sqlc.arg(observation_id),observation_hash=sqlc.arg(observation_hash),
 profile_scope_key=sqlc.arg(profile_scope_key),profile=sqlc.arg(profile),
 profile_at=sqlc.arg(profile_at),profile_observation_id=sqlc.arg(observation_id),work_title=sqlc.arg(work_title),
 lease_id=NULL,lease_until=NULL,next_at=NULL,failures=0,decision_version=1,decided_at=statement_timestamp(),updated_at=statement_timestamp()
WHERE id=sqlc.arg(id);

-- name: NoteIntakeObservation :exec
UPDATE game_intake_candidates SET attempt_id=$2,observation_id=$3,observation_hash=$4,
 profile_scope_key=$5,profile=$6,profile_at=$7,work_title=$8
WHERE id=$1;

-- name: ExistingIntakeSource :one
SELECT s.* FROM game_sources s WHERE s.platform_code=$1 AND s.external_key=$2;

-- name: ExistingIntakeItchSources :many
SELECT s.* FROM game_sources s WHERE s.platform_code='itch_io' AND lower(btrim(s.external_key))=$1 LIMIT 2;

-- name: InsertIntakeSource :one
INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (platform_code,external_key) DO NOTHING RETURNING *;

-- name: IntakeSourceCapacity :one
SELECT count(*)<16 FROM game_sources WHERE game_id=$1 AND active;

-- name: PickIntakeSource :exec
UPDATE games SET intake_display_source=coalesce(intake_display_source,$2) WHERE id=$1;

-- name: IntakeTitleConflict :one
SELECT (
 EXISTS (SELECT 1 FROM game_intake_candidates WHERE state='accepted' AND work_title<>'' AND work_title=sqlc.arg(title))
 OR EXISTS (SELECT 1 FROM game_localizations WHERE lower(btrim(name))=sqlc.arg(title))
)::boolean;

-- name: PutIntakeReview :exec
UPDATE game_intake_candidates SET review=$2,review_revoked=false,updated_at=statement_timestamp()
WHERE id=$1;

-- name: RevokeIntakeReview :exec
UPDATE game_intake_candidates SET review_revoked=true,updated_at=statement_timestamp() WHERE id=$1;

-- name: WakeIntake :exec
UPDATE game_intake_candidates SET state='discovered',reason='',next_at=statement_timestamp(),failures=0,updated_at=statement_timestamp()
WHERE id=$1 AND state='deferred' AND next_at IS NULL;

-- name: DismissIntake :execrows
UPDATE game_intake_candidates SET state='rejected',reason='operator_dismissed',decision_version=1,
 next_at=NULL,decided_at=statement_timestamp(),updated_at=statement_timestamp()
WHERE id=$1 AND state IN ('discovered','deferred');

-- name: SaveIntakeRelation :exec
UPDATE game_intake_candidates SET relation=$2,updated_at=statement_timestamp() WHERE id=$1;

-- name: NoteIntakeConflict :exec
UPDATE game_intake_candidates SET reason='merge_suggested',decided_at=statement_timestamp(),updated_at=statement_timestamp() WHERE id=$1;

-- name: RevokeIntakeRelation :exec
UPDATE game_intake_candidates SET state='rejected',reason='relation_revoked',next_at=NULL,decided_at=statement_timestamp(),updated_at=statement_timestamp()
WHERE id=$1;

-- name: DeactivateIntakeSource :exec
UPDATE game_sources SET active=false WHERE id=$1 AND game_id=$2;

-- name: MarkIntakePublished :exec
UPDATE game_intake_candidates SET published_at=statement_timestamp(),updated_at=statement_timestamp() WHERE id=$1;

-- name: FindIntakePlan :one
SELECT p.id FROM collection_plans p
WHERE p.source_id=$1 AND p.capability='steam.details' AND p.request_schema=1 AND p.params=$2 AND p.scope=$3 AND p.policy='state_refresh'
ORDER BY (p.schedule_id IS NOT NULL) DESC,p.id LIMIT 1;

-- name: IntakeStatus :many
SELECT i.state,i.reason,coalesce(g.status,'')::text AS game_status,count(*) AS total
FROM game_intake_candidates i LEFT JOIN games g ON g.id=i.game_id
GROUP BY i.state,i.reason,g.status ORDER BY i.state,i.reason,g.status;

-- name: IntakeGameStatus :one
SELECT g.public_id,g.revision,
 (CASE WHEN g.status='published' AND g.publication_blocked
   THEN 'hidden' ELSE g.status END)::text AS game_status
FROM games g WHERE g.id=$1;

-- name: WakeRelatedIntake :exec
UPDATE game_intake_candidates SET next_at=statement_timestamp(),updated_at=statement_timestamp()
WHERE state='deferred' AND reason='related_source_unavailable' AND next_at IS NULL
 AND relation->>'platform'=sqlc.arg(platform)::text AND relation->>'key'=sqlc.arg(source_key)::text;

-- name: PruneIntakeAbandoned :execrows
DELETE FROM game_intake_candidates WHERE id IN (
 SELECT id FROM game_intake_candidates
 WHERE state='deferred' AND next_at IS NULL AND job_started_at IS NULL AND target_id IS NULL
   AND review='{}' AND relation='{}' AND updated_at<statement_timestamp()-interval '30 days'
 ORDER BY updated_at LIMIT 128 FOR UPDATE SKIP LOCKED
);

-- name: RefreshIntakeProfiles :many
WITH newer AS (
 SELECT i.id,c.observation_id,c.confirmed_at,c.received_at,c.state,c.data
 FROM collection_current c JOIN collection_sources s ON s.id=c.source_id
 JOIN game_intake_candidates i ON i.namespace=s.namespace AND i.platform_code='steam' AND i.external_key=s.external_key
    AND i.profile_scope_key=c.scope_key AND i.state='accepted'
 JOIN game_sources gs ON gs.id=i.game_source_id AND gs.game_id=i.game_id AND gs.active
 WHERE s.provider='steam' AND s.resource_type='app' AND (i.target_id IS NULL OR i.target_id=s.target_id)
    AND c.generation=sqlc.arg(generation) AND c.capability='steam.details' AND c.output_schema=1
    AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id))
    AND (sqlc.narg(scope_key)::text IS NULL OR c.scope_key=sqlc.narg(scope_key))
    AND coalesce(c.confirmed_at,c.received_at)>coalesce(i.profile_at,'-infinity')
    AND (c.state='withdrawn' OR c.data->>'type' IS DISTINCT FROM 'game'
      OR (c.data->>'name'<>'' AND c.data->>'description'<>''))
    AND EXISTS (
      SELECT 1 FROM collection_bindings b JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=b.source_id
      JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=b.source_id
      WHERE b.source_id=s.id AND b.game_source_id=gs.id AND b.game_id=i.game_id AND b.effective_until IS NULL
        AND p.capability=c.capability AND p.request_schema=c.output_schema AND p.scope=c.scope-'namespace'-'dimensions'
        AND m.attached_at<=coalesce(c.confirmed_at,c.received_at) AND b.effective_from<=coalesce(c.confirmed_at,c.received_at)
    )
)
UPDATE game_intake_candidates i SET
 profile=CASE WHEN n.state='present' AND n.data->>'type'='game'
   THEN jsonb_build_object('locale','en','name',n.data->>'name','summary',n.data->>'description') ELSE '{}'::jsonb END,
 profile_at=coalesce(n.confirmed_at,n.received_at),
 profile_observation_id=CASE WHEN n.state='present' AND n.data->>'type'='game' THEN n.observation_id END,
 updated_at=statement_timestamp()
FROM newer n WHERE i.id=n.id AND coalesce(n.confirmed_at,n.received_at)>coalesce(i.profile_at,'-infinity')
RETURNING i.id,i.game_id,i.reason;
