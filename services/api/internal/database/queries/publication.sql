-- name: SaveApprovedGamePresentation :exec
INSERT INTO game_approved_presentations
SELECT p.game_id,p.locale,p.name,p.summary,p.game_source_id,p.observation_id,p.observed_at,p.approval
FROM game_intake_presentation(sqlc.arg(game_id)::uuid,false) AS p(game_id,locale,name,summary,game_source_id,observation_id,observed_at,approval)
ON CONFLICT (game_id,locale) DO UPDATE SET
 name=excluded.name,summary=excluded.summary,observation_id=excluded.observation_id,observed_at=excluded.observed_at,
 approval=excluded.approval
WHERE game_approved_presentations.game_source_id=excluded.game_source_id
 AND game_approved_presentations.observed_at<excluded.observed_at
 AND (game_approved_presentations.name,game_approved_presentations.summary)
     IS DISTINCT FROM (excluded.name,excluded.summary);

-- name: SetGamePublicationBlock :one
UPDATE games SET publication_blocked=$2,revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: AddCatalogPublicationDecision :exec
INSERT INTO catalog_publication_decisions (id,entity_kind,entity_id,revision,action,actor,reason)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: RestoreSitePublication :one
UPDATE sites SET status='published',archived_at=NULL,published_at=coalesce(published_at,transaction_timestamp()),
 revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: RestoreGamePublication :one
UPDATE games SET status='published',archived_at=NULL,published_at=coalesce(published_at,transaction_timestamp()),
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

-- name: PublishedGamesMissingPresentation :one
SELECT count(*) FROM games g WHERE g.status='published'
 AND NOT EXISTS (SELECT 1 FROM game_localizations l WHERE l.game_id=g.id)
 AND NOT EXISTS (SELECT 1 FROM game_approved_presentations p WHERE p.game_id=g.id);

-- name: SaveSiteTechnology :exec
SELECT save_site_technology(sqlc.arg(generation)::uuid,sqlc.narg(source_id)::uuid,sqlc.narg(scope_key)::text);
