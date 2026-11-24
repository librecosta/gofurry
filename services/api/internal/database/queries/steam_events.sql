-- name: EligibleSteamEventObservations :many
SELECT DISTINCT ON (gs.id,c.scope_key,c.execution_egress_region)
 gs.id AS game_source_id,src.id AS collection_source_id,b.id AS binding_id,src.target_id,src.namespace,
 c.scope_key,(c.scope->>'locale')::text AS requested_locale,c.execution_egress_region AS egress_region,
 c.observation_id,c.content_hash,c.observed_at,coalesce(c.confirmed_at,c.received_at) AS confirmed_at,c.data
FROM collection_current c
JOIN collection_sources src ON src.id=c.source_id AND src.provider='steam' AND src.resource_type='app' AND src.target_id IS NOT NULL
JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam' AND b.effective_until IS NULL
JOIN game_sources gs ON gs.id=b.game_source_id AND gs.game_id=b.game_id AND gs.active AND gs.platform_code='steam' AND gs.external_key=src.external_key
JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=c.capability
 AND p.output_schema=c.output_schema AND p.scope=c.scope-'namespace'-'dimensions'
WHERE c.generation=sqlc.arg(generation)::uuid AND src.namespace=sqlc.arg(namespace)::text
 AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id))
 AND (sqlc.arg(filter_scope)::text='' OR c.scope_key=sqlc.arg(filter_scope))
 AND (gs.id,c.scope_key,c.execution_egress_region) >
     (sqlc.arg(after_game_source)::uuid,sqlc.arg(after_scope)::text,sqlc.arg(after_egress)::text)
 AND c.capability='steam.events' AND c.output_schema=1 AND c.scope->'dimensions'='{"section":"events"}'
 AND c.scope->>'namespace'=src.namespace AND c.scope->>'locale' IN ('en','zh-cn') AND c.execution_egress_region<>''
 AND c.state='present' AND c.ruleset='gofurry-steam-events-1' AND c.observation_id IS NOT NULL AND c.data->>'app_id'=gs.external_key
 AND coalesce(c.confirmed_at,c.received_at)>=greatest(b.effective_from,m.attached_at)
 AND (c.received_at>=greatest(b.effective_from,m.attached_at) OR NOT EXISTS (
  SELECT 1 FROM collection_bindings prior WHERE prior.game_source_id=b.game_source_id AND prior.effective_from<b.effective_from))
ORDER BY gs.id,c.scope_key,c.execution_egress_region,b.effective_from DESC,c.position DESC,b.id DESC LIMIT 16;

-- name: UpsertSteamEvent :one
INSERT INTO game_steam_events AS e
 (id,game_source_id,collection_source_id,binding_id,target_id,namespace,scope_key,requested_locale,egress_region,
 observation_id,content_hash,observed_at,confirmed_at,event_gid,announcement_gid,language,headline,
 published_at,updated_at,eligible,body_hash,metadata,metrics)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
ON CONFLICT (game_source_id,egress_region,event_gid,language) DO UPDATE SET
 collection_source_id=excluded.collection_source_id,binding_id=excluded.binding_id,target_id=excluded.target_id,
 namespace=excluded.namespace,scope_key=excluded.scope_key,requested_locale=excluded.requested_locale,
 observation_id=excluded.observation_id,content_hash=excluded.content_hash,observed_at=excluded.observed_at,confirmed_at=excluded.confirmed_at,
 headline=excluded.headline,published_at=coalesce(excluded.published_at,e.published_at),updated_at=coalesce(excluded.updated_at,e.updated_at),
 eligible=excluded.eligible,body_hash=excluded.body_hash,metadata=excluded.metadata,metrics=e.metrics||excluded.metrics
WHERE e.announcement_gid=excluded.announcement_gid
 AND (coalesce(excluded.updated_at,e.updated_at,'-infinity'),excluded.observed_at,excluded.confirmed_at,excluded.observation_id)
  >= (coalesce(e.updated_at,'-infinity'),e.observed_at,e.confirmed_at,e.observation_id)
