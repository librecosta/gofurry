-- name: RecentGameChannelCards :many
WITH picked AS MATERIALIZED (
 SELECT g.* FROM game_channel_public g WHERE
 (sqlc.arg(store_filter)::text='' OR EXISTS (
  SELECT 1 FROM game_sources s WHERE s.game_id=g.id AND s.platform_code=sqlc.arg(store_filter) AND s.active))
 AND (g.first_published_at IS NULL OR g.first_published_at<=sqlc.arg(as_of)::timestamptz)
 AND (NOT sqlc.arg(has_after)::boolean OR
  (g.first_published_at IS NULL AND sqlc.arg(after_bucket)::integer=0)
  OR (g.first_published_at=sqlc.arg(after_time)::timestamptz AND g.public_id>sqlc.arg(after_id)::text)
  OR g.first_published_at<sqlc.arg(after_time)::timestamptz
  OR (g.first_published_at IS NULL AND sqlc.arg(after_bucket)=1 AND g.public_id>sqlc.arg(after_id)))
 ORDER BY g.first_published_at DESC NULLS LAST,g.public_id LIMIT sqlc.arg(take_rows)::integer
)
SELECT game_channel_card(g.id,g.public_id,g.slug,g.first_published_at,
 game_presentation(g.id,g.intake_display_source)->'localizations',f.genres,f.systems,f.release_state,f.release,
 game_channel_cover(g.id,g.intake_display_source,sqlc.arg(language)::text,sqlc.arg(country)::text,sqlc.arg(egress)::text),
 (SELECT q.price FROM game_channel_quotes(sqlc.arg(country),sqlc.arg(currency)::text,sqlc.arg(egress)) q(game_id,game_source_id,locale,confirmed_at,price)
  WHERE q.game_id=g.id ORDER BY (q.game_source_id=g.intake_display_source) DESC NULLS LAST,
   (q.locale=sqlc.arg(language)) DESC,q.confirmed_at DESC,q.game_source_id LIMIT 1))::jsonb AS card,
 (g.first_published_at IS NULL)::integer AS e_bucket,
 (-coalesce((extract(epoch FROM g.first_published_at)*1000000)::bigint,0))::bigint AS e_ordering,
 ''::text AS e_name_key,g.public_id
FROM picked g CROSS JOIN LATERAL game_channel_facts(g.id,g.intake_display_source,sqlc.arg(language),sqlc.arg(country),sqlc.arg(egress))
 f(genres,systems,developers,release_state,release_day,release)
ORDER BY g.first_published_at DESC NULLS LAST,g.public_id;

-- name: DisableGameChannelJIT :exec
SELECT set_config('jit','off',true);

