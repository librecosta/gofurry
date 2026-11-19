-- name: SaveSteamMetadata :exec
WITH eligible AS MATERIALIZED (
 SELECT DISTINCT ON (b.game_source_id,locale,store_country,egress_region,section) b.game_source_id,b.id AS binding_id,
  c.scope->>'locale' AS locale,c.scope->>'region' AS store_country,c.scope->>'egress_region' AS egress_region,
  c.scope#>>'{dimensions,section}' AS section,c.scope_key,c.position,c.received_at,c.state,c.output_schema,c.ruleset,
  c.observation_id,nullif(c.content_hash,'') AS content_hash,c.observed_at,coalesce(c.confirmed_at,c.received_at) AS confirmed_at,
  CASE WHEN c.state='present' THEN c.data-ARRAY['app_id','tier','source_url'] END AS payload
 FROM collection_current c
 JOIN collection_sources src ON src.id=c.source_id AND src.provider='steam' AND src.resource_type='app' AND src.target_id IS NOT NULL
 JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam' AND b.resource_type='app' AND b.effective_until IS NULL
 JOIN game_sources gs ON gs.id=b.game_source_id AND gs.game_id=b.game_id AND gs.active AND gs.platform_code='steam' AND gs.external_key=src.external_key
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=c.capability AND p.scope=c.scope-'namespace'-'dimensions'
 WHERE c.generation=sqlc.arg(generation)::uuid AND c.capability='steam.details' AND p.request_schema=1 AND c.output_schema=1
  AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id))
  AND (sqlc.narg(scope_key)::text IS NULL OR c.scope_key=sqlc.narg(scope_key))
  AND c.scope->>'namespace'=src.namespace
  AND c.scope->>'locale' IN ('en','zh-cn') AND c.scope->>'region' IN ('cn','us','hk')
  AND c.scope#>>'{dimensions,section}' IN ('details','media')
  AND c.scope->'dimensions'=jsonb_build_object('section',c.scope#>>'{dimensions,section}')
  AND p.params->>'store_country'=upper(c.scope->>'region')
  AND p.params->>'language'=CASE c.scope->>'locale' WHEN 'en' THEN 'english' ELSE 'schinese' END
  AND coalesce(c.confirmed_at,c.received_at)>=greatest(b.effective_from,m.attached_at)
  AND (c.received_at>=greatest(b.effective_from,m.attached_at) OR NOT EXISTS (
   SELECT 1 FROM collection_bindings prior WHERE prior.game_source_id=b.game_source_id AND prior.effective_from<b.effective_from))
  AND (c.state='withdrawn' OR (c.state='present' AND c.ruleset='gofurry-steam-1' AND c.observation_id IS NOT NULL
   AND c.data->>'app_id'=gs.external_key AND c.data->>'tier'='observed_storefront'
   AND c.data->>'source_url'='https://store.steampowered.com/api/appdetails'))
 ORDER BY b.game_source_id,locale,store_country,egress_region,section,b.effective_from DESC,c.position DESC,b.id DESC
)

 INSERT INTO game_source_metadata AS old (game_source_id,locale,store_country,egress_region,section,binding_id,scope_key,
  epoch,position,event_at,state,output_schema,ruleset,observation_id,content_hash,observed_at,confirmed_at,payload)
 SELECT e.game_source_id,e.locale,e.store_country,e.egress_region,e.section,e.binding_id,e.scope_key,
  sqlc.arg(epoch)::uuid,e.position,e.received_at,e.state,e.output_schema,e.ruleset,e.observation_id,e.content_hash,
  CASE WHEN e.payload IS NOT NULL THEN e.observed_at END,CASE WHEN e.payload IS NOT NULL THEN e.confirmed_at END,e.payload
 FROM eligible e
 ORDER BY e.game_source_id,e.locale,e.store_country,e.egress_region,e.section
 ON CONFLICT (game_source_id,locale,store_country,egress_region,section) DO UPDATE SET
  binding_id=CASE WHEN excluded.payload IS NOT NULL THEN excluded.binding_id ELSE old.binding_id END,
  scope_key=CASE WHEN excluded.payload IS NOT NULL THEN excluded.scope_key ELSE old.scope_key END,
  epoch=excluded.epoch,position=excluded.position,event_at=excluded.event_at,state=excluded.state,
  ruleset=CASE WHEN excluded.payload IS NOT NULL THEN excluded.ruleset ELSE old.ruleset END,
  observation_id=CASE WHEN excluded.payload IS NOT NULL AND (excluded.payload IS DISTINCT FROM old.payload OR excluded.binding_id<>old.binding_id)
   THEN excluded.observation_id ELSE old.observation_id END,
  content_hash=CASE WHEN excluded.payload IS NOT NULL AND (excluded.payload IS DISTINCT FROM old.payload OR excluded.binding_id<>old.binding_id)
   THEN excluded.content_hash ELSE old.content_hash END,
  observed_at=CASE WHEN excluded.payload IS NOT NULL AND (excluded.payload IS DISTINCT FROM old.payload OR excluded.binding_id<>old.binding_id)
   THEN excluded.observed_at ELSE old.observed_at END,
  confirmed_at=greatest(old.confirmed_at,excluded.confirmed_at),payload=coalesce(excluded.payload,old.payload)
 WHERE ((excluded.epoch=old.epoch AND excluded.position>old.position)
  OR (excluded.epoch<>old.epoch AND excluded.event_at>old.event_at)
  OR (excluded.epoch=old.epoch AND excluded.position=old.position AND old.state='present' AND excluded.state='present'
   AND excluded.confirmed_at>old.confirmed_at))
  -- A sparse partial description cannot erase a usable last success.
 AND NOT (excluded.state='present' AND old.section='details'
  AND coalesce(old.payload->>'description','')<>'' AND coalesce(excluded.payload->>'description','')='');
