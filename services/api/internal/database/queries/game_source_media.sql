-- name: SaveGameSourceMedia :exec
WITH eligible AS MATERIALIZED (
 SELECT DISTINCT ON (gs.id,c.capability,c.output_schema,c.scope_key,coalesce(prior_exit.egress_region,c.execution_egress_region))
  gs.id AS game_source_id,src.id AS collection_source_id,b.id AS binding_id,src.target_id,src.namespace,
  c.capability,c.output_schema,c.scope->>'locale' AS locale,c.scope->>'region' AS store_country,
  coalesce(prior_exit.egress_region,c.execution_egress_region)::text AS egress_region,
  c.scope->>'egress_region' AS requested_egress_region,c.scope_key,c.position,c.received_at,c.state,c.observation_id,
  nullif(c.content_hash,'') AS content_hash,c.ruleset,c.observed_at,coalesce(c.confirmed_at,c.received_at) AS confirmed_at,
  CASE WHEN c.state<>'withdrawn' THEN c.data END AS payload
 FROM collection_current c
 JOIN collection_sources src ON src.id=c.source_id AND src.provider='steam' AND src.resource_type='app' AND src.target_id IS NOT NULL
 JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam' AND b.effective_until IS NULL
 JOIN game_sources gs ON gs.id=b.game_source_id AND gs.game_id=b.game_id AND gs.active AND gs.platform_code='steam' AND gs.external_key=src.external_key
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=c.capability
  AND p.output_schema=c.output_schema AND p.scope=c.scope-'namespace'-'dimensions'
 LEFT JOIN game_source_media prior_exit ON c.state='withdrawn' AND prior_exit.game_source_id=gs.id
  AND prior_exit.namespace=src.namespace AND prior_exit.capability=c.capability
  AND prior_exit.output_schema=c.output_schema AND prior_exit.scope_key=c.scope_key
 WHERE c.generation=sqlc.arg(generation)::uuid AND src.namespace=sqlc.arg(namespace)::text
  AND (c.execution_egress_region<>'' OR prior_exit.egress_region IS NOT NULL)
  AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id))
  AND (sqlc.narg(scope_key)::text IS NULL OR c.scope_key=sqlc.narg(scope_key))
  AND ((c.capability='steam.details' AND c.output_schema=2 AND c.scope->'dimensions'='{"section":"media"}')
    OR (c.capability='steam.assets' AND c.output_schema=1 AND c.scope->'dimensions'='{"section":"assets"}'))
  AND c.scope->>'namespace'=src.namespace AND c.scope->>'locale' IN ('en','zh-cn') AND c.scope->>'region' IN ('cn','us','hk')
  AND p.params->>'store_country'=upper(c.scope->>'region')
  AND p.params->>'language'=CASE c.scope->>'locale' WHEN 'en' THEN 'english' ELSE 'schinese' END
  AND coalesce(c.confirmed_at,c.received_at)>=greatest(b.effective_from,m.attached_at)
  AND (c.received_at>=greatest(b.effective_from,m.attached_at) OR NOT EXISTS (
   SELECT 1 FROM collection_bindings prior WHERE prior.game_source_id=b.game_source_id AND prior.effective_from<b.effective_from))
  AND (c.state='withdrawn' OR (c.state IN ('present','confirmed_empty') AND c.ruleset='gofurry-steam-media-2'
   AND c.observation_id IS NOT NULL AND c.data->>'app_id'=gs.external_key))
 ORDER BY gs.id,c.capability,c.output_schema,c.scope_key,coalesce(prior_exit.egress_region,c.execution_egress_region),b.effective_from DESC,c.position DESC,b.id DESC
)
INSERT INTO game_source_media AS old (game_source_id,collection_source_id,binding_id,target_id,namespace,capability,output_schema,
 locale,store_country,egress_region,requested_egress_region,scope_key,epoch,position,event_at,state,observation_id,content_hash,ruleset,observed_at,confirmed_at,payload)
