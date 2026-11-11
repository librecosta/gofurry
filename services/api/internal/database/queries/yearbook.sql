-- name: CompactSiteCoverage :one
WITH picked AS MATERIALIZED (
 -- Limit the raw, indexed scan before joining provenance. Otherwise a low
 -- selectivity estimate for the provenance predicates can cause a full sort.
 SELECT d.* FROM site_daily_coverage d
 WHERE d.utc_day<sqlc.arg(before_day)::date
  AND (d.utc_day,d.site_id)>=(sqlc.arg(after_day)::date,sqlc.arg(after_site)::uuid)
  AND (d.utc_day,d.site_id,d.source_id,d.capability,d.output_schema,d.scope_key)>
      (sqlc.arg(after_day)::date,sqlc.arg(after_site)::uuid,sqlc.arg(after_source)::uuid,
       sqlc.arg(after_capability)::text,sqlc.arg(after_schema)::integer,sqlc.arg(after_scope)::text)
 ORDER BY d.utc_day,d.site_id,d.source_id,d.capability,d.output_schema,d.scope_key
 LIMIT sqlc.arg(batch_size) FOR UPDATE OF d SKIP LOCKED
), batch AS MATERIALIZED (
 SELECT d.*,s.external_key,s.target_id,b.purpose,b.effective_from,b.effective_until,date_trunc('month',d.utc_day::timestamp)::date AS month,
  (1::bigint<<(extract(day FROM d.utc_day)::integer-1)) AS day_bit
 FROM picked d
 JOIN collection_bindings b ON b.id=d.binding_id AND b.site_id=d.site_id AND b.source_id=d.source_id
 JOIN collection_sources s ON s.id=d.source_id AND s.provider='web' AND s.resource_type=b.resource_type
 WHERE d.capability IN ('web.http','web.dns','web.tls','web.rdap')
  AND d.output_schema=1 AND d.scope_key ~ '^[a-f0-9]{64}$' AND s.target_id IS NOT NULL
  AND s.resource_type=CASE WHEN d.capability IN ('web.dns','web.rdap') THEN 'domain' ELSE 'origin' END
  AND d.utc_day=(d.confirmed_at AT TIME ZONE 'UTC')::date
  AND d.confirmed_at>=b.effective_from AND (b.effective_until IS NULL OR d.confirmed_at<b.effective_until)
), merged AS (
 INSERT INTO site_monthly_coverage
 (site_id,binding_id,source_id,target_id,source_key,binding_purpose,binding_from,binding_until,capability,output_schema,scope_key,month,day_mask,first_confirmed_at,last_confirmed_at)
 SELECT site_id,binding_id,source_id,target_id,external_key,purpose,effective_from,effective_until,capability,output_schema,scope_key,month,
  bit_or(day_bit),min(confirmed_at),max(confirmed_at)
 FROM batch GROUP BY site_id,binding_id,source_id,target_id,external_key,purpose,effective_from,effective_until,capability,output_schema,scope_key,month
 ORDER BY site_id,month,binding_id,source_id,capability,output_schema,scope_key
 ON CONFLICT (site_id,month,binding_id,source_id,capability,output_schema,scope_key) DO UPDATE SET
  day_mask=site_monthly_coverage.day_mask|excluded.day_mask,
  binding_until=coalesce(excluded.binding_until,site_monthly_coverage.binding_until),
  first_confirmed_at=least(site_monthly_coverage.first_confirmed_at,excluded.first_confirmed_at),
  last_confirmed_at=greatest(site_monthly_coverage.last_confirmed_at,excluded.last_confirmed_at)
 RETURNING site_id,binding_id,source_id,capability,output_schema,scope_key,month,day_mask
), verified AS (
 SELECT a.* FROM batch a JOIN merged m USING (site_id,binding_id,source_id,capability,output_schema,scope_key,month)
 WHERE (m.day_mask&a.day_bit)=a.day_bit
), pruned AS (
 DELETE FROM site_daily_coverage d USING verified v
 -- Parameterized index bounds prevent PostgreSQL from reading the entire
 -- daily table to join a small batch against its exact verified row keys.
 WHERE d.utc_day BETWEEN (SELECT min(utc_day) FROM batch) AND (SELECT max(utc_day) FROM batch)
  AND (d.site_id,d.source_id,d.capability,d.output_schema,d.scope_key,d.utc_day,d.binding_id,d.confirmed_at)
      =(v.site_id,v.source_id,v.capability,v.output_schema,v.scope_key,v.utc_day,v.binding_id,v.confirmed_at)
 RETURNING d.site_id
)
SELECT (SELECT count(*) FROM picked)::integer AS scanned,
 (SELECT count(*) FROM batch)::integer AS selected,(SELECT count(*) FROM pruned)::integer AS pruned,
 coalesce(p.utc_day,sqlc.arg(after_day)::date)::date AS last_day,
 coalesce(p.site_id,sqlc.arg(after_site)::uuid)::uuid AS last_site,
 coalesce(p.source_id,sqlc.arg(after_source)::uuid)::uuid AS last_source,
 coalesce(p.capability,sqlc.arg(after_capability)::text)::text AS last_capability,
 coalesce(p.output_schema,sqlc.arg(after_schema)::integer)::integer AS last_schema,
 coalesce(p.scope_key,sqlc.arg(after_scope)::text)::text AS last_scope
