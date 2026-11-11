-- name: LockNavigationSite :one
SELECT * FROM sites WHERE public_id=$1 FOR UPDATE;

-- name: UpdateNavigationSite :one
UPDATE sites SET content_rating=$2,legacy_detail=false,primary_category=$3,secondary_categories=$4,
 navigation_tags=$5,supported_languages=$6,revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING *;

-- name: AddNavigationReview :exec
INSERT INTO site_navigation_reviews (id,site_id,site_revision,previous_rating,rating,actor,reason)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: LockNavigationFeatures :many
SELECT * FROM site_navigation_features ORDER BY slot FOR UPDATE;

-- name: InspectNavigationFeatures :many
SELECT f.*,coalesce(s.public_id,'')::text AS public_id,(v.id IS NOT NULL)::boolean AS visible
FROM site_navigation_features f LEFT JOIN sites s ON s.id=f.site_id
LEFT JOIN site_public_navigation v ON v.id=f.site_id ORDER BY f.slot;

-- name: UpdateNavigationFeature :one
UPDATE site_navigation_features SET site_id=$2,actor=$3,reason=$4,revision=revision+1,updated_at=transaction_timestamp()
WHERE slot=$1 RETURNING *;

-- name: NavigationHeatState :one
SELECT * FROM site_navigation_heat_state WHERE singleton;

-- name: LockNavigationHeat :one
SELECT pg_try_advisory_xact_lock(hashtextextended('gofurry/navigation/heat-v1',0));

-- name: RefreshNavigationHeat :one
WITH weighted AS MATERIALIZED (
 SELECT d.site_id,sum((d.detail_views::bigint+3*d.outbound_clicks::bigint) *
  (ARRAY[64000000,54400000,46240000,39304000,33408400,28397140,24137569])[1+(sqlc.arg(window_end)::date-d.utc_day)])::bigint AS score
 FROM site_interaction_days d JOIN site_public_navigation s ON s.id=d.site_id
 WHERE d.utc_day BETWEEN sqlc.arg(window_end)::date-6 AND sqlc.arg(window_end)::date
 GROUP BY d.site_id HAVING count(*)>=2 AND sum(d.detail_views::bigint+d.outbound_clicks::bigint)>=10
), saved AS (
 INSERT INTO site_navigation_heat (site_id,score) SELECT site_id,score FROM weighted
 ON CONFLICT (site_id) DO UPDATE SET score=excluded.score WHERE site_navigation_heat.score<>excluded.score
), removed AS (
 DELETE FROM site_navigation_heat h WHERE NOT EXISTS (SELECT 1 FROM weighted w WHERE w.site_id=h.site_id)
), published AS (
 INSERT INTO site_navigation_heat_state (singleton,algorithm,window_end,computed_at,aggregate_cutoff)
 VALUES (true,1,sqlc.arg(window_end)::date,sqlc.arg(computed_at)::timestamptz,sqlc.arg(aggregate_cutoff)::timestamptz)
 ON CONFLICT (singleton) DO UPDATE SET algorithm=excluded.algorithm,window_end=excluded.window_end,
  computed_at=excluded.computed_at,aggregate_cutoff=excluded.aggregate_cutoff
)
SELECT count(*)::bigint AS ranked FROM weighted;

-- name: NavigationAggregateCutoff :one
SELECT transaction_timestamp()::timestamptz;

-- name: CountNavigationInteraction :execrows
WITH visible AS (
 SELECT s.id FROM site_public_navigation s WHERE s.public_id=sqlc.arg(public_id) FOR SHARE OF s
)
INSERT INTO site_interaction_days (site_id,utc_day,detail_views,outbound_clicks)
SELECT id,sqlc.arg(utc_day)::date,sqlc.arg(detail)::integer,sqlc.arg(outbound)::integer FROM visible
ON CONFLICT (site_id,utc_day) DO UPDATE SET detail_views=site_interaction_days.detail_views+excluded.detail_views,
 outbound_clicks=site_interaction_days.outbound_clicks+excluded.outbound_clicks