-- name: GameChannelCards :many
WITH settings AS NOT MATERIALIZED (
 SELECT sqlc.arg(locale)::text AS locale,sqlc.arg(country)::text AS country,sqlc.arg(currency)::text AS currency,
 sqlc.arg(egress)::text AS egress,sqlc.arg(sort_key)::text AS sort,
 sqlc.arg(genre_filters)::text[] AS genres,sqlc.arg(store_filter)::text AS store,sqlc.arg(system_filter)::text AS os,
 sqlc.arg(phase_filter)::text AS phase,sqlc.arg(price_filter)::text AS price,sqlc.arg(keyword)::text AS keyword
), prices AS NOT MATERIALIZED (
 SELECT q.* FROM settings p CROSS JOIN LATERAL game_channel_quotes(p.country,p.currency,p.egress) q(game_id,game_source_id,locale,confirmed_at,price)
), works AS (
 SELECT g.*,0 AS position FROM game_channel_public g WHERE sqlc.arg(view_kind)::text='directory'
 UNION ALL
 SELECT g.*,m.position FROM game_collection_members m JOIN game_channel_public g ON g.id=m.game_id
 WHERE m.collection_id=sqlc.narg(collection_id)::uuid AND sqlc.arg(view_kind)::text='members'
), eligible AS NOT MATERIALIZED (
 SELECT g.*,p.locale,p.country,p.currency,p.egress,p.sort,p.genres,p.os,p.phase,p.price,p.keyword
 FROM settings p CROSS JOIN works g
 WHERE (p.store='' OR EXISTS (SELECT 1 FROM game_sources s WHERE s.game_id=g.id AND s.platform_code=p.store AND s.active))
 AND (g.first_published_at IS NULL OR g.first_published_at<=sqlc.arg(as_of)::timestamptz)
), inspected AS (
 SELECT e.*,f.genres AS found_genres,f.systems,f.release_state,f.release_day,f.release,
  q.price AS quote
 FROM eligible e
 CROSS JOIN LATERAL game_channel_facts(e.id,e.intake_display_source,
  CASE e.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END,e.country,e.egress) f(genres,systems,developers,release_state,release_day,release)
 LEFT JOIN LATERAL (
  SELECT price FROM prices q WHERE q.game_id=e.id AND e.price<>''
  ORDER BY (q.game_source_id=e.intake_display_source) DESC NULLS LAST,
   (q.locale=CASE e.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END) DESC,q.confirmed_at DESC,q.game_source_id LIMIT 1
 ) q ON true
 WHERE (cardinality(e.genres)=0 OR f.genres && e.genres) AND (e.os='' OR e.os=ANY(f.systems))
 AND (e.phase='' OR f.release_state=e.phase)
 AND (e.keyword='' OR EXISTS (SELECT 1 FROM game_channel_locales t WHERE t.game_id=e.id
  AND (strpos(normalize(lower(t.name),NFKC),e.keyword)>0 OR strpos(normalize(lower(t.summary),NFKC),e.keyword)>0))
  OR EXISTS (SELECT 1 FROM unnest(f.developers) d WHERE strpos(normalize(lower(d),NFKC),e.keyword)>0))
 AND (e.price='' OR (e.price='free' AND q.price->>'state'='free') OR
  (e.price='discount' AND q.price->>'state' IN ('priced','zero_price')
   AND (q.price#>>'{amounts,discount_percent}')::int>0
   AND (q.price#>>'{amounts,initial}')::bigint>(q.price#>>'{amounts,final}')::bigint))
), ranked AS (
 SELECT i.*,
  CASE i.sort WHEN 'added_desc' THEN (i.first_published_at IS NULL)::int
   WHEN 'release_desc' THEN (i.release_state<>'released' OR i.release_day IS NULL)::int
   WHEN 'upcoming_asc' THEN (i.release_state<>'upcoming' OR i.release_day IS NULL)::int
   WHEN 'timeline' THEN CASE WHEN i.release_state='released' AND i.release_day IS NOT NULL THEN 0
    WHEN i.release_state='upcoming' AND i.release_day IS NOT NULL THEN 1 ELSE 2 END
   ELSE 0 END AS bucket,
  CASE i.sort WHEN 'added_desc' THEN -coalesce((extract(epoch FROM i.first_published_at)*1000000)::bigint,0)
   WHEN 'release_desc' THEN CASE WHEN i.release_state='released' THEN -coalesce(i.release_day-date '2000-01-01',0) ELSE 0 END
   WHEN 'upcoming_asc' THEN CASE WHEN i.release_state='upcoming' THEN coalesce(i.release_day-date '2000-01-01',0) ELSE 0 END
   WHEN 'timeline' THEN CASE WHEN i.release_state IN ('released','upcoming') AND i.release_day IS NOT NULL THEN i.release_day-date '2000-01-01' ELSE 0 END
   WHEN 'editorial' THEN i.position ELSE 0 END::bigint AS ordering,
  CASE i.sort WHEN 'name_asc' THEN coalesce((SELECT normalize(lower(t.name),NFKC) FROM game_channel_locales t WHERE t.game_id=i.id
    ORDER BY (t.locale=i.locale) DESC,t.locale LIMIT 1),'') ELSE '' END COLLATE "C" AS name_key
 FROM inspected i
), picked AS MATERIALIZED (
 SELECT r.id,r.public_id,r.slug,r.first_published_at,r.intake_display_source,r.locale,r.country,r.currency,r.egress,
  r.found_genres,r.systems,r.release_state,r.release,r.quote,r.bucket,r.ordering,r.name_key FROM ranked r
 WHERE NOT sqlc.arg(has_after)::boolean OR (r.bucket,r.ordering,r.name_key,r.public_id)>
  (sqlc.arg(after_bucket)::integer,sqlc.arg(after_ordering)::bigint,sqlc.arg(after_name)::text COLLATE "C",sqlc.arg(after_id)::text)
 ORDER BY r.bucket,r.ordering,r.name_key,r.public_id LIMIT sqlc.arg(take_rows)::integer
)
SELECT game_channel_card(e.id,e.public_id,e.slug,e.first_published_at,game_presentation(e.id,e.intake_display_source)->'localizations',e.found_genres,e.systems,e.release_state,e.release,
 game_channel_cover(e.id,e.intake_display_source,CASE e.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END,e.country,e.egress),
 coalesce(e.quote,(SELECT q.price FROM prices q WHERE q.game_id=e.id
  ORDER BY (q.game_source_id=e.intake_display_source) DESC NULLS LAST,
   (q.locale=CASE e.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END) DESC,q.confirmed_at DESC,q.game_source_id LIMIT 1)))::jsonb AS card,
 e.bucket::integer,e.ordering::bigint,e.name_key::text,e.public_id
FROM picked e ORDER BY e.bucket,e.ordering,e.name_key COLLATE "C",e.public_id;

-- name: GameChannelHome :many
WITH settings AS (
 SELECT sqlc.arg(locale)::text AS locale,sqlc.arg(country)::text AS country,sqlc.arg(currency)::text AS currency,sqlc.arg(egress)::text AS egress
), facts AS MATERIALIZED (
 SELECT DISTINCT ON (g.id) g.id,g.public_id,m.genres,m.coming_soon,m.release_day
 FROM game_channel_steam m JOIN game_channel_public g ON g.id=m.game_id
 JOIN game_sources s ON s.id=m.game_source_id AND s.game_id=g.id
 CROSS JOIN settings p WHERE m.section='details'
 ORDER BY g.id,(m.game_source_id=g.intake_display_source) DESC NULLS LAST,s.active DESC,
  (m.egress_region=p.egress AND m.store_country=p.country) DESC,
  (m.locale=CASE p.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END) DESC,
  m.observed_at DESC,m.game_source_id,m.locale,m.store_country,m.egress_region
), quotes AS MATERIALIZED (
 SELECT DISTINCT ON (q.game_id) q.game_id,q.price,q.confirmed_at FROM settings p
 CROSS JOIN LATERAL game_channel_quotes(p.country,p.currency,p.egress) q(game_id,game_source_id,locale,confirmed_at,price) JOIN game_channel_public g ON g.id=q.game_id
 ORDER BY q.game_id,(q.game_source_id=g.intake_display_source) DESC NULLS LAST,
 (q.locale=CASE p.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END) DESC,q.confirmed_at DESC,q.game_source_id
), lanes AS (
 SELECT 'featured'::text AS kind,s.slot,ARRAY_POSITION(ARRAY['primary','secondary_1','secondary_2'],s.slot)::bigint AS ranking,g.id
 FROM game_channel_features s JOIN game_channel_public g ON g.id=s.entity_id WHERE s.slot IN ('primary','secondary_1','secondary_2')
 UNION ALL
 SELECT 'recent','',row_number() OVER (ORDER BY x.first_published_at DESC,x.public_id),x.id FROM (
  SELECT id,first_published_at,public_id FROM game_channel_public WHERE first_published_at IS NOT NULL
  ORDER BY first_published_at DESC,public_id LIMIT 8) x
 UNION ALL
 SELECT 'upcoming','',row_number() OVER (ORDER BY x.release_day NULLS LAST,x.public_id),x.id FROM (
  SELECT id,release_day,public_id FROM facts WHERE coming_soon ORDER BY release_day NULLS LAST,public_id LIMIT 4) x
 UNION ALL
 SELECT 'released','',row_number() OVER (ORDER BY x.release_day DESC,x.public_id),x.id FROM (
  SELECT id,release_day,public_id FROM facts WHERE NOT coming_soon
  AND release_day BETWEEN (statement_timestamp() AT TIME ZONE 'UTC')::date-90 AND (statement_timestamp() AT TIME ZONE 'UTC')::date
  ORDER BY release_day DESC,public_id LIMIT 4) x
 UNION ALL
 SELECT 'offers','',row_number() OVER (ORDER BY x.confirmed_at DESC,x.game_id),x.game_id FROM (
  SELECT game_id,confirmed_at FROM quotes WHERE price->>'state'='free' OR
   ((price#>>'{amounts,discount_percent}')::int>0 AND (price#>>'{amounts,initial}')::bigint>(price#>>'{amounts,final}')::bigint)
  ORDER BY confirmed_at DESC,game_id LIMIT 4) x
), genre_counts AS (
 SELECT genre,count(*) AS total FROM facts CROSS JOIN LATERAL unnest(genres) genre GROUP BY genre
)
SELECT l.kind,l.slot,l.ranking,game_channel_card(g.id,g.public_id,g.slug,g.first_published_at,
 game_presentation(g.id,g.intake_display_source)->'localizations',f.genres,f.systems,f.release_state,f.release,
 game_channel_cover(g.id,g.intake_display_source,CASE p.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END,p.country,p.egress),q.price)::jsonb AS payload
FROM lanes l JOIN game_channel_public g ON g.id=l.id CROSS JOIN settings p
CROSS JOIN LATERAL game_channel_facts(g.id,g.intake_display_source,CASE p.locale WHEN 'zh' THEN 'zh-cn' ELSE 'en' END,p.country,p.egress)
 f(genres,systems,developers,release_state,release_day,release) LEFT JOIN quotes q ON q.game_id=g.id
UNION ALL SELECT 'genre','',0,jsonb_build_object('code',genre,'count',total) FROM genre_counts
ORDER BY 1,3,2;

-- name: GameChannelNews :many
WITH latest AS (
 SELECT DISTINCT ON (g.public_id,e.gid) g.public_id,g.slug,s.external_key,e.gid,e.published_at,e.observation_id,e.value
 FROM game_evidence_news e
 JOIN collection_sources src ON src.id=e.source_id AND src.provider='steam' AND src.resource_type='app'
 JOIN game_sources s ON s.platform_code='steam' AND s.external_key=src.external_key AND s.active
 JOIN game_channel_public g ON g.id=s.game_id
 WHERE e.published_at<=least(e.observed_at,statement_timestamp()) AND e.published_at>=statement_timestamp()-interval '90 days'
  AND e.value->>'title'<>''
  AND (e.value->>'url' ~ ('^https://steamcommunity\.com/games/'||s.external_key||'/announcements/detail/[0-9]+/?$')
   OR e.value->>'url' ~ ('^https://store\.steampowered\.com/news/app/'||s.external_key||'/view/[0-9]+/?$'))
 AND EXISTS (SELECT 1 FROM collection_bindings b
  JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=b.source_id
  JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id AND p.capability='steam.news'
  WHERE b.game_source_id=s.id AND b.game_id=g.id AND b.source_id=src.id AND b.purpose='steam'
  AND e.observed_at>=greatest(b.effective_from,m.attached_at) AND (b.effective_until IS NULL OR e.observed_at<b.effective_until)
  AND p.scope->>'egress_region'=sqlc.arg(egress)::text
  AND e.scope_key=encode(sha256(convert_to('{"namespace":'||to_json(src.namespace)::text||',"egress_region":'||
   to_json(p.scope->>'egress_region')::text||',"dimensions":{"section":"news"}}','UTF8')),'hex'))
 ORDER BY g.public_id,e.gid,e.observed_at DESC,src.id
)
SELECT public_id,slug,external_key,gid,published_at,observation_id,value FROM latest
ORDER BY published_at DESC,gid DESC,public_id LIMIT 6;

-- name: ListGameCollections :many
SELECT public_id,slug,localizations,first_published_at FROM game_collections c
WHERE status='published' AND c.first_published_at<=sqlc.arg(as_of)::timestamptz
AND (NOT sqlc.arg(has_after)::boolean OR c.first_published_at<sqlc.arg(after_time)::timestamptz
 OR (c.first_published_at=sqlc.arg(after_time) AND c.public_id>sqlc.arg(after_id)::text))
AND (sqlc.arg(keyword)::text='' OR EXISTS (SELECT 1 FROM jsonb_array_elements(c.localizations) t
 WHERE strpos(normalize(lower(t->>'name'),NFKC),sqlc.arg(keyword))>0 OR strpos(normalize(lower(t->>'summary'),NFKC),sqlc.arg(keyword))>0))
ORDER BY first_published_at DESC,public_id LIMIT sqlc.arg(take_rows)::integer;

-- name: FeaturedGameCollections :many
SELECT c.public_id,c.slug,c.localizations,c.first_published_at FROM game_channel_features f
JOIN game_collections c ON c.id=f.entity_id AND c.status='published' WHERE f.slot IN ('collection_1','collection_2','collection_3') ORDER BY f.slot;

-- name: PublicGameCollection :one
SELECT * FROM game_collections WHERE public_id=$1 AND status='published';

-- name: ReadGameCollection :one
SELECT * FROM game_collections WHERE public_id=$1;

-- name: CreateGameCollection :one
INSERT INTO game_collections (id,public_id,slug,localizations) VALUES ($1,$2,$3,$4)
ON CONFLICT (public_id) DO NOTHING RETURNING *;

-- name: LockGameCollection :one
SELECT * FROM game_collections WHERE public_id=$1 FOR UPDATE;

-- name: EditGameCollection :one
UPDATE game_collections SET slug=$2,localizations=$3,revision=revision+1,updated_at=statement_timestamp() WHERE id=$1 RETURNING *;

-- name: UpdateGameCollectionStatus :one
UPDATE game_collections SET status=sqlc.arg(new_status),
 first_published_at=CASE WHEN sqlc.arg(new_status)::text='published' THEN coalesce(first_published_at,transaction_timestamp()) ELSE first_published_at END,
 revision=revision+1,updated_at=statement_timestamp() WHERE id=sqlc.arg(id) RETURNING *;

-- name: ClearGameCollectionMembers :exec
DELETE FROM game_collection_members WHERE collection_id=$1;

-- name: SaveGameCollectionMembers :execrows
INSERT INTO game_collection_members (collection_id,game_id,position)
SELECT sqlc.arg(collection_id)::uuid,g.id,m.position::integer
FROM unnest(sqlc.arg(public_ids)::text[]) WITH ORDINALITY m(public_id,position) JOIN games g ON g.public_id=m.public_id;

-- name: TouchGameCollection :one
UPDATE game_collections SET revision=revision+1,updated_at=statement_timestamp() WHERE id=$1 RETURNING *;

-- name: PrivateGameCollectionMembers :many
SELECT g.public_id FROM game_collection_members m JOIN games g ON g.id=m.game_id WHERE m.collection_id=$1 ORDER BY m.position;

-- name: ReadGameChannelFeatures :many
SELECT f.slot,f.revision,coalesce(g.public_id,c.public_id,'')::text AS public_id FROM game_channel_features f
LEFT JOIN games g ON g.id=f.entity_id AND f.slot IN ('primary','secondary_1','secondary_2')
LEFT JOIN game_collections c ON c.id=f.entity_id AND f.slot IN ('collection_1','collection_2','collection_3')
ORDER BY f.slot;

-- name: SetGameChannelFeature :one
UPDATE game_channel_features SET entity_id=sqlc.narg(entity_id)::uuid,revision=revision+1
WHERE slot=sqlc.arg(slot) AND revision=sqlc.arg(expected_revision) RETURNING revision;