FROM (VALUES(1)) AS one(dummy)
LEFT JOIN LATERAL (
 SELECT utc_day,site_id,source_id,capability,output_schema,scope_key FROM picked
 ORDER BY utc_day DESC,site_id DESC,source_id DESC,capability DESC,output_schema DESC,scope_key DESC LIMIT 1
) p ON true;

-- name: YearbookCoverage :many
WITH periods AS (
 SELECT m.binding_id,m.source_id,m.source_key,m.capability,m.output_schema,m.scope_key,m.month,m.day_mask,
  m.first_confirmed_at,m.last_confirmed_at
 FROM site_monthly_coverage m
 WHERE m.site_id=sqlc.arg(site_id) AND m.month>=sqlc.arg(from_day)::date AND m.month<sqlc.arg(until_day)::date
 UNION ALL
 SELECT d.binding_id,d.source_id,s.external_key,d.capability,d.output_schema,d.scope_key,
  date_trunc('month',d.utc_day::timestamp)::date,
  (1::bigint<<(extract(day FROM d.utc_day)::integer-1)),d.confirmed_at,d.confirmed_at
 FROM site_daily_coverage d
 JOIN collection_bindings b ON b.id=d.binding_id AND b.site_id=d.site_id AND b.source_id=d.source_id
 JOIN collection_sources s ON s.id=d.source_id AND s.provider='web' AND s.resource_type=b.resource_type
 WHERE d.site_id=sqlc.arg(site_id) AND d.utc_day>=sqlc.arg(from_day)::date AND d.utc_day<sqlc.arg(until_day)::date
  AND d.capability IN ('web.http','web.dns','web.tls','web.rdap') AND d.output_schema=1 AND d.scope_key ~ '^[a-f0-9]{64}$'
  AND s.target_id IS NOT NULL AND s.resource_type=CASE WHEN d.capability IN ('web.dns','web.rdap') THEN 'domain' ELSE 'origin' END
  AND d.utc_day=(d.confirmed_at AT TIME ZONE 'UTC')::date
  AND d.confirmed_at>=b.effective_from AND (b.effective_until IS NULL OR d.confirmed_at<b.effective_until)
)
SELECT binding_id,source_id,source_key,capability,output_schema,scope_key,month,
 bit_or(day_mask)::bigint AS day_mask,min(first_confirmed_at)::timestamptz AS first_confirmed_at,
 max(last_confirmed_at)::timestamptz AS last_confirmed_at
FROM periods GROUP BY binding_id,source_id,source_key,capability,output_schema,scope_key,month
ORDER BY month,binding_id,source_id,capability,output_schema,scope_key LIMIT 4097;

