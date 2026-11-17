-- name: SearchEditorialTags :many
SELECT t.code, c.code AS category,
 coalesce((SELECT l->>'name' FROM jsonb_array_elements(t.localizations) l
  ORDER BY (l->>'locale'='zh') DESC LIMIT 1),t.code)::text AS name
FROM game_editorial_tags t JOIN game_editorial_tag_categories c ON c.id=t.category_id
WHERE t.archived_at IS NULL AND c.archived_at IS NULL AND
 (strpos(t.code,lower(sqlc.arg(keyword)::text))>0 OR EXISTS (
  SELECT 1 FROM jsonb_array_elements(t.localizations) l WHERE strpos(lower(l->>'name'),lower(sqlc.arg(keyword)))>0)
  OR sqlc.arg(keyword)='')
ORDER BY c.sort_order,c.code,t.code LIMIT 30;

-- name: ListEditorialTagCategories :many
SELECT code,coalesce((SELECT l->>'name' FROM jsonb_array_elements(localizations) l
 ORDER BY (l->>'locale'='zh') DESC LIMIT 1),code)::text AS name
FROM game_editorial_tag_categories WHERE archived_at IS NULL ORDER BY sort_order,code LIMIT 128;

-- name: CreateEditorialTag :one
INSERT INTO game_editorial_tags (id,category_id,code,localizations)
SELECT sqlc.arg(tag_id)::uuid,c.id,sqlc.arg(tag_code)::text,sqlc.arg(names)::jsonb
FROM game_editorial_tag_categories c WHERE c.code=sqlc.arg(category_code) AND c.archived_at IS NULL
RETURNING game_editorial_tags.code;

-- name: ArchiveEditorialTag :one
UPDATE game_editorial_tags SET archived_at=now() WHERE code=$1 AND archived_at IS NULL RETURNING code;

-- name: ManagementGameTags :many
SELECT t.code, a.role, (t.archived_at IS NOT NULL OR c.id IS NULL OR c.archived_at IS NOT NULL) AS archived,
 coalesce((SELECT l->>'name' FROM jsonb_array_elements(t.localizations) l
  ORDER BY (l->>'locale'='zh') DESC LIMIT 1),t.code)::text AS name
FROM game_editorial_tag_members a JOIN game_editorial_tags t ON t.id=a.tag_id
LEFT JOIN game_editorial_tag_categories c ON c.id=t.category_id
WHERE a.game_id=$1 ORDER BY a.role,t.code;

-- name: ManagementCollectionTags :many
SELECT t.code, (t.archived_at IS NOT NULL OR c.id IS NULL OR c.archived_at IS NOT NULL) AS archived,
 coalesce((SELECT l->>'name' FROM jsonb_array_elements(t.localizations) l
  ORDER BY (l->>'locale'='zh') DESC LIMIT 1),t.code)::text AS name
FROM game_collection_tag_rules r JOIN game_editorial_tags t ON t.id=r.tag_id
LEFT JOIN game_editorial_tag_categories c ON c.id=t.category_id
WHERE r.collection_id=$1 ORDER BY t.code;

-- name: ManagementCollectionExclusions :many
SELECT g.public_id,coalesce((SELECT name FROM game_channel_locales l
 WHERE l.game_id=g.id ORDER BY l.locale DESC LIMIT 1),g.public_id)::text AS name
FROM game_collection_exclusions e JOIN games g ON g.id=e.game_id
WHERE e.collection_id=$1 ORDER BY g.public_id;

-- name: ResolveEditorialTag :one
SELECT t.id FROM game_editorial_tags t JOIN game_editorial_tag_categories c ON c.id=t.category_id
WHERE t.code=$1 AND t.archived_at IS NULL AND c.archived_at IS NULL;

-- name: RemoveGameEditorialMember :exec
DELETE FROM game_editorial_tag_members a USING game_editorial_tags t
WHERE a.game_id=sqlc.arg(game_id) AND a.tag_id=t.id AND t.code=sqlc.arg(code);

-- name: RemoveGameCollectionRule :exec
DELETE FROM game_collection_tag_rules r USING game_editorial_tags t
WHERE r.collection_id=sqlc.arg(collection_id) AND r.tag_id=t.id AND t.code=sqlc.arg(code);

-- name: RemoveGameCollectionExclusion :exec
DELETE FROM game_collection_exclusions e USING games g
WHERE e.collection_id=sqlc.arg(collection_id) AND e.game_id=g.id AND g.public_id=sqlc.arg(public_id);

-- name: ResolveEditorialGame :one
SELECT id FROM games WHERE public_id=$1;