WHERE site_interaction_days.detail_views<1000000000 AND site_interaction_days.outbound_clicks<1000000000;

-- name: NavigationStats :one
SELECT coalesce(sum(d.detail_views),0)::bigint AS detail_views,coalesce(sum(d.outbound_clicks),0)::bigint AS outbound_clicks,
 count(d.utc_day)::integer AS confirmed_days
FROM site_public_navigation s LEFT JOIN site_interaction_days d ON d.site_id=s.id
 AND d.utc_day BETWEEN sqlc.arg(utc_day)::date-6 AND sqlc.arg(utc_day)::date
WHERE s.public_id=sqlc.arg(public_id) GROUP BY s.id;

-- name: NavigationDirectory :one
WITH scored AS MATERIALIZED (
 SELECT s.id,s.public_id,
  CASE sqlc.arg(order_by)::text WHEN 'heat_7d' THEN coalesce(h.score,0)
   WHEN 'published_desc' THEN coalesce((extract(epoch FROM s.first_published_at)*1000000)::bigint,-9223372036854775807)
   WHEN 'relevance' THEN ((CASE WHEN f.exact_name THEN 8 WHEN f.prefix_name THEN 4 WHEN f.in_name THEN 2 ELSE 0 END)
    + CASE WHEN f.in_summary THEN 1 ELSE 0 END + CASE WHEN s.navigation_tags && sqlc.arg(search_tags)::text[] THEN 2 ELSE 0 END)::bigint
   ELSE 0::bigint END AS metric,
  CASE WHEN sqlc.arg(order_by)::text='name_asc' THEN coalesce((
   SELECT lower(l.name) FROM site_localizations l WHERE l.site_id=s.id
   ORDER BY (l.locale=sqlc.arg(locale)::text) DESC,l.locale LIMIT 1
  ),'') ELSE '' END COLLATE "C" AS name_key
 FROM site_public_navigation s
 LEFT JOIN site_navigation_heat h ON sqlc.arg(use_heat)::boolean AND h.site_id=s.id
 CROSS JOIN LATERAL (
  SELECT coalesce(bool_or(lower(l.name)=lower(sqlc.arg(keyword)::text)),false) AS exact_name,
   coalesce(bool_or(starts_with(lower(l.name),lower(sqlc.arg(keyword)::text))),false) AS prefix_name,
   coalesce(bool_or(strpos(lower(l.name),lower(sqlc.arg(keyword)::text))>0),false) AS in_name,
   coalesce(bool_or(strpos(lower(l.summary),lower(sqlc.arg(keyword)::text))>0),false) AS in_summary
  FROM site_localizations l WHERE sqlc.arg(keyword)::text<>'' AND l.site_id=s.id
 ) f
 WHERE (cardinality(sqlc.arg(categories)::text[])=0 OR s.primary_category=ANY(sqlc.arg(categories)::text[])
  OR s.secondary_categories && sqlc.arg(categories)::text[])
 AND (cardinality(sqlc.arg(tags)::text[])=0 OR s.navigation_tags && sqlc.arg(tags)::text[])
 AND (cardinality(sqlc.arg(languages)::text[])=0 OR s.supported_languages && sqlc.arg(languages)::text[])
 AND (sqlc.arg(keyword)::text='' OR f.in_name OR f.in_summary OR s.navigation_tags && sqlc.arg(search_tags)::text[])
), anchor AS MATERIALIZED (
 SELECT * FROM scored WHERE public_id=sqlc.arg(after_id)::text
), page AS (
 SELECT a.* FROM scored a LEFT JOIN anchor b ON true
 WHERE sqlc.arg(after_id)::text='' OR a.metric<b.metric OR (a.metric=b.metric AND (a.name_key,a.public_id)>(b.name_key,b.public_id))
 ORDER BY a.metric DESC,a.name_key,a.public_id LIMIT sqlc.arg(page_size)
)
SELECT (SELECT count(*) FROM scored)::bigint AS total, (sqlc.arg(after_id)::text='' OR EXISTS(SELECT 1 FROM anchor))::boolean AS cursor_valid,
 coalesce(jsonb_agg(jsonb_build_object(
  'public_id',s.public_id,'slug',s.slug,'primary_category',s.primary_category,'secondary_categories',s.secondary_categories,
   'heat_qualified',sqlc.arg(use_heat)::boolean AND p.metric>0,
   'tags',s.navigation_tags,'languages',s.supported_languages,'first_published_at',s.first_published_at,
   'localizations',coalesce((SELECT jsonb_agg(jsonb_build_object('locale',l.locale,'name',l.name,'summary',l.summary) ORDER BY l.locale)
    FROM site_localizations l WHERE l.site_id=s.id),'[]'::jsonb)) ORDER BY p.metric DESC,p.name_key,p.public_id),'[]'::jsonb)::jsonb AS items
