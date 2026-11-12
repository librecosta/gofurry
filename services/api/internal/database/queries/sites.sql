-- name: CreateSite :one
INSERT INTO sites (id,public_id,slug) VALUES ($1,$2,$3)
ON CONFLICT (public_id) DO NOTHING RETURNING *;

-- name: LockSite :one
SELECT * FROM sites WHERE id=$1 FOR UPDATE;

-- name: UpdateSite :one
UPDATE sites SET slug=$2,revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: ChangeSiteStatus :one
UPDATE sites SET status=sqlc.arg(new_status),revision=revision+1,updated_at=transaction_timestamp(),
 published_at=CASE WHEN sqlc.arg(new_status)::text='published' THEN transaction_timestamp() ELSE published_at END,
 first_published_at=CASE WHEN sqlc.arg(new_status)::text='published' AND published_at IS NULL
  THEN coalesce(first_published_at,transaction_timestamp()) ELSE first_published_at END,
 archived_at=CASE WHEN sqlc.arg(new_status)::text='archived' THEN transaction_timestamp() ELSE archived_at END
WHERE id=sqlc.arg(id) RETURNING *;

-- name: RemoveOmittedSiteLocalizations :exec
DELETE FROM site_localizations WHERE site_id=$1 AND NOT (locale=ANY(sqlc.arg(locales)::text[]));

-- name: AddSiteLocalization :exec
INSERT INTO site_localizations (site_id,locale,name,summary) VALUES ($1,$2,$3,$4)
ON CONFLICT (site_id,locale) DO UPDATE SET name=excluded.name,summary=excluded.summary;

-- name: DisableSiteAddresses :exec
UPDATE site_addresses SET active=false WHERE site_id=$1 AND active;

-- name: CanPublishSite :one
SELECT (EXISTS(SELECT 1 FROM sites s WHERE s.id=sqlc.arg(id) AND s.content_rating='sfw' AND s.primary_category IS NOT NULL)
 AND EXISTS(SELECT 1 FROM site_localizations l WHERE l.site_id=sqlc.arg(id))
 AND EXISTS(SELECT 1 FROM site_addresses a WHERE a.site_id=sqlc.arg(id) AND active AND kind='primary'))::boolean;

-- name: SaveSiteAddress :exec
INSERT INTO site_addresses (id,site_id,url,kind) VALUES ($1,$2,$3,$4)
ON CONFLICT (site_id,url) DO UPDATE SET kind=excluded.kind,active=true;

-- name: PublishedSite :one
SELECT e.public_id,e.slug,COALESCE((SELECT jsonb_agg(jsonb_build_object('locale',l.locale,'name',l.name,'summary',l.summary,
 'body',CASE WHEN n.id IS NOT NULL THEN l.body ELSE '' END) ORDER BY l.locale)
 FROM site_localizations l WHERE l.site_id=e.id),'[]'::jsonb)::jsonb AS localizations,
 (n.id IS NOT NULL)::boolean AS interaction_eligible,
 n.primary_category,n.secondary_categories,n.navigation_tags,n.supported_languages,s.logo_asset_key,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('kind',x.kind,'url',x.url) ORDER BY x.kind DESC,x.url COLLATE "C")
 FROM site_addresses x WHERE x.site_id=n.id AND x.active),'[]'::jsonb)::jsonb AS addresses
FROM site_public_history e JOIN sites s ON s.id=e.id LEFT JOIN site_public_navigation n ON n.id=e.id
WHERE e.public_id=$1 AND e.status='published';

-- name: PublishedSitePage :many
SELECT e.public_id,e.slug,COALESCE((SELECT jsonb_agg(jsonb_build_object('locale',l.locale,'name',l.name,'summary',l.summary) ORDER BY l.locale)
 FROM site_localizations l WHERE l.site_id=e.id),'[]'::jsonb)::jsonb AS localizations
FROM site_public_navigation e WHERE e.public_id>sqlc.arg(after_public_id)
ORDER BY e.public_id LIMIT sqlc.arg(page_size);
