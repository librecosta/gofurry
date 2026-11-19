-- name: AddCatalogPublicationDecision :exec
INSERT INTO catalog_publication_decisions (id,entity_kind,entity_id,revision,action,actor,reason,management_request_id,content_rating)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: RestoreSitePublication :one
UPDATE sites SET status='published',archived_at=NULL,published_at=coalesce(published_at,transaction_timestamp()),
 revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: LockGamePublicID :one
SELECT * FROM games WHERE public_id=$1 FOR UPDATE;

-- name: InitialSitePolicy :one
UPDATE sites SET content_rating='sfw',revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: PromoteLegacySitePolicy :many
WITH candidates AS MATERIALIZED (
 SELECT s.id,s.revision,r.actor,r.reason
 FROM sites s CROSS JOIN LATERAL (
  SELECT review.rating,review.actor,review.reason FROM site_navigation_reviews review
  WHERE review.site_id=s.id ORDER BY review.site_revision DESC LIMIT 1
 ) r
 WHERE s.legacy_detail AND s.content_rating='unreviewed' AND s.status IN ('published','archived') AND r.rating='sfw'
 ORDER BY s.public_id LIMIT sqlc.arg(batch_size) FOR UPDATE OF s SKIP LOCKED
), changed AS (
 UPDATE sites s SET content_rating='sfw',legacy_detail=false,revision=s.revision+1,updated_at=transaction_timestamp()
 FROM candidates c WHERE s.id=c.id AND s.revision=c.revision
 RETURNING s.id,s.revision,c.actor,c.reason
), audited AS (
 INSERT INTO site_navigation_reviews (id,site_id,site_revision,previous_rating,rating,actor,reason)
 SELECT uuidv7(),id,revision,'unreviewed','sfw',actor,reason FROM changed
)
SELECT id FROM changed;

-- name: LegacySitePolicyPending :one
SELECT count(*) FROM sites WHERE legacy_detail AND content_rating='unreviewed' AND status IN ('published','archived');

-- name: SaveSiteTechnology :exec
SELECT save_site_technology(sqlc.arg(generation)::uuid,sqlc.narg(source_id)::uuid,sqlc.narg(scope_key)::text);
