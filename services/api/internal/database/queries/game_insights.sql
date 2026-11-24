-- name: SaveGamePriceObservations :exec
SELECT save_game_price_observations(sqlc.arg(generation)::uuid,sqlc.narg(source_id)::uuid,sqlc.arg(scope_key)::text);

-- name: GameInsightSources :many
SELECT s.id,s.external_key FROM game_channel_public g JOIN game_sources s ON s.game_id=g.id AND s.platform_code='steam'
WHERE g.public_id=$1
ORDER BY (s.id=g.intake_display_source) DESC NULLS LAST,s.active DESC,s.external_key LIMIT 16;

-- name: GamePlayerDays :many
SELECT d.utc_day::text AS day,count(*)::integer AS samples,max((p.value->>0)::bigint)::bigint AS maximum,
 round(avg((p.value->>0)::bigint),2)::double precision AS average,max((p.value->>1)::timestamptz)::timestamptz AS observed_at
FROM game_player_daily d CROSS JOIN LATERAL jsonb_each(d.samples) p
WHERE d.game_source_id=sqlc.arg(source_id)::uuid AND d.egress_region=sqlc.arg(egress)::text
 AND d.utc_day BETWEEN ((transaction_timestamp() AT TIME ZONE 'UTC')::date+1-sqlc.arg(days)::integer)
 AND (transaction_timestamp() AT TIME ZONE 'UTC')::date AND (p.value->>1)::timestamptz<=transaction_timestamp()
GROUP BY d.utc_day ORDER BY d.utc_day LIMIT 365;

-- name: GamePriceSeries :many
WITH bounds AS (
 SELECT (transaction_timestamp() AT TIME ZONE 'UTC')::date AS last_day,
  (transaction_timestamp() AT TIME ZONE 'UTC')::date+1-sqlc.arg(days)::integer AS first_day
), market AS NOT MATERIALIZED (
 SELECT p.* FROM game_price_observations p
 WHERE p.game_source_id=sqlc.arg(source_id)::uuid AND p.egress_region=sqlc.arg(egress)::text AND p.store_country=sqlc.arg(country)::text
  AND p.currency=sqlc.arg(currency)::text AND p.observed_at<=transaction_timestamp()
), language AS MATERIALIZED (
 SELECT coalesce(
  (SELECT p.locale FROM market p WHERE p.locale=sqlc.arg(locale)::text LIMIT 1),
  (SELECT p.locale FROM market p LIMIT 1)) AS locale
), scoped AS NOT MATERIALIZED (
 SELECT m.* FROM market m JOIN language l USING(locale)
), eligible AS (
 SELECT p.* FROM scoped p,bounds WHERE p.utc_day BETWEEN first_day AND last_day
 UNION ALL
 (SELECT p.* FROM scoped p,bounds WHERE p.utc_day<first_day ORDER BY p.utc_day DESC,p.observed_at DESC,p.observation_id DESC LIMIT 1)
), points AS (
 SELECT p.*,lag(jsonb_build_array(state,original,final,discount)) OVER (PARTITION BY binding_id ORDER BY observed_at,observation_id) AS previous,
  row_number() OVER (PARTITION BY utc_day ORDER BY observed_at DESC,observation_id DESC) AS last_in_day
 FROM eligible p
), flags AS (
 SELECT p.*,last_in_day=1 AS daily,previous IS NOT NULL AND previous<>jsonb_build_array(state,original,final,discount) AS changed
 FROM points p,bounds WHERE utc_day>=first_day
), capped AS (
 SELECT *,count(*) FILTER (WHERE changed) OVER (ORDER BY observed_at DESC,observation_id DESC) AS change_rank
 FROM flags
)
SELECT utc_day::text AS day,observed_at,observation_id,state,original,final,discount,locale,daily::boolean AS daily,
 (changed AND change_rank<=60)::boolean AS changed
