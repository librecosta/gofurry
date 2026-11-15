-- name: RegisterLegacyImportBatch :exec
INSERT INTO legacy_import_batches (plan_sha256,legacy_deployment,backup_sha256,content_sha256,decisions_sha256,rule_version)
VALUES ($1,$2,$3,$4,$5,1) ON CONFLICT DO NOTHING;

-- name: FindLegacyImportMapping :one
SELECT target_id,public_id_at_link,origin,first_plan_sha256 FROM legacy_editorial_mappings
WHERE legacy_deployment=$1 AND kind=$2 AND legacy_id=$3;

-- name: LegacyImportTargetOwners :one
SELECT count(*) FROM legacy_editorial_mappings
WHERE legacy_deployment=$1 AND kind=$2 AND target_id=$3;

-- name: AddLegacySiteMapping :exec
INSERT INTO legacy_site_ids (legacy_deployment,legacy_id,target_id,public_id_at_link,first_plan_sha256,origin,decision_sha256,verified_at,verified_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,transaction_timestamp(),sqlc.arg(verified_by)::text);

-- name: AddLegacyGameMapping :exec
INSERT INTO legacy_game_ids (legacy_deployment,legacy_id,target_id,public_id_at_link,first_plan_sha256,origin,decision_sha256,verified_at,verified_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,transaction_timestamp(),sqlc.arg(verified_by)::text);

-- name: AddLegacyEditorialMapping :exec
INSERT INTO legacy_editorial_ids (legacy_deployment,kind,legacy_id,target_id,public_id_at_link,first_plan_sha256,origin,decision_sha256,verified_at,verified_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,transaction_timestamp(),sqlc.arg(verified_by)::text);

-- name: ReadLegacyImportReceipt :one
SELECT * FROM legacy_import_receipts WHERE plan_sha256=$1 AND source_key=$2 AND phase=$3;

-- name: LatestLegacyImportReceipt :one
SELECT * FROM legacy_import_receipts
WHERE source_key=sqlc.arg(source_key)::text AND target_id=sqlc.arg(target_id)::uuid
ORDER BY revision DESC,committed_at DESC,phase LIMIT 1;

-- name: PriorLegacyImportPhase :one
SELECT * FROM legacy_import_receipts
WHERE source_key=sqlc.arg(source_key)::text AND target_id=sqlc.arg(target_id)::uuid AND phase=sqlc.arg(phase)::text
 AND outcome<>'verified_existing'
ORDER BY revision DESC,committed_at DESC LIMIT 1;

-- name: AddLegacyImportReceipt :exec
INSERT INTO legacy_import_receipts (plan_sha256,source_key,phase,intent_sha256,decision_sha256,outcome,target_id,public_id,revision,post_sha256)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: LegacySourceOwners :many
SELECT * FROM game_sources WHERE platform_code=$1 AND
 (external_key=$2 OR (platform_code='itch_io' AND lower(btrim(external_key))=$2))
ORDER BY id LIMIT 2;

-- name: LegacySiteHint :one
SELECT EXISTS(SELECT 1 FROM site_localizations l WHERE lower(btrim(l.name))=ANY(sqlc.arg(names)::text[])
 UNION ALL SELECT 1 FROM sites WHERE slug=sqlc.arg(slug)::text
 UNION ALL SELECT 1 FROM site_addresses a WHERE a.url=ANY(sqlc.arg(urls)::text[]));

-- name: LegacyGameHint :one
SELECT EXISTS(SELECT 1 FROM game_localizations l WHERE lower(btrim(l.name))=ANY(sqlc.arg(names)::text[])
 UNION ALL SELECT 1 FROM games WHERE slug=sqlc.arg(slug)::text);

-- name: LegacyEditorialCodeExists :one
SELECT (CASE sqlc.arg(kind)::text WHEN 'tag' THEN EXISTS(SELECT 1 FROM game_editorial_tags WHERE code=sqlc.arg(code)::text)
 WHEN 'tag_category' THEN EXISTS(SELECT 1 FROM game_editorial_tag_categories WHERE code=sqlc.arg(code)::text)
 ELSE false END)::boolean;

-- name: AddGameEditorialCategory :exec
INSERT INTO game_editorial_tag_categories (id,code,localizations,sort_order,archived_at)
VALUES ($1,$2,$3,$4,$5);

-- name: AddGameEditorialTag :exec
INSERT INTO game_editorial_tags (id,category_id,code,localizations,archived_at)
VALUES ($1,$2,$3,$4,$5);