-- name: YearbookFacts :many
SELECT e.*,coalesce(s.external_key,m.source_key)::text AS external_key,coalesce(b.purpose,m.binding_purpose)::text AS purpose,
 coalesce(b.effective_from,m.binding_from)::timestamptz AS effective_from,
 CASE WHEN b.id IS NOT NULL THEN b.effective_until ELSE m.binding_until END::timestamptz AS effective_until
FROM site_evidence_changes e
LEFT JOIN collection_bindings b ON b.id=e.binding_id AND b.site_id=e.site_id
LEFT JOIN collection_sources s ON s.id=b.source_id AND s.target_id=e.target_id AND s.provider='web' AND s.resource_type=b.resource_type
 AND s.resource_type=CASE WHEN e.capability IN ('web.dns','web.rdap') THEN 'domain' ELSE 'origin' END
LEFT JOIN LATERAL (
 SELECT h.source_key,h.binding_purpose,h.binding_from,h.binding_until FROM site_monthly_coverage h
 WHERE h.site_id=e.site_id AND h.binding_id=e.binding_id AND h.target_id=e.target_id
 ORDER BY h.month DESC LIMIT 1
) m ON s.id IS NULL
WHERE e.site_id=sqlc.arg(site_id) AND e.first_observed_at>=sqlc.arg(from_time) AND e.first_observed_at<sqlc.arg(until_time)
 AND e.received_at<=sqlc.arg(input_as_of)
 AND e.output_schema=1 AND e.capability IN ('web.http','web.dns','web.tls','web.rdap')
 AND (s.id IS NOT NULL OR m.source_key IS NOT NULL)
 AND e.first_observed_at>=coalesce(b.effective_from,m.binding_from)
 AND e.first_observed_at<coalesce(CASE WHEN b.id IS NOT NULL THEN b.effective_until ELSE m.binding_until END,'infinity')
ORDER BY e.first_observed_at,e.id LIMIT 4097;

-- name: YearbookPriorFacts :many
-- The latest update can carry a fact forward, but the original verified
-- baseline is still needed to qualify a year with no new material changes.
SELECT DISTINCT ON (e.binding_id,e.capability,e.scope_key,e.fact_key,(e.event_kind='baseline')) e.*,
 coalesce(s.external_key,m.source_key)::text AS external_key,coalesce(b.purpose,m.binding_purpose)::text AS purpose,
 coalesce(b.effective_from,m.binding_from)::timestamptz AS effective_from,
 CASE WHEN b.id IS NOT NULL THEN b.effective_until ELSE m.binding_until END::timestamptz AS effective_until
FROM site_evidence_changes e
LEFT JOIN collection_bindings b ON b.id=e.binding_id AND b.site_id=e.site_id
LEFT JOIN collection_sources s ON s.id=b.source_id AND s.target_id=e.target_id AND s.provider='web' AND s.resource_type=b.resource_type
 AND s.resource_type=CASE WHEN e.capability IN ('web.dns','web.rdap') THEN 'domain' ELSE 'origin' END
LEFT JOIN LATERAL (
 SELECT h.source_key,h.binding_purpose,h.binding_from,h.binding_until FROM site_monthly_coverage h
 WHERE h.site_id=e.site_id AND h.binding_id=e.binding_id AND h.target_id=e.target_id
 ORDER BY h.month DESC LIMIT 1
) m ON s.id IS NULL
WHERE e.site_id=sqlc.arg(site_id) AND e.first_observed_at<sqlc.arg(from_time)
 AND e.received_at<=sqlc.arg(input_as_of)
 AND e.output_schema=1 AND e.capability IN ('web.http','web.dns','web.tls','web.rdap')
 AND (s.id IS NOT NULL OR m.source_key IS NOT NULL)
 AND e.first_observed_at>=coalesce(b.effective_from,m.binding_from)
 AND e.first_observed_at<coalesce(CASE WHEN b.id IS NOT NULL THEN b.effective_until ELSE m.binding_until END,'infinity')
 AND coalesce(CASE WHEN b.id IS NOT NULL THEN b.effective_until ELSE m.binding_until END,'infinity')>=sqlc.arg(from_time)