RETURNING id;

-- name: WriteSteamEventVersion :exec
INSERT INTO game_steam_event_versions
 (event_id,body_hash,headline,raw_body,html,plain_text,summary,observation_id,observed_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (event_id,body_hash) DO NOTHING;

-- name: BoundSteamEventVersions :exec
DELETE FROM game_steam_event_versions v WHERE v.event_id=$1 AND v.body_hash<>$2
 AND v.body_hash NOT IN (SELECT keep.body_hash FROM game_steam_event_versions keep WHERE keep.event_id=v.event_id AND keep.body_hash<>$2
 ORDER BY keep.observed_at DESC,keep.body_hash DESC LIMIT 9);

-- name: SteamEventOperationsPage :many
SELECT e.id,gs.external_key AS app_id,e.event_gid,e.announcement_gid,e.language,e.headline,e.published_at,e.updated_at,
 e.observed_at,e.hidden,e.eligible,e.revision,e.metrics,e.metadata
FROM games g JOIN game_sources gs ON gs.game_id=g.id AND gs.platform_code='steam'
JOIN game_steam_events e ON e.game_source_id=gs.id
WHERE g.public_id=sqlc.arg(public_id) AND e.id<sqlc.arg(before_id)::uuid
 AND (sqlc.arg(locale)::text='' OR CASE sqlc.arg(locale)::text WHEN 'en' THEN e.language=0 WHEN 'zh' THEN e.language=6 ELSE e.language NOT IN (0,6) END)
 AND (sqlc.arg(search)::text='' OR strpos(lower(e.headline),lower(sqlc.arg(search)::text))>0
      OR e.event_gid=sqlc.arg(search)::text OR e.announcement_gid=sqlc.arg(search)::text)
ORDER BY e.id DESC LIMIT 31;

-- name: SteamEventOperationsDetail :one
SELECT e.*,gs.external_key AS app_id,v.html,v.raw_body,v.plain_text,v.summary,
 (SELECT jsonb_build_object('headline',p.headline,'plain_text',p.plain_text,'observed_at',p.observed_at)
 FROM game_steam_event_versions p WHERE p.event_id=e.id AND p.body_hash<>e.body_hash ORDER BY p.observed_at DESC,p.body_hash DESC LIMIT 1) AS previous
FROM games g JOIN game_sources gs ON gs.game_id=g.id AND gs.platform_code='steam'
JOIN game_steam_events e ON e.game_source_id=gs.id
JOIN game_steam_event_versions v ON v.event_id=e.id AND v.body_hash=e.body_hash
WHERE g.public_id=$1 AND e.id=$2;

-- name: ChangeSteamEventVisibility :one
UPDATE game_steam_events e SET hidden=$3,revision=e.revision+1
FROM games g JOIN game_sources gs ON gs.game_id=g.id AND gs.platform_code='steam'
WHERE g.public_id=$1 AND e.id=$2 AND e.game_source_id=gs.id AND e.revision=$4
RETURNING e.revision;

-- name: PublicSteamEventDetail :one
SELECT e.id,gs.external_key AS app_id,e.announcement_gid,e.language,e.headline,e.published_at,e.updated_at,
 v.html,v.summary,e.metrics,ARRAY(SELECT jsonb_array_elements_text(CASE
 WHEN jsonb_typeof(e.metadata->'tags')='array' THEN e.metadata->'tags' ELSE '[]'::jsonb END))::text[] AS tags
FROM game_channel_public g JOIN game_sources gs ON gs.game_id=g.id AND gs.platform_code='steam' AND gs.active
JOIN game_steam_events e ON e.game_source_id=gs.id
JOIN game_steam_event_versions v ON v.event_id=e.id AND v.body_hash=e.body_hash
WHERE g.public_id=$1 AND e.id=$2 AND e.egress_region=$3 AND NOT e.hidden AND e.eligible AND e.published_at<=transaction_timestamp();
