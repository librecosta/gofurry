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
 LEFT JOIN collection_consumer consumer ON consumer.namespace=src.namespace
 WHERE n.public_id=$1 AND src.target_id IS NOT NULL
 ORDER BY b.effective_from DESC,b.id DESC LIMIT 1
), live AS MATERIALIZED (
 SELECT c.binding_id,c.generation,c.capability,c.scope_key,c.section,c.position,c.state,
  c.observed_at,c.last_observed_at,c.ruleset,c.facts
 FROM binding b CROSS JOIN LATERAL site_technology_scopes(b.generation,b.source_id,NULL,b.id)
  AS c(binding_id,generation,capability,scope_key,section,position,state,observed_at,last_observed_at,ruleset,facts)
 WHERE NOT EXISTS (SELECT 1 FROM collection_gaps g WHERE g.namespace=b.namespace AND g.prior_epoch=b.epoch
   AND g.prior_generation=b.generation)
), scopes AS MATERIALIZED (
 SELECT l.*,true AS verified_current FROM live l
 WHERE NOT EXISTS (
  SELECT 1 FROM site_technology_snapshots h
  WHERE h.binding_id=l.binding_id AND h.capability=l.capability AND h.scope_key=l.scope_key
   AND h.last_observed_at>l.last_observed_at)
 UNION ALL
 SELECT h.*,false AS verified_current FROM site_technology_snapshots h JOIN binding b ON h.binding_id=b.id
 WHERE NOT EXISTS (SELECT 1 FROM live l WHERE l.capability=h.capability AND l.scope_key=h.scope_key
  AND l.last_observed_at>=h.last_observed_at)
  AND h.last_observed_at BETWEEN b.effective_from AND statement_timestamp()
), recent AS MATERIALIZED (
 SELECT * FROM scopes ORDER BY last_observed_at DESC,capability,scope_key LIMIT 16
)
SELECT b.external_key,s.ruleset,s.observed_at,s.last_observed_at,
 (NOT s.verified_current OR s.last_observed_at<statement_timestamp()-interval '30 days')::boolean AS historical,
 f.fact_key::text AS fact_key,(s.facts->f.fact_key)::jsonb AS value,history.first_observed_at
FROM recent s JOIN binding b ON b.id=s.binding_id
CROSS JOIN LATERAL unnest(sqlc.arg(fact_keys)::text[]) AS f(fact_key)
LEFT JOIN LATERAL (
 SELECT e.first_observed_at FROM site_evidence_changes e
 WHERE e.site_id=b.site_id AND e.binding_id=b.id AND e.target_id=b.target_id AND e.capability=s.capability
  AND e.output_schema=1 AND e.scope_key=s.scope_key AND e.fact_key=f.fact_key
  AND e.event_kind IN ('baseline','change') AND e.ruleset=s.ruleset AND e.value=s.facts->f.fact_key
  AND e.first_observed_at BETWEEN b.effective_from AND s.last_observed_at
 ORDER BY e.first_observed_at DESC,e.id DESC LIMIT 1
) history ON true
WHERE s.state='present' AND s.facts ? f.fact_key AND (
 (s.section='http' AND f.fact_key='http.response')
 OR (s.section='tls' AND f.fact_key='tls.certificate')
 OR (s.section IN ('http','html') AND f.fact_key LIKE 'tech.detected.%'))
ORDER BY s.last_observed_at DESC,s.capability,s.scope_key,fact_key;