ORDER BY e.binding_id,e.capability,e.scope_key,e.fact_key,(e.event_kind='baseline'),e.first_observed_at DESC,e.position DESC,e.id DESC LIMIT 257;

-- name: YearbookCandidates :many
WITH recorded AS (
 SELECT DISTINCT d.site_id,extract(year FROM d.utc_day)::integer AS year
 FROM site_daily_coverage d WHERE d.utc_day>=make_date(sqlc.arg(first_year)::integer,1,1)
 AND d.utc_day<make_date(sqlc.arg(last_year)::integer+1,1,1)
 UNION
 SELECT DISTINCT m.site_id,extract(year FROM m.month)::integer AS year
 FROM site_monthly_coverage m WHERE m.month>=make_date(sqlc.arg(first_year)::integer,1,1)
 AND m.month<make_date(sqlc.arg(last_year)::integer+1,1,1)
)
SELECT s.public_id,r.year FROM recorded r
JOIN sites s ON s.id=r.site_id AND s.status='published'
LEFT JOIN site_yearbooks y ON y.site_id=r.site_id AND y.year=r.year
WHERE NOT sqlc.arg(automatic)::boolean OR (y.published_revision IS NULL AND y.status IS DISTINCT FROM 'hidden'
 AND (y.checked_at IS NULL OR y.checked_at<sqlc.arg(recheck_before)::timestamptz))
ORDER BY y.checked_at NULLS FIRST,r.year,s.public_id LIMIT sqlc.arg(batch_size);

-- name: LockYearbookSite :one
SELECT * FROM sites WHERE public_id=$1 FOR UPDATE;

-- name: FindYearbookSite :one
SELECT * FROM sites WHERE public_id=$1;

-- name: YearbookSiteNames :many
SELECT locale,name FROM site_localizations WHERE site_id=$1 ORDER BY locale;

-- name: TryYearbookCompaction :one
SELECT pg_try_advisory_xact_lock(hashtextextended('gofurry/yearbook/compaction',0));

-- name: YearbookHead :one
SELECT * FROM site_yearbooks WHERE site_id=$1 AND year=$2;

-- name: RecordYearbookCheck :exec
INSERT INTO site_yearbooks (site_id,year,public_id,stored_slug,checked_at) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (site_id,year) DO UPDATE SET checked_at=excluded.checked_at,stored_slug=excluded.stored_slug;

-- name: YearbookRevision :one
SELECT * FROM site_yearbook_revisions WHERE site_id=$1 AND year=$2 AND revision=$3;

-- name: AddYearbookRevision :exec
INSERT INTO site_yearbook_revisions (site_id,year,revision,digest,algorithm,document,input_as_of,generated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8);

-- name: AdvanceYearbookRevision :exec
UPDATE site_yearbooks SET latest_revision=$3 WHERE site_id=$1 AND year=$2;

-- name: PublishYearbookRevision :execrows
WITH selected AS (
 UPDATE site_yearbook_revisions AS r SET first_published_at=coalesce(r.first_published_at,sqlc.arg(published_at)::timestamptz)
 WHERE r.site_id=sqlc.arg(site_id) AND r.year=sqlc.arg(year) AND r.revision=sqlc.arg(revision) AND r.digest=sqlc.arg(digest)
 RETURNING r.site_id,r.year,r.revision,r.digest
)
UPDATE site_yearbooks y SET status='published',published_revision=s.revision,published_digest=s.digest,published_at=sqlc.arg(published_at)
FROM selected s WHERE y.site_id=s.site_id AND y.year=s.year AND y.latest_revision=s.revision;

-- name: HideYearbook :execrows
UPDATE site_yearbooks SET status='hidden',checked_at=$3 WHERE public_id=$1 AND year=$2 AND published_revision IS NOT NULL;

-- name: PublishedYearbook :one
SELECT y.public_id,coalesce(s.slug,y.stored_slug)::text AS slug,y.year,r.revision,r.document,r.digest,y.published_at,
 coalesce(s.status='published',false)::boolean AS site_available,
 coalesce((SELECT jsonb_agg(jsonb_build_object('locale',l.locale,'name',l.name) ORDER BY l.locale)
  FROM site_localizations l WHERE l.site_id=s.id),'[]'::jsonb)::jsonb AS current_names
