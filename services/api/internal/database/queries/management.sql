-- name: ManagementSchemaReady :one
SELECT (to_regclass('game_draft_media') IS NOT NULL AND to_regclass('management_operations') IS NOT NULL
 AND to_regclass('steam_editorial_prefills') IS NOT NULL)::boolean;

-- name: RecordManagementOperation :exec
INSERT INTO management_operations (request_id,actor_id,actor_name,actor_role,action,target,revision)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: ManagementReceipt :one
SELECT action,target,revision,occurred_at FROM management_operations WHERE request_id=$1 AND actor_id=$2;

-- name: ManagementSites :many
SELECT s.public_id,s.slug,s.revision,s.status,s.content_rating,
 coalesce(s.primary_category,'')::text AS primary_category,
 coalesce((SELECT l.name FROM site_localizations l WHERE l.site_id=s.id ORDER BY l.locale DESC LIMIT 1),s.public_id)::text AS name
FROM sites s
WHERE s.public_id>sqlc.arg(after_id)::text AND (sqlc.arg(state)::text='' OR s.status=sqlc.arg(state))
 AND (sqlc.arg(rating)::text='' OR s.content_rating=sqlc.arg(rating))
 AND (NOT sqlc.arg(uncategorized)::boolean OR s.primary_category IS NULL)
 AND (sqlc.arg(keyword)::text='' OR s.public_id=sqlc.arg(keyword) OR EXISTS (
  SELECT 1 FROM site_localizations l WHERE l.site_id=s.id AND strpos(lower(l.name),lower(sqlc.arg(keyword)))>0)
  OR EXISTS (SELECT 1 FROM site_addresses a WHERE a.site_id=s.id AND a.active AND strpos(lower(a.url),lower(sqlc.arg(keyword)))>0))
ORDER BY s.public_id LIMIT sqlc.arg(take_rows)::integer;

-- name: ManagementSiteText :many
SELECT locale,name,summary,body FROM site_localizations WHERE site_id=$1 ORDER BY locale;

-- name: ManagementSiteAddresses :many
SELECT kind,url FROM site_addresses WHERE site_id=$1 AND active ORDER BY kind DESC,url;

-- name: ManagementGames :many
SELECT g.public_id,g.slug,g.revision,g.status,g.content_rating,
 coalesce((SELECT l.name FROM game_localizations l WHERE l.game_id=g.id ORDER BY l.locale DESC LIMIT 1),g.public_id)::text AS name
FROM games g
WHERE g.public_id>sqlc.arg(after_id)::text AND (sqlc.arg(state)::text='' OR g.status=sqlc.arg(state))
 AND (sqlc.arg(platform)::text='' OR EXISTS (
  SELECT 1 FROM game_sources s WHERE s.game_id=g.id AND s.active AND s.platform_code=sqlc.arg(platform)))
 AND (sqlc.arg(keyword)::text='' OR g.public_id=sqlc.arg(keyword) OR EXISTS (
  SELECT 1 FROM game_localizations l WHERE l.game_id=g.id AND strpos(lower(l.name),lower(sqlc.arg(keyword)))>0)
  OR EXISTS (SELECT 1 FROM game_sources s WHERE s.game_id=g.id AND s.active AND s.external_key=sqlc.arg(keyword)))
ORDER BY g.public_id LIMIT sqlc.arg(take_rows)::integer;

-- name: ManagementGame :one
SELECT * FROM games WHERE public_id=$1 FOR SHARE;

-- name: ManagementGameText :many
SELECT locale,name,summary,text_source_id,text_observation_id FROM game_localizations WHERE game_id=$1 ORDER BY locale;

-- name: ManagementGameSources :many
SELECT s.platform_code,s.external_key,s.source_url,s.active,
 EXISTS(SELECT 1 FROM collection_bindings b WHERE b.game_source_id=s.id AND b.effective_until IS NULL)::boolean AS bound,
 (SELECT m.observed_at FROM game_source_metadata m WHERE m.game_source_id=s.id AND m.payload IS NOT NULL
  ORDER BY m.observed_at DESC LIMIT 1)::timestamptz AS observed_at
FROM game_sources s WHERE s.game_id=$1 ORDER BY s.active DESC,s.platform_code,s.external_key LIMIT 32;