-- name: AddGameEditorialMember :exec
INSERT INTO game_editorial_tag_members (game_id,tag_id,role) VALUES ($1,$2,$3);

-- name: AddGameCollectionRule :exec
INSERT INTO game_collection_tag_rules (collection_id,tag_id) VALUES ($1,$2);

-- name: AddGameCollectionExclusion :exec
INSERT INTO game_collection_exclusions (collection_id,game_id) VALUES ($1,$2);

-- name: LockLegacyCollection :one
SELECT * FROM game_collections WHERE id=$1 FOR UPDATE;

-- name: LockLegacyTag :one
SELECT * FROM game_editorial_tags WHERE id=$1 FOR UPDATE;

-- name: LockLegacyTagCategory :one
SELECT * FROM game_editorial_tag_categories WHERE id=$1 FOR UPDATE;

-- name: ReadLegacyEntity :one
SELECT t.public_id,t.revision,t.snapshot FROM (
 SELECT s.public_id,s.revision,jsonb_build_object('entity',to_jsonb(s),
 'locales',(SELECT jsonb_agg(to_jsonb(l) ORDER BY l.locale) FROM site_localizations l WHERE l.site_id=s.id),
 'addresses',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM site_addresses a WHERE a.site_id=s.id)) AS snapshot
 FROM sites s WHERE s.id=sqlc.arg(target_id)::uuid AND sqlc.arg(kind)::text='site'
 UNION ALL
 SELECT g.public_id,g.revision,jsonb_build_object('entity',to_jsonb(g),
 'locales',(SELECT jsonb_agg(to_jsonb(l) ORDER BY l.locale) FROM game_localizations l WHERE l.game_id=g.id),
 'sources',(SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id) FROM game_sources v WHERE v.game_id=g.id),
 'tags',(SELECT jsonb_agg(to_jsonb(t) ORDER BY t.tag_id) FROM game_editorial_tag_members t WHERE t.game_id=g.id))
 FROM games g WHERE g.id=sqlc.arg(target_id) AND sqlc.arg(kind)='game'
 UNION ALL
 SELECT c.public_id,c.revision,jsonb_build_object('entity',to_jsonb(c)-ARRAY['updated_at','revision','derived_membership_revision']::text[],
 'editorial_revision',c.revision-c.derived_membership_revision,
 'members',(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.position) FROM game_collection_members m WHERE m.collection_id=c.id),
 'rules',(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.tag_id) FROM game_collection_tag_rules r WHERE r.collection_id=c.id),
 'exclusions',(SELECT jsonb_agg(to_jsonb(e) ORDER BY e.game_id) FROM game_collection_exclusions e WHERE e.collection_id=c.id))
 FROM game_collections c WHERE c.id=sqlc.arg(target_id) AND sqlc.arg(kind)='collection'
 UNION ALL
 SELECT ''::text,1::bigint,to_jsonb(t) FROM game_editorial_tags t WHERE t.id=sqlc.arg(target_id) AND sqlc.arg(kind)='tag'
 UNION ALL
 SELECT ''::text,1::bigint,to_jsonb(k) FROM game_editorial_tag_categories k WHERE k.id=sqlc.arg(target_id) AND sqlc.arg(kind)='tag_category'
) t;

-- name: LegacyCollectionCounts :one
SELECT
 (SELECT count(*) FROM game_collection_members WHERE collection_id=sqlc.arg(id)::uuid) AS manual_count,
 (SELECT count(*) FROM game_collection_tag_rules WHERE collection_id=sqlc.arg(id)::uuid) AS rule_count,
 (SELECT count(*) FROM game_collection_exclusions WHERE collection_id=sqlc.arg(id)::uuid) AS exclusion_count,
 (SELECT count(*) FROM game_collection_members m JOIN game_collection_exclusions e USING (collection_id,game_id)
  WHERE m.collection_id=sqlc.arg(id)::uuid) AS manual_excluded_count,
 (SELECT count(*) FROM game_collection_effective_members WHERE collection_id=sqlc.arg(id)::uuid) AS effective_count,
 (SELECT count(*) FROM game_collection_effective_members m JOIN game_channel_public g ON g.id=m.game_id
  JOIN game_collections c ON c.id=m.collection_id AND c.status='published' WHERE m.collection_id=sqlc.arg(id)::uuid) AS public_count;


-- name: TouchLegacyGameTags :exec
UPDATE games SET revision=revision+1,updated_at=transaction_timestamp() WHERE id=$1;


-- name: LockLegacyItchSourceClaims :exec
LOCK TABLE game_sources IN SHARE ROW EXCLUSIVE MODE;
