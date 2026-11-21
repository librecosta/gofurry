-- name: ManagementCollectionEntity :one
SELECT s.id FROM sites s WHERE sqlc.arg(kind)::text='sites' AND s.public_id=sqlc.arg(entity_public_id)
UNION ALL SELECT g.id FROM games g WHERE sqlc.arg(kind)='games' AND g.public_id=sqlc.arg(entity_public_id);

-- name: ManagementCollectionTargets :many
SELECT DISTINCT src.id,src.provider,src.resource_type,src.external_key,src.target_id
FROM collection_bindings b JOIN collection_sources src ON src.id=b.source_id
WHERE b.effective_until IS NULL AND (
 (sqlc.arg(kind)::text='sites' AND b.site_id=sqlc.arg(entity_id)::uuid AND EXISTS (SELECT 1 FROM site_addresses a WHERE a.id=b.address_id AND a.active))
 OR (sqlc.arg(kind)='games' AND b.game_id=sqlc.arg(entity_id) AND EXISTS (SELECT 1 FROM game_sources gs WHERE gs.id=b.game_source_id AND gs.active)))
ORDER BY src.id LIMIT 33;

-- name: ListEntityCollectionPlans :many
WITH bound AS MATERIALIZED (
 SELECT DISTINCT ON (p.id) p.id,p.source_id,p.capability,p.request_schema,p.params,p.scope,p.interval_seconds,p.anchor_at,
  p.policy,p.schedule_id,p.remote_enabled,p.checked_at,src.namespace,src.provider,src.external_key,src.target_id,
  b.id AS binding_id,b.site_id,b.game_source_id,m.enabled AS desired_enabled,EXISTS (SELECT FROM sites st WHERE st.id=b.site_id AND st.status<>'archived' UNION ALL SELECT FROM games g WHERE g.id=b.game_id AND g.status<>'archived')::boolean AS actionable,greatest(b.effective_from,m.attached_at)::timestamptz AS since
 FROM collection_bindings b JOIN collection_sources src ON src.id=b.source_id
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=b.source_id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id
 WHERE b.effective_until IS NULL
  AND (sqlc.narg(only_plan)::uuid IS NULL OR p.id=sqlc.narg(only_plan))
  AND (sqlc.narg(after_plan)::uuid IS NULL OR p.id>sqlc.narg(after_plan))
  AND ((sqlc.arg(kind)::text='sites' AND b.site_id=sqlc.arg(entity_id)::uuid AND EXISTS (SELECT 1 FROM site_addresses a WHERE a.id=b.address_id AND a.active))
   OR (sqlc.arg(kind)='games' AND b.game_id=sqlc.arg(entity_id) AND EXISTS (SELECT 1 FROM game_sources gs WHERE gs.id=b.game_source_id AND gs.active)))
 ORDER BY p.id,since,b.id LIMIT 9
)
SELECT b.*,coalesce(last.state,'')::text AS observation_state,last.observation_id,
 coalesce(last.scope_key,'')::text AS scope_key,last.observed_at,
 greatest(last.success_at, CASE WHEN b.capability='steam.details' THEN (
   SELECT max(md.confirmed_at) FROM game_source_metadata md
   WHERE md.binding_id=b.binding_id AND md.game_source_id=b.game_source_id AND md.payload IS NOT NULL
    AND md.confirmed_at>=b.since AND md.locale=b.scope->>'locale' AND md.store_country=b.scope->>'region'
    AND md.egress_region=b.scope->>'egress_region'
  ) END, (SELECT max(d.confirmed_at) FROM site_daily_coverage d WHERE d.site_id=b.site_id AND d.binding_id=b.binding_id
   AND d.capability=b.capability AND d.scope_key=last.scope_key AND d.evidence_state='present' AND d.confirmed_at>=b.since))::timestamptz AS last_success_at
FROM bound b LEFT JOIN collection_consumer cn ON cn.namespace=b.namespace
LEFT JOIN LATERAL (
 SELECT c.state,c.observation_id,c.scope_key,c.observed_at,
  max(coalesce(c.confirmed_at,c.observed_at)) FILTER (WHERE c.state='present') OVER () AS success_at
 FROM collection_current c
 WHERE c.generation=cn.generation AND c.source_id=b.source_id AND c.capability=b.capability AND c.output_schema=1
  AND c.scope->>'namespace'=b.namespace AND c.scope-'namespace'-'dimensions'=b.scope
  AND (c.capability<>'web.dns' OR (b.params->'types') ? (c.scope->'dimensions'->>'rrtype'))
  AND coalesce(c.confirmed_at,c.received_at)>=b.since
 ORDER BY coalesce(c.confirmed_at,c.received_at) DESC,c.position DESC LIMIT 1
) last ON true ORDER BY b.id;