-- name: ManagementGameFacts :many
SELECT s.platform_code,s.external_key,m.locale,m.store_country,m.observed_at,
 coalesce(m.payload->>'name','')::text AS name,coalesce(m.payload->>'description','')::text AS summary,
 coalesce(m.payload->'content_rating','{}'::jsonb)::jsonb AS storefront_rating
FROM game_sources s JOIN LATERAL (
 SELECT m.* FROM game_source_metadata m WHERE m.game_source_id=s.id AND m.section='details' AND m.payload IS NOT NULL
 ORDER BY m.observed_at DESC,m.locale,m.store_country LIMIT 2
) m ON true WHERE s.game_id=$1
ORDER BY s.active DESC,s.platform_code,s.external_key,m.locale,m.store_country LIMIT 32;

-- name: ManagementCollections :many
SELECT c.public_id,c.slug,c.revision,c.status,
 coalesce((SELECT l->>'name' FROM jsonb_array_elements(c.localizations) l ORDER BY l->>'locale' DESC LIMIT 1),c.public_id)::text AS name
FROM game_collections c
WHERE c.public_id>sqlc.arg(after_id)::text AND (sqlc.arg(state)::text='' OR c.status=sqlc.arg(state))
 AND (sqlc.arg(keyword)::text='' OR c.public_id=sqlc.arg(keyword) OR EXISTS (
  SELECT FROM jsonb_array_elements(c.localizations) l WHERE strpos(lower(l->>'name'),lower(sqlc.arg(keyword)))>0))
ORDER BY c.public_id LIMIT sqlc.arg(take_rows)::integer;

-- name: ManagementOverviewCounts :many
SELECT 'site'::text AS kind,status,count(*)::bigint AS total FROM sites GROUP BY status
UNION ALL SELECT 'game',status,count(*) FROM games GROUP BY status
UNION ALL SELECT 'collection',status,count(*) FROM game_collections GROUP BY status;

-- name: ManagementRecentIntake :many
SELECT c.external_key,c.state,c.reason,c.updated_at,coalesce(g.public_id,'')::text AS public_id
FROM game_intake_candidates c LEFT JOIN games g ON g.id=c.game_id
WHERE c.state='accepted' ORDER BY c.updated_at DESC,c.id LIMIT 8;

-- name: ManagementIntakeRows :many
SELECT c.external_key,c.state,c.reason,c.discovered_via,c.next_at,c.updated_at,coalesce(g.public_id,'')::text AS public_id
FROM game_intake_candidates c LEFT JOIN games g ON g.id=c.game_id
WHERE c.platform_code='steam'
 AND (sqlc.arg(state)::text='' OR c.state=sqlc.arg(state))
 AND (sqlc.arg(app_id)::text='' OR c.external_key=sqlc.arg(app_id))
ORDER BY c.updated_at DESC,c.id LIMIT 40;

-- name: ManagementIntakeRecentCounts :many
SELECT state,count(*)::bigint AS total FROM game_intake_candidates
WHERE updated_at>=statement_timestamp()-interval '7 days' GROUP BY state;

-- name: ManagementCollectionState :one
SELECT c.ack_pending,(c.reset_ack IS NOT NULL)::boolean AS resetting,(c.generation IS NOT NULL)::boolean AS initialized,
 (SELECT r.confirmed_at FROM collection_current r WHERE r.generation=c.generation AND r.confirmed_at IS NOT NULL
  ORDER BY r.confirmed_at DESC LIMIT 1)::timestamptz AS confirmed_at,
 (SELECT max(g.detected_at) FROM collection_gaps g WHERE g.namespace=c.namespace)::timestamptz AS last_gap
FROM collection_consumer c WHERE c.namespace=$1;

-- name: ManagementCollectionPlans :many
SELECT p.capability,
 CASE WHEN p.post_started AND p.schedule_id IS NULL THEN 'uncertain'
 WHEN p.schedule_id IS NULL THEN 'pending'
 WHEN p.remote_enabled THEN 'enabled' ELSE 'disabled' END::text AS state,count(*)::bigint AS total
FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id WHERE s.namespace=$1
GROUP BY p.capability,2;