FROM capped WHERE daily OR (changed AND change_rank<=60)
ORDER BY observed_at DESC,observation_id DESC;

-- name: GameInsightCoverage :one
WITH days AS (
 SELECT d.utc_day FROM game_player_daily d
 WHERE d.game_source_id=sqlc.arg(source_id)::uuid AND d.egress_region=sqlc.arg(egress)::text
  AND d.utc_day BETWEEN (transaction_timestamp() AT TIME ZONE 'UTC')::date-364 AND (transaction_timestamp() AT TIME ZONE 'UTC')::date-30
 UNION ALL
 SELECT p.utc_day FROM game_price_observations p
 WHERE p.game_source_id=sqlc.arg(source_id)::uuid AND p.egress_region=sqlc.arg(egress)::text
  AND p.store_country=sqlc.arg(country)::text AND p.currency=sqlc.arg(currency)::text
  AND p.utc_day BETWEEN (transaction_timestamp() AT TIME ZONE 'UTC')::date-364 AND (transaction_timestamp() AT TIME ZONE 'UTC')::date-30
)
SELECT coalesce(bool_or(utc_day>=(transaction_timestamp() AT TIME ZONE 'UTC')::date-89),false)::boolean AS show_ninety,
 coalesce(bool_or(utc_day<(transaction_timestamp() AT TIME ZONE 'UTC')::date-89),false)::boolean AS show_year
FROM days;

-- name: GameProfileNews :many
WITH work AS MATERIALIZED (
 SELECT id FROM game_channel_public WHERE public_id=sqlc.arg(public_id)::text
), sources AS MATERIALIZED (
 SELECT s.id,s.external_key FROM game_sources s JOIN work w ON w.id=s.game_id
 WHERE s.platform_code='steam' AND (sqlc.arg(app_id)::text='' OR s.external_key=sqlc.arg(app_id)::text)
 ORDER BY s.active DESC,s.external_key LIMIT 16
), latest AS (
 SELECT DISTINCT ON (s.external_key,n.gid) s.external_key AS app_id,n.gid,n.published_at,n.observed_at,
  n.observation_id,n.value->>'title' AS title,n.value->>'url' AS url
 FROM sources s
 JOIN collection_bindings b ON b.game_source_id=s.id AND b.purpose='steam'
 JOIN collection_sources src ON src.id=b.source_id AND src.provider='steam' AND src.external_key=s.external_key AND src.target_id IS NOT NULL
 JOIN game_evidence_news n ON n.source_id=src.id AND n.scope_key=game_insight_scope_hash(src.namespace,sqlc.arg(egress)::text,'news')
 WHERE n.value->>'title'<>'' AND n.published_at<=transaction_timestamp()
  AND n.value->>'url' ~ '^https://(store[.]steampowered[.]com/news/app/[0-9]+/view/[0-9]+|steamcommunity[.]com/games/[0-9]+/announcements/detail/[0-9]+|steamcommunity[.]com/ogg/[0-9]+/announcements/detail/[0-9]+|steamcommunity[.]com/announcements/detail/[0-9]+)$'
  AND coalesce(substring(n.value->>'url' FROM '/(?:games|ogg|news/app)/([0-9]+)/'),s.external_key)=s.external_key
  AND n.observed_at>=b.effective_from AND (b.effective_until IS NULL OR n.observed_at<b.effective_until)
  AND EXISTS (SELECT 1 FROM collection_plan_bindings m JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id
   WHERE m.binding_id=b.id AND m.source_id=src.id AND m.attached_at<=n.observed_at AND p.capability='steam.news'
    AND p.scope->>'egress_region'=sqlc.arg(egress)::text)
 ORDER BY s.external_key,n.gid,n.observed_at DESC,n.observation_id DESC
), events AS MATERIALIZED (
 SELECT DISTINCT ON (s.external_key,e.announcement_gid) s.external_key AS app_id,e.announcement_gid AS gid,
  e.published_at,e.headline AS title,'https://store.steampowered.com/news/app/'||s.external_key||'/view/'||e.announcement_gid AS url,
  e.id AS article_id,e.language, e.hidden OR NOT e.eligible AS suppressed,v.summary
 FROM sources s JOIN game_steam_events e ON e.game_source_id=s.id
 JOIN game_sources active ON active.id=s.id AND active.active
 JOIN game_steam_event_versions v ON v.event_id=e.id AND v.body_hash=e.body_hash
 WHERE e.egress_region=sqlc.arg(egress)::text AND e.language IN (0,6) AND e.published_at<=transaction_timestamp()
 ORDER BY s.external_key,e.announcement_gid,
   (e.language=CASE sqlc.arg(locale)::text WHEN 'zh' THEN 6 ELSE 0 END) DESC,e.observed_at DESC,e.id
), combined AS (
 SELECT e.app_id,e.gid,e.published_at,e.title,e.url,e.article_id,e.language,e.summary
 FROM events e WHERE NOT e.suppressed
 UNION ALL
 SELECT n.app_id,n.gid,n.published_at,n.title,n.url,'00000000-0000-0000-0000-000000000000'::uuid,-1,''::text FROM latest n
 WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.app_id=n.app_id AND
  (e.gid=n.gid OR e.gid=substring(n.url FROM '/([0-9]+)$')))
)
SELECT app_id,gid,published_at,title::text AS title,url::text AS url,article_id,language::integer,summary::text FROM combined
WHERE (published_at,gid,app_id)<(sqlc.arg(after_time)::timestamptz,sqlc.arg(after_gid)::text,sqlc.arg(after_app)::text)
ORDER BY published_at DESC,gid DESC,app_id DESC LIMIT sqlc.arg(take_rows)::integer;