FROM site_yearbooks y LEFT JOIN sites s ON s.id=y.site_id AND s.public_id=y.public_id
JOIN site_yearbook_revisions r ON r.site_id=y.site_id AND r.year=y.year AND r.revision=y.published_revision AND r.digest=y.published_digest
WHERE y.public_id=sqlc.arg(public_id) AND y.year=sqlc.arg(year) AND y.status='published'
 AND y.year<=sqlc.arg(last_year) AND (s.id IS NULL OR s.status IN ('published','archived'));

-- name: PublishedYearbookList :many
SELECT y.year FROM site_yearbooks y LEFT JOIN sites s ON s.id=y.site_id AND s.public_id=y.public_id
JOIN site_yearbook_revisions r ON r.site_id=y.site_id AND r.year=y.year AND r.revision=y.published_revision AND r.digest=y.published_digest
WHERE y.public_id=sqlc.arg(public_id) AND y.status='published' AND y.year<=sqlc.arg(last_year)
 AND (s.id IS NULL OR s.status IN ('published','archived'))
ORDER BY y.year DESC LIMIT 64;

-- name: AuditYearbookReferences :many
SELECT 'binding_ownership'::text AS kind,count(*)::bigint AS mismatches FROM collection_bindings b
 LEFT JOIN collection_sources s ON s.id=b.source_id AND s.resource_type=b.resource_type
 LEFT JOIN site_addresses a ON a.id=b.address_id AND a.site_id=b.site_id
 LEFT JOIN game_sources g ON g.id=b.game_source_id AND g.game_id=b.game_id AND g.platform_code='steam'
 WHERE s.id IS NULL OR (b.site_id IS NOT NULL AND a.id IS NULL) OR (b.game_id IS NOT NULL AND g.id IS NULL)
UNION ALL SELECT 'plan_membership',count(*) FROM collection_plan_bindings m
 LEFT JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id
 LEFT JOIN collection_bindings b ON b.id=m.binding_id AND b.source_id=m.source_id
 WHERE p.id IS NULL OR b.id IS NULL
UNION ALL SELECT 'plan_source',count(*) FROM collection_plans p LEFT JOIN collection_sources s ON s.id=p.source_id WHERE s.id IS NULL
UNION ALL SELECT 'consumer_gap',count(*) FROM collection_gaps g LEFT JOIN collection_consumer c ON c.namespace=g.namespace WHERE c.namespace IS NULL
UNION ALL SELECT 'current_source',count(*) FROM collection_current c LEFT JOIN collection_sources s ON s.id=c.source_id WHERE s.id IS NULL
UNION ALL SELECT 'evidence_binding',count(*) FROM site_evidence_changes e LEFT JOIN collection_bindings b ON b.id=e.binding_id AND b.site_id=e.site_id
 WHERE b.id IS NULL AND NOT EXISTS (SELECT 1 FROM site_monthly_coverage m WHERE m.site_id=e.site_id AND m.binding_id=e.binding_id AND m.target_id=e.target_id)
UNION ALL SELECT 'daily_binding',count(*) FROM site_daily_coverage d LEFT JOIN collection_bindings b ON b.id=d.binding_id AND b.site_id=d.site_id AND b.source_id=d.source_id WHERE b.id IS NULL
UNION ALL SELECT 'game_news_source',count(*) FROM game_evidence_news n LEFT JOIN collection_sources s ON s.id=n.source_id AND s.provider='steam' WHERE s.id IS NULL
UNION ALL SELECT 'game_players_source',count(*) FROM game_evidence_players p LEFT JOIN collection_sources s ON s.id=p.source_id AND s.provider='steam' WHERE s.id IS NULL
UNION ALL SELECT 'published_revision',count(*) FROM site_yearbooks y LEFT JOIN site_yearbook_revisions r
 ON r.site_id=y.site_id AND r.year=y.year AND r.revision=y.published_revision AND r.digest=y.published_digest
 WHERE y.published_revision IS NOT NULL AND r.site_id IS NULL;
