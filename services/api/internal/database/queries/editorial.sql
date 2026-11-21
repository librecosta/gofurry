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
FROM game_editorial_tag_categories
WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL)
 AND (strpos(code,lower(sqlc.arg(keyword)::text))>0 OR EXISTS (
  SELECT 1 FROM jsonb_array_elements(localizations) l WHERE strpos(lower(l->>'name'),lower(sqlc.arg(keyword)))>0)
  OR sqlc.arg(keyword)='')
ORDER BY sort_order,code LIMIT 128;

-- name: ListManagedEditorialCategories :many
SELECT c.code,c.localizations,c.sort_order,(c.archived_at IS NOT NULL)::boolean AS archived
FROM game_editorial_tag_categories c
WHERE (sqlc.arg(cursor_code)::text='' OR (c.sort_order,c.code) >
 (SELECT sort_order,code FROM game_editorial_tag_categories WHERE code=sqlc.arg(cursor_code)))
 AND (sqlc.arg(state)::text='' OR (c.archived_at IS NOT NULL)=(sqlc.arg(state)='archived'))
 AND (strpos(c.code,lower(sqlc.arg(keyword)::text))>0 OR EXISTS (
  SELECT 1 FROM jsonb_array_elements(c.localizations) l WHERE strpos(lower(l->>'name'),lower(sqlc.arg(keyword)))>0)
  OR sqlc.arg(keyword)='')
ORDER BY c.sort_order,c.code LIMIT 31;

-- name: ListManagedEditorialTags :many
SELECT t.code,c.code AS category,t.localizations,(t.archived_at IS NOT NULL)::boolean AS archived,
 (c.archived_at IS NOT NULL)::boolean AS category_archived,
 coalesce((SELECT l->>'name' FROM jsonb_array_elements(c.localizations) l
  ORDER BY (l->>'locale'='zh') DESC LIMIT 1),c.code)::text AS category_name
FROM game_editorial_tags t JOIN game_editorial_tag_categories c ON c.id=t.category_id
WHERE t.code>sqlc.arg(cursor_code)::text
 AND (sqlc.arg(category)::text='' OR c.code=sqlc.arg(category))
 AND (sqlc.arg(state)::text='' OR (t.archived_at IS NOT NULL)=(sqlc.arg(state)='archived'))
 AND (strpos(t.code,lower(sqlc.arg(keyword)::text))>0 OR EXISTS (
  SELECT 1 FROM jsonb_array_elements(t.localizations) l WHERE strpos(lower(l->>'name'),lower(sqlc.arg(keyword)))>0)
  OR sqlc.arg(keyword)='')
ORDER BY t.code LIMIT 31;

-- name: LockEditorialDefinitions :exec
SELECT pg_advisory_xact_lock(hashtextextended('gofurry/editorial-tags',0));

-- name: LockEditorialCategory :one
SELECT * FROM game_editorial_tag_categories WHERE code=$1 FOR UPDATE;

-- name: LockEditorialTag :one
SELECT * FROM game_editorial_tags WHERE code=$1 FOR UPDATE;

-- name: EditorialCategoryHasTags :one
SELECT EXISTS (SELECT 1 FROM game_editorial_tags WHERE category_id=$1 AND archived_at IS NULL)::boolean;

-- name: EditorialTagIsReferenced :one
SELECT (EXISTS (SELECT 1 FROM game_editorial_tag_members WHERE tag_id=sqlc.arg(tag_id)::uuid)
 OR EXISTS (SELECT 1 FROM game_collection_tag_rules WHERE tag_id=sqlc.arg(tag_id)::uuid))::boolean;

-- name: CreateEditorialCategory :one
INSERT INTO game_editorial_tag_categories (id,code,localizations,sort_order)
SELECT sqlc.arg(id)::uuid,sqlc.arg(code)::text,sqlc.arg(names)::jsonb,sqlc.arg(sort_order)::integer
WHERE NOT sqlc.arg(check_names)::boolean OR NOT EXISTS (
 SELECT 1 FROM game_editorial_tag_categories c
 WHERE c.localizations @> sqlc.arg(names) AND sqlc.arg(names) @> c.localizations)
ON CONFLICT (code) DO NOTHING RETURNING game_editorial_tag_categories.code;

-- name: UpdateEditorialCategory :exec
UPDATE game_editorial_tag_categories SET localizations=$2,sort_order=$3 WHERE id=$1;

-- name: SetEditorialCategoryArchived :exec
UPDATE game_editorial_tag_categories
SET archived_at=CASE WHEN sqlc.arg(archived)::boolean THEN now() ELSE NULL END WHERE id=$1;

-- name: CreateEditorialTag :one
INSERT INTO game_editorial_tags (id,category_id,code,localizations)
SELECT sqlc.arg(tag_id)::uuid,sqlc.arg(category_id)::uuid,sqlc.arg(tag_code)::text,sqlc.arg(names)::jsonb
WHERE NOT sqlc.arg(check_names)::boolean OR NOT EXISTS (
 SELECT 1 FROM game_editorial_tags t WHERE t.category_id=sqlc.arg(category_id)
 AND t.localizations @> sqlc.arg(names) AND sqlc.arg(names) @> t.localizations)
ON CONFLICT (code) DO NOTHING RETURNING game_editorial_tags.code;

-- name: UpdateEditorialTag :exec
UPDATE game_editorial_tags SET category_id=$2,localizations=$3 WHERE id=$1;

-- name: SetEditorialTagArchived :exec
UPDATE game_editorial_tags
SET archived_at=CASE WHEN sqlc.arg(archived)::boolean THEN now() ELSE NULL END WHERE id=$1;

-- name: EditorialTagCategoryArchived :one
SELECT (archived_at IS NOT NULL)::boolean FROM game_editorial_tag_categories WHERE id=$1;

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