-- name: RelatedGameProfiles :many
WITH current_game AS MATERIALIZED (
 SELECT id FROM game_channel_public WHERE public_id=sqlc.arg(public_id)::text
), genre_keys AS (
 SELECT coalesce(array_agg(DISTINCT genre),ARRAY[]::text[]) AS genres FROM current_game g
 JOIN game_channel_steam s ON s.game_id=g.id AND s.section='details'
 CROSS JOIN LATERAL unnest(s.genres) genre
), matches AS (
 SELECT m.game_id,3 AS weight FROM current_game g
 JOIN game_collection_effective_members own ON own.game_id=g.id
 JOIN game_collections c ON c.id=own.collection_id AND c.status='published'
 JOIN game_collection_effective_members m ON m.collection_id=c.id AND m.game_id<>g.id
 GROUP BY m.game_id
 UNION ALL
 SELECT s.game_id,count(DISTINCT genre)::integer AS weight
 FROM genre_keys k JOIN game_channel_steam s ON s.section='details' AND s.genres && k.genres
 CROSS JOIN LATERAL unnest(s.genres) genre
 WHERE genre=ANY(k.genres) AND s.game_id<>(SELECT id FROM current_game)
 GROUP BY s.game_id
), picked AS MATERIALIZED (
 SELECT g.id,g.public_id,g.slug,g.intake_display_source,sum(m.weight)::bigint AS score
 FROM matches m JOIN game_channel_public g ON g.id=m.game_id GROUP BY g.id,g.public_id,g.slug,g.intake_display_source
 ORDER BY score DESC,g.public_id LIMIT 6
)
SELECT g.public_id,g.slug,game_published_text(g.id)::jsonb AS localizations,
 coalesce(game_published_cover(g.id,sqlc.arg(locale)::text),'')::text AS cover
FROM picked g ORDER BY g.score DESC,g.public_id;

-- name: PublicGameIdentity :one
SELECT ((transaction_timestamp() AT TIME ZONE 'UTC')::date)::text AS today
FROM game_channel_public WHERE public_id=$1;
