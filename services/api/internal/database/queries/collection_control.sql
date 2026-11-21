-- name: ListManagementPlanOwners :many
SELECT DISTINCT ON (p.id) p.id,
 CASE WHEN b.site_id IS NOT NULL THEN 'sites' ELSE 'games' END::text AS entity_kind,
 coalesce(st.public_id,g.public_id)::text AS public_id,
 coalesce((SELECT l.name FROM site_localizations l WHERE l.site_id=st.id ORDER BY (l.locale='zh') DESC,l.locale LIMIT 1),
          (SELECT l.name FROM game_localizations l WHERE l.game_id=g.id ORDER BY (l.locale='zh') DESC,l.locale LIMIT 1),
          st.slug,g.slug)::text AS name
FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id
JOIN collection_plan_bindings m ON m.plan_id=p.id AND m.source_id=s.id
JOIN collection_bindings b ON b.id=m.binding_id AND b.source_id=s.id AND b.effective_until IS NULL
LEFT JOIN sites st ON st.id=b.site_id LEFT JOIN games g ON g.id=b.game_id
WHERE s.namespace=sqlc.arg(namespace) AND (sqlc.narg(after_plan)::uuid IS NULL OR p.id>sqlc.narg(after_plan))
 AND (sqlc.arg(kind)::text='' OR (sqlc.arg(kind)='sites' AND st.id IS NOT NULL) OR (sqlc.arg(kind)='games' AND g.id IS NOT NULL))
 AND (sqlc.arg(public_id)::text='' OR coalesce(st.public_id,g.public_id)=sqlc.arg(public_id))
 AND (sqlc.arg(capability)::text='' OR p.capability=sqlc.arg(capability))
 AND (sqlc.arg(keyword)::text='' OR strpos(lower(s.external_key),lower(sqlc.arg(keyword)))>0
   OR EXISTS (SELECT 1 FROM site_localizations l WHERE l.site_id=st.id AND strpos(lower(l.name),lower(sqlc.arg(keyword)))>0)
   OR EXISTS (SELECT 1 FROM game_localizations l WHERE l.game_id=g.id AND strpos(lower(l.name),lower(sqlc.arg(keyword)))>0))
ORDER BY p.id,(coalesce(st.status,g.status)<>'archived') DESC,m.enabled DESC,m.attached_at,b.id LIMIT 9;

-- name: ManagementJobOwners :many
SELECT p.id,p.schedule_id,p.capability,p.params,p.scope,p.request_schema,s.target_id,
 CASE WHEN b.site_id IS NOT NULL THEN 'sites' ELSE 'games' END::text AS entity_kind,
 coalesce(st.public_id,g.public_id)::text AS public_id,
 coalesce((SELECT l.name FROM site_localizations l WHERE l.site_id=st.id ORDER BY (l.locale='zh') DESC,l.locale LIMIT 1),
          (SELECT l.name FROM game_localizations l WHERE l.game_id=g.id ORDER BY (l.locale='zh') DESC,l.locale LIMIT 1),
          st.slug,g.slug)::text AS name,
 s.provider,s.external_key,
 (b.effective_until IS NULL AND coalesce(st.status,g.status)<>'archived')::boolean AS actionable,
 greatest(b.effective_from,m.attached_at)::timestamptz AS bound_at,b.effective_until
FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id
JOIN collection_plan_bindings m ON m.plan_id=p.id AND m.source_id=p.source_id
JOIN collection_bindings b ON b.id=m.binding_id AND b.source_id=m.source_id
LEFT JOIN sites st ON st.id=b.site_id LEFT JOIN games g ON g.id=b.game_id
WHERE s.namespace=sqlc.arg(namespace) AND p.schedule_id=ANY(sqlc.arg(schedule_ids)::uuid[])
 AND (sqlc.arg(entity)::text='' OR coalesce(st.public_id,g.public_id)=sqlc.arg(entity))
ORDER BY p.schedule_id,(b.effective_until IS NULL) DESC,m.attached_at,b.id LIMIT 200;

-- name: SetManagedPlanBinding :execrows
UPDATE collection_plan_bindings SET enabled=sqlc.arg(enabled)
WHERE plan_id=sqlc.arg(plan_id) AND binding_id=sqlc.arg(binding_id) AND enabled=sqlc.arg(expected);

-- name: ManagementEntityBinding :one
SELECT b.id,b.resource_type,b.site_id,b.game_id FROM collection_bindings b JOIN collection_sources s ON s.id=b.source_id
LEFT JOIN sites st ON st.id=b.site_id LEFT JOIN games g ON g.id=b.game_id
WHERE s.namespace=sqlc.arg(namespace) AND s.id=sqlc.arg(source_id) AND b.effective_until IS NULL
 AND ((sqlc.arg(kind)::text='sites' AND st.public_id=sqlc.arg(public_id) AND st.status<>'archived') OR
      (sqlc.arg(kind)='games' AND g.public_id=sqlc.arg(public_id) AND g.status<>'archived'))
ORDER BY b.id LIMIT 1 FOR UPDATE OF b;

-- name: CountUnsynchronizedCollectionPlans :one
SELECT count(*)
FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id
CROSS JOIN LATERAL (SELECT EXISTS (
 SELECT FROM collection_plan_bindings m JOIN collection_bindings b ON b.id=m.binding_id
 LEFT JOIN sites st ON st.id=b.site_id LEFT JOIN games g ON g.id=b.game_id
 WHERE m.plan_id=p.id AND m.enabled AND b.effective_until IS NULL AND coalesce(st.status,g.status)<>'archived'
) AS enabled) desired
WHERE s.namespace=sqlc.arg(namespace) AND
 ((p.schedule_id IS NULL AND desired.enabled) OR
  (p.schedule_id IS NOT NULL AND p.remote_enabled IS DISTINCT FROM desired.enabled));

-- name: LatestCollectionConfirmation :one
SELECT max(coalesce(c.confirmed_at,c.observed_at))::timestamptz
 FROM collection_current c JOIN collection_sources s ON s.id=c.source_id
 JOIN collection_consumer g ON g.namespace=s.namespace AND g.generation=c.generation
 WHERE s.namespace=sqlc.arg(namespace) AND c.state='present';

-- name: ManagementPlanResults :many
SELECT c.observation_id,c.state,c.observed_at,c.data,
 coalesce(c.scope->'dimensions'->>'rrtype',c.scope->'dimensions'->>'section','')::text AS dimension
 FROM collection_current c JOIN collection_sources s ON s.id=c.source_id
 JOIN collection_consumer g ON g.namespace=s.namespace AND g.generation=c.generation
 JOIN collection_plans p ON p.source_id=s.id AND p.capability=c.capability
 WHERE p.id=sqlc.arg(plan_id) AND c.output_schema=1 AND c.scope-'namespace'-'dimensions'=p.scope
  AND (c.capability<>'web.dns' OR (p.params->'types') ? (c.scope->'dimensions'->>'rrtype'))
  AND coalesce(c.confirmed_at,c.received_at)>=sqlc.arg(bound_at)::timestamptz
 ORDER BY dimension,c.scope_key LIMIT 12;
