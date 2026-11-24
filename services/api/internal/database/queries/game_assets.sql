-- name: PutGameAssets :exec
INSERT INTO game_assets (game_id,game_source_id,platform_code,external_key,kind,category,source_family,source_role,source_identity)
SELECT s.game_id,s.id,s.platform_code,sqlc.arg(external_key),x.kind,x.category,sqlc.arg(source_family),x.role,x.identity
FROM game_sources s CROSS JOIN jsonb_to_recordset(sqlc.arg(items)::jsonb)
 AS x(kind text,category text,role text,identity text)
WHERE s.id=sqlc.arg(game_source_id)
ON CONFLICT (game_source_id,external_key,source_family,source_role,source_identity) DO NOTHING;

-- name: PutGameAssetVersions :exec
INSERT INTO game_asset_versions (game_id,asset_id,binding_id,observation_id,scope_key,egress_region,locale,origin,name,original_order,highlight,truncated)
SELECT a.game_id,a.id,sqlc.arg(binding_id),sqlc.arg(observation_id),sqlc.arg(scope_key),sqlc.arg(egress_region),sqlc.arg(locale),
 sqlc.arg(origin),x.name,x.original_order,x.highlight,sqlc.narg(truncated)
FROM jsonb_to_recordset(sqlc.arg(items)::jsonb) AS x(role text,identity text,name text,original_order int,highlight bool)
JOIN game_assets a ON a.game_source_id=sqlc.arg(game_source_id) AND a.external_key=sqlc.arg(external_key)
 AND a.source_family=sqlc.arg(source_family) AND a.source_role=x.role AND a.source_identity=x.identity
ON CONFLICT (asset_id,binding_id,observation_id,scope_key,egress_region) DO NOTHING;

-- name: PutGameAssetVariants :many
INSERT INTO game_asset_variants (version_id,variant_key,url,format,source_role,source_asset_id,source_slot,source_digest,source_family,source_order)
SELECT v.id,y.key,y.url,nullif(y.format,''),y.role,y.source_id,y.slot,y.digest,y.family,y.source_order
FROM jsonb_to_recordset(sqlc.arg(items)::jsonb) AS x(role text,identity text,variants jsonb)
JOIN game_assets a ON a.game_source_id=sqlc.arg(game_source_id) AND a.external_key=sqlc.arg(external_key)
 AND a.source_family=sqlc.arg(source_family) AND a.source_role=x.role AND a.source_identity=x.identity
JOIN game_asset_versions v ON v.asset_id=a.id AND v.binding_id=sqlc.arg(binding_id)
 AND v.observation_id=sqlc.arg(observation_id) AND v.scope_key=sqlc.arg(scope_key) AND v.egress_region=sqlc.arg(egress_region)
CROSS JOIN jsonb_to_recordset(x.variants)
 AS y(key text,url text,format text,role text,source_id text,slot text,digest text,family text,source_order int)
ON CONFLICT (version_id,variant_key) DO NOTHING
RETURNING id;

-- name: CountGameAssetNormalization :one
SELECT count(*)::int FROM jsonb_to_recordset(sqlc.arg(items)::jsonb) AS x(role text,identity text,variants jsonb)
JOIN game_assets a ON a.game_source_id=sqlc.arg(game_source_id) AND a.external_key=sqlc.arg(external_key)
 AND a.source_family=sqlc.arg(source_family) AND a.source_role=x.role AND a.source_identity=x.identity
JOIN game_asset_versions v ON v.asset_id=a.id AND v.binding_id=sqlc.arg(binding_id)
 AND v.observation_id=sqlc.arg(observation_id) AND v.scope_key=sqlc.arg(scope_key) AND v.egress_region=sqlc.arg(egress_region)
CROSS JOIN jsonb_to_recordset(x.variants) AS y(key text,url text,source_id text,role text)
JOIN game_asset_variants t ON t.version_id=v.id AND t.variant_key=y.key AND t.url=y.url AND t.source_asset_id=y.source_id AND t.source_role=y.role;

-- name: GameAssetSteamBinding :one
SELECT gs.game_id,gs.external_key FROM game_sources gs
JOIN collection_bindings b ON b.game_id=gs.game_id AND b.game_source_id=gs.id AND b.purpose='steam'
JOIN collection_sources cs ON cs.id=b.source_id AND cs.external_key=gs.external_key AND cs.resource_type='app' AND cs.provider='steam'
WHERE gs.id=sqlc.arg(game_source_id) AND gs.platform_code='steam' AND b.id=sqlc.arg(binding_id)
 AND cs.id=sqlc.arg(collection_source_id) AND cs.target_id=sqlc.arg(target_id) AND cs.namespace=sqlc.arg(namespace);