FROM page p JOIN sites s ON s.id=p.id;

-- name: NavigationHome :many
WITH featured AS MATERIALIZED (
 SELECT f.slot,s.id FROM site_navigation_features f JOIN site_public_navigation s ON s.id=f.site_id
), trending AS MATERIALIZED (
 SELECT s.id,row_number() OVER (ORDER BY h.score DESC,s.public_id)::integer AS rank
 FROM site_navigation_heat h JOIN site_public_navigation s ON s.id=h.site_id
 WHERE sqlc.arg(use_heat)::boolean AND NOT EXISTS (SELECT 1 FROM featured f WHERE f.id=s.id)
 ORDER BY h.score DESC,s.public_id LIMIT 8
), recent AS MATERIALIZED (
 SELECT s.id,row_number() OVER (ORDER BY s.first_published_at DESC NULLS LAST,s.public_id)::integer AS rank
 FROM site_public_navigation s WHERE s.first_published_at IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM featured f WHERE f.id=s.id) AND NOT EXISTS (SELECT 1 FROM trending t WHERE t.id=s.id)
 ORDER BY s.first_published_at DESC NULLS LAST,s.public_id LIMIT 8
), random_site AS (
 SELECT s.id FROM site_public_navigation s WHERE NOT EXISTS (SELECT 1 FROM featured f WHERE f.id=s.id)
  AND NOT EXISTS (SELECT 1 FROM trending t WHERE t.id=s.id) AND NOT EXISTS (SELECT 1 FROM recent r WHERE r.id=s.id)
 ORDER BY md5(s.public_id||sqlc.arg(seed)::text),s.public_id LIMIT 1
), sections AS (
 SELECT 'featured'::text AS section,slot AS key,0 AS rank,id FROM featured
 UNION ALL SELECT 'trending','',rank,id FROM trending UNION ALL SELECT 'recent','',rank,id FROM recent
 UNION ALL SELECT 'random','',0,id FROM random_site
)
SELECT x.section,x.key,jsonb_build_object('public_id',s.public_id,'slug',s.slug,'primary_category',s.primary_category,
 'secondary_categories',s.secondary_categories,'tags',s.navigation_tags,'languages',s.supported_languages,'first_published_at',s.first_published_at,
 'localizations',coalesce((SELECT jsonb_agg(jsonb_build_object('locale',l.locale,'name',l.name,'summary',l.summary) ORDER BY l.locale)
 FROM site_localizations l WHERE l.site_id=s.id),'[]'::jsonb))::jsonb AS item
FROM sections x JOIN sites s ON s.id=x.id ORDER BY x.section,x.rank,x.key;

-- name: NavigationCategoryCounts :many
SELECT c.code::text AS code,count(*)::bigint AS count FROM site_public_navigation s
CROSS JOIN LATERAL unnest(ARRAY[s.primary_category]||s.secondary_categories) AS c(code) GROUP BY c.code;