SELECT e.game_source_id,e.collection_source_id,e.binding_id,e.target_id,e.namespace,e.capability,e.output_schema,
 e.locale,e.store_country,e.egress_region,e.requested_egress_region,e.scope_key,sqlc.arg(epoch)::uuid,e.position,e.received_at,e.state,e.observation_id,
 e.content_hash,e.ruleset,e.observed_at,e.confirmed_at,e.payload FROM eligible e
ORDER BY e.game_source_id,e.namespace,e.capability,e.output_schema,e.scope_key,e.egress_region
ON CONFLICT (game_source_id,namespace,capability,output_schema,scope_key,egress_region) DO UPDATE SET
 collection_source_id=excluded.collection_source_id,binding_id=excluded.binding_id,target_id=excluded.target_id,
 epoch=excluded.epoch,position=excluded.position,event_at=excluded.event_at,state=excluded.state,
 observation_id=CASE WHEN (excluded.state,excluded.content_hash,excluded.binding_id)=(old.state,old.content_hash,old.binding_id)
  THEN old.observation_id ELSE excluded.observation_id END,
 content_hash=excluded.content_hash,ruleset=excluded.ruleset,
 observed_at=CASE WHEN (excluded.state,excluded.content_hash,excluded.binding_id)=(old.state,old.content_hash,old.binding_id)
  THEN old.observed_at ELSE excluded.observed_at END,
 confirmed_at=excluded.confirmed_at,
 payload=CASE WHEN (excluded.state,excluded.content_hash,excluded.binding_id,excluded.ruleset)=(old.state,old.content_hash,old.binding_id,old.ruleset)
  THEN old.payload ELSE excluded.payload END
WHERE (excluded.epoch=old.epoch AND excluded.position>old.position)
 OR (excluded.epoch<>old.epoch AND (excluded.event_at>old.event_at
  OR (excluded.event_at=old.event_at AND excluded.observation_id=old.observation_id AND excluded.confirmed_at>old.confirmed_at)))
 OR (excluded.epoch=old.epoch AND excluded.position=old.position AND excluded.content_hash=old.content_hash
  AND excluded.confirmed_at>old.confirmed_at);

-- name: ListGameSourceMedia :many
SELECT page.* FROM game_sources s CROSS JOIN LATERAL (
 SELECT m.* FROM game_source_media m JOIN collection_bindings b ON b.id=m.binding_id AND b.game_source_id=m.game_source_id
 WHERE m.game_source_id=s.id AND b.game_id=s.game_id AND b.effective_until IS NULL AND m.state IN ('present','confirmed_empty')
  AND m.id>sqlc.arg(after_id)::uuid
 ORDER BY m.id LIMIT sqlc.arg(take_rows)
) page WHERE s.game_id=sqlc.arg(game_id) AND s.active ORDER BY page.id LIMIT sqlc.arg(take_rows);

-- name: GetGameSourceMediaForSelection :one
SELECT m.* FROM game_sources s JOIN game_source_media m ON m.game_source_id=s.id
 JOIN collection_bindings b ON b.id=m.binding_id AND b.game_source_id=s.id AND b.game_id=s.game_id
 WHERE s.game_id=sqlc.arg(game_id) AND s.id=sqlc.arg(game_source_id) AND s.active
 AND b.effective_until IS NULL AND m.state='present' AND m.observation_id=sqlc.arg(observation_id);

-- name: ReadGameDraftMediaV2 :many
SELECT locale,media FROM game_draft_media_v2 WHERE game_id=$1 ORDER BY locale;

-- name: ClearGameDraftMediaV2 :exec
DELETE FROM game_draft_media_v2 WHERE game_id=$1;

-- name: SaveGameDraftMediaV2 :exec
INSERT INTO game_draft_media_v2 (game_id,locale,media) VALUES ($1,$2,$3);