-- name: FindGameAssetVersion :one
SELECT v.id FROM game_assets a JOIN game_asset_versions v ON v.asset_id=a.id
WHERE a.game_source_id=sqlc.arg(game_source_id) AND a.external_key=sqlc.arg(external_key) AND a.source_family=sqlc.arg(source_family)
 AND a.source_role=sqlc.arg(source_role) AND a.source_identity=sqlc.arg(source_identity)
 AND v.binding_id=sqlc.arg(binding_id) AND v.observation_id=sqlc.arg(observation_id)
 AND v.scope_key=sqlc.arg(scope_key) AND v.egress_region=sqlc.arg(egress_region);

-- name: GameAssetBackfillSources :many
SELECT m.* FROM game_source_media m
WHERE m.id>sqlc.arg(after_id)::uuid AND m.state='present' AND m.ruleset='gofurry-steam-media-2'
ORDER BY m.id LIMIT 24;

-- name: GameAssetBackfillGames :many
SELECT g.id,g.public_id FROM games g WHERE g.id>sqlc.arg(after_id)::uuid
AND (EXISTS (SELECT 1 FROM game_draft_media_v2 d WHERE d.game_id=g.id)
 OR EXISTS (SELECT 1 FROM game_published_localizations p WHERE p.game_id=g.id AND p.media_v2 IS NOT NULL))
ORDER BY g.id LIMIT 24;

-- name: GameAssetBackfillSnapshots :many
SELECT d.locale,'legacy_draft'::text AS surface,0::bigint AS revision,d.media
FROM game_draft_media_v2 d WHERE d.game_id=sqlc.arg(game_id)
UNION ALL
SELECT p.locale,'legacy_published',p.revision,p.media_v2 FROM game_published_localizations p
WHERE p.game_id=sqlc.arg(game_id) AND p.media_v2 IS NOT NULL;

-- name: ListGameAssetVersions :many
SELECT sqlc.embed(a),sqlc.embed(v),l.selected_at,
 (s.active AND b.effective_until IS NULL AND EXISTS (
   SELECT 1 FROM game_source_media m WHERE m.game_source_id=a.game_source_id AND m.binding_id=v.binding_id AND m.observation_id=v.observation_id
    AND m.scope_key=v.scope_key AND m.egress_region=v.egress_region AND m.state='present'
 ))::boolean AS current,
 coalesce((SELECT t.url FROM game_asset_variants t WHERE t.version_id=v.id
  AND t.variant_key IN ('standard','2x','thumbnail','full','poster')
  ORDER BY CASE t.variant_key WHEN 'standard' THEN 0 WHEN 'thumbnail' THEN 1 WHEN 'poster' THEN 2 ELSE 3 END,t.variant_key LIMIT 1),'')::text AS preview_url
FROM game_asset_versions v JOIN game_assets a ON a.id=v.asset_id JOIN game_sources s ON s.id=a.game_source_id
JOIN collection_bindings b ON b.id=v.binding_id
LEFT JOIN game_asset_library l ON l.game_id=v.game_id AND l.version_id=v.id
WHERE v.game_id=sqlc.arg(game_id) AND v.id>sqlc.arg(after_id)::uuid
 AND (NOT sqlc.arg(library_only)::boolean OR l.version_id IS NOT NULL)
 AND (sqlc.arg(platform)::text='' OR a.platform_code=sqlc.arg(platform))
 AND (sqlc.arg(kind)::text='' OR a.kind=sqlc.arg(kind))
 AND (sqlc.arg(category)::text='' OR a.category=sqlc.arg(category))
 AND (sqlc.arg(locale)::text='' OR v.locale=sqlc.arg(locale))
 AND (sqlc.arg(source_role)::text='' OR a.source_role=sqlc.arg(source_role) OR EXISTS (
   SELECT 1 FROM game_asset_variants t WHERE t.version_id=v.id AND t.source_role=sqlc.arg(source_role)))
 AND (sqlc.narg(asset_id)::uuid IS NULL OR a.id=sqlc.narg(asset_id))
ORDER BY v.id LIMIT sqlc.arg(take_rows);

-- name: ReadGameAssetVersion :one
SELECT sqlc.embed(a),sqlc.embed(v),l.selected_at,
 (s.active AND b.effective_until IS NULL AND EXISTS (
   SELECT 1 FROM game_source_media m WHERE m.game_source_id=a.game_source_id AND m.binding_id=v.binding_id AND m.observation_id=v.observation_id
    AND m.scope_key=v.scope_key AND m.egress_region=v.egress_region AND m.state='present'
 ))::boolean AS current
FROM game_asset_versions v JOIN game_assets a ON a.id=v.asset_id JOIN game_sources s ON s.id=a.game_source_id
JOIN collection_bindings b ON b.id=v.binding_id
LEFT JOIN game_asset_library l ON l.game_id=v.game_id AND l.version_id=v.id
WHERE v.game_id=sqlc.arg(game_id) AND a.id=sqlc.arg(asset_id) AND v.id=sqlc.arg(version_id);

