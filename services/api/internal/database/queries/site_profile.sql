-- name: SetSiteDescription :execrows
UPDATE site_localizations SET body=$3 WHERE site_id=$1 AND locale=$2;

-- name: SiteTechnology :many
WITH binding AS MATERIALIZED (
 SELECT b.id,b.site_id,b.effective_from,src.id AS source_id,src.target_id,src.external_key,src.namespace,
  consumer.epoch,consumer.generation
 FROM site_public_navigation n
 JOIN site_addresses a ON a.site_id=n.id AND a.active AND a.kind='primary'
 JOIN collection_bindings b ON b.site_id=n.id AND b.address_id=a.id AND b.purpose='primary'
  AND b.resource_type='origin' AND b.effective_until IS NULL AND b.effective_from<=statement_timestamp()
 JOIN collection_sources src ON src.id=b.source_id AND src.provider='web' AND src.resource_type='origin'
 JOIN collection_consumer consumer ON consumer.namespace=src.namespace
 WHERE n.public_id=$1 AND src.target_id IS NOT NULL
 ORDER BY b.effective_from DESC,b.id DESC LIMIT 1
), scopes AS MATERIALIZED (
 SELECT b.*,c.scope,c.scope_key,c.capability,c.output_schema,c.position,c.observed_at,c.confirmed_at,c.ruleset,c.facts
 FROM binding b
 JOIN collection_current c ON c.source_id=b.source_id AND c.generation=b.generation
 WHERE c.state='present' AND c.observation_id IS NOT NULL AND c.output_schema=1
  AND c.scope->>'namespace'=b.namespace AND c.scope->>'egress_region'~'^[a-z0-9][a-z0-9_-]{0,34}$'
  AND c.observed_at<=c.confirmed_at AND c.confirmed_at BETWEEN statement_timestamp()-interval '30 days' AND statement_timestamp()
  AND ((c.capability='web.http' AND c.scope#>>'{dimensions,section}' IN ('http','html','tls'))
   OR (c.capability='web.tls' AND c.scope#>>'{dimensions,section}'='tls'))
  AND NOT EXISTS (SELECT 1 FROM collection_gaps g WHERE g.namespace=b.namespace AND g.prior_epoch=b.epoch
   AND g.prior_generation=b.generation)
  AND EXISTS (
   SELECT 1 FROM collection_plan_bindings m JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id
   WHERE m.binding_id=b.id AND m.source_id=b.source_id AND p.capability=c.capability AND p.request_schema=1
    AND p.scope=c.scope-'namespace'-'dimensions' AND c.scope->'dimensions'=jsonb_build_object('section',c.scope#>>'{dimensions,section}')
    AND c.confirmed_at>=greatest(b.effective_from,m.attached_at)
  )
 ORDER BY c.confirmed_at DESC,c.capability,c.scope_key LIMIT 16
)
SELECT s.external_key,s.ruleset,s.observed_at,s.confirmed_at,
 f.fact_key::text AS fact_key,(s.facts->f.fact_key)::jsonb AS value,history.first_observed_at
FROM scopes s
CROSS JOIN LATERAL unnest(sqlc.arg(fact_keys)::text[]) AS f(fact_key)
LEFT JOIN LATERAL (
 SELECT e.first_observed_at FROM site_evidence_changes e
 WHERE e.site_id=s.site_id AND e.binding_id=s.id AND e.target_id=s.target_id AND e.capability=s.capability
  AND e.output_schema=s.output_schema AND e.scope_key=s.scope_key AND e.fact_key=f.fact_key
  AND e.epoch=s.epoch AND e.event_kind IN ('baseline','change') AND e.ruleset=s.ruleset AND e.value=s.facts->f.fact_key
  AND e.first_observed_at BETWEEN s.effective_from AND s.confirmed_at
 ORDER BY e.first_observed_at DESC,e.id DESC LIMIT 1
) history ON true
WHERE s.facts ? f.fact_key AND (
 (s.scope#>>'{dimensions,section}'='http' AND f.fact_key='http.response')
 OR (s.scope#>>'{dimensions,section}'='tls' AND f.fact_key='tls.certificate')
 OR (s.scope#>>'{dimensions,section}' IN ('http','html') AND f.fact_key LIKE 'tech.detected.%'))
ORDER BY s.confirmed_at DESC,s.capability,s.scope_key,fact_key;