-- name: ListGameAssetVariants :many
SELECT * FROM game_asset_variants WHERE version_id=$1 ORDER BY variant_key;

-- name: AddGameAssetLibrary :execrows
INSERT INTO game_asset_library (game_id,version_id,selected_by,selection_origin)
SELECT v.game_id,v.id,sqlc.narg(selected_by),sqlc.arg(selection_origin)
FROM game_asset_versions v WHERE v.id=sqlc.arg(version_id) AND v.game_id=sqlc.arg(game_id)
ON CONFLICT DO NOTHING;

-- name: RemoveGameAssetLibrary :execrows
DELETE FROM game_asset_library WHERE game_id=$1 AND version_id=$2;

-- name: ReadGameAssetPresentation :one
SELECT * FROM game_asset_presentations WHERE game_id=$1 AND locale=$2 AND surface=$3;

-- name: ListGameAssetPresentation :many
SELECT i.slot,i.position,v.asset_id,i.version_id,i.variant_key
FROM game_asset_presentation_items i JOIN game_asset_versions v ON v.id=i.version_id
WHERE i.game_id=sqlc.arg(game_id) AND i.locale=sqlc.arg(locale) AND i.surface=sqlc.arg(surface)
 AND i.slot=sqlc.arg(slot) AND i.position>sqlc.arg(after_position)::bigint
ORDER BY i.position LIMIT sqlc.arg(take_rows);

-- name: EnsureGameAssetPresentation :exec
INSERT INTO game_asset_presentations (game_id,locale,surface,revision) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: ReviseGameAssetPresentation :exec
UPDATE game_asset_presentations SET revision=$4 WHERE game_id=$1 AND locale=$2 AND surface=$3;

-- name: SetGameAssetPresentationItem :execrows
INSERT INTO game_asset_presentation_items (game_id,locale,surface,slot,position,version_id,variant_key)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (game_id,locale,surface,slot,position) DO UPDATE
SET version_id=excluded.version_id,variant_key=excluded.variant_key
WHERE (game_asset_presentation_items.version_id,game_asset_presentation_items.variant_key)
 IS DISTINCT FROM (excluded.version_id,excluded.variant_key);

-- name: RemoveGameAssetPresentationItem :execrows
DELETE FROM game_asset_presentation_items WHERE game_id=$1 AND locale=$2 AND surface=$3 AND slot=$4 AND position=$5;

-- name: GameAssetPresentationPick :one
SELECT a.kind, EXISTS (SELECT 1 FROM game_asset_library l WHERE l.game_id=v.game_id AND l.version_id=v.id)::boolean AS selected,
 (sqlc.narg(variant_key)::text IS NULL OR EXISTS (
  SELECT 1 FROM game_asset_variants t WHERE t.version_id=v.id AND t.variant_key=sqlc.narg(variant_key)))::boolean AS variant_valid
FROM game_asset_versions v JOIN game_assets a ON a.id=v.asset_id WHERE v.id=sqlc.arg(version_id) AND v.game_id=sqlc.arg(game_id);

-- name: GameAssetPresentationChanged :one
WITH drafts AS (
 SELECT p.locale,i.slot,i.position,i.version_id,i.variant_key
 FROM game_asset_presentations p LEFT JOIN game_asset_presentation_items i USING(game_id,locale,surface)
 WHERE p.game_id=$1 AND p.surface='draft'
), published AS (
 SELECT p.locale,i.slot,i.position,i.version_id,i.variant_key
 FROM game_asset_presentations p LEFT JOIN game_asset_presentation_items i USING(game_id,locale,surface)
 WHERE p.game_id=$1 AND p.surface='published'
)
SELECT EXISTS ((SELECT * FROM drafts EXCEPT SELECT * FROM published)
 UNION ALL (SELECT * FROM published EXCEPT SELECT * FROM drafts));

-- name: ClearPublishedGameAssetPresentation :exec
DELETE FROM game_asset_presentations WHERE game_id=$1 AND surface='published';

-- name: SnapshotGameAssetPresentation :exec
INSERT INTO game_asset_presentations (game_id,locale,surface,revision)
SELECT p.game_id,p.locale,'published',sqlc.arg(revision) FROM game_asset_presentations p WHERE p.game_id=sqlc.arg(game_id) AND p.surface='draft';

-- name: SnapshotGameAssetPresentationItems :exec
INSERT INTO game_asset_presentation_items (game_id,locale,surface,slot,position,version_id,variant_key)
SELECT i.game_id,i.locale,'published',i.slot,i.position,i.version_id,i.variant_key
FROM game_asset_presentation_items i WHERE i.game_id=$1 AND surface='draft';
