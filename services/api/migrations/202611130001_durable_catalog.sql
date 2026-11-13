-- +goose Up
ALTER TABLE games ADD COLUMN publication_blocked boolean NOT NULL DEFAULT false;
ALTER TABLE game_intake_candidates ADD COLUMN review_revoked boolean NOT NULL DEFAULT false;
UPDATE game_intake_candidates SET review_revoked=true WHERE review_until IS NULL AND review->>'policy'='furry-and-sfw-1';

CREATE TABLE catalog_publication_decisions (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 entity_kind text NOT NULL CHECK (entity_kind IN ('site','game')),
 entity_id uuid NOT NULL CHECK (uuid_extract_version(entity_id)=7),
 revision bigint NOT NULL CHECK (revision>0),
 action text NOT NULL CHECK (action IN ('publish','unpublish','archive','restore','block','clear_block')),
 actor text NOT NULL CHECK (actor ~ '^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,79}$'),
 reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 400),
 decided_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE (entity_kind,entity_id,revision)
);

CREATE TABLE game_approved_presentations (
 game_id uuid NOT NULL CHECK (uuid_extract_version(game_id)=7),
 locale text NOT NULL CHECK (locale IN ('en','zh')),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
 summary text NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 2000),
 game_source_id uuid NOT NULL CHECK (uuid_extract_version(game_source_id)=7),
 observation_id uuid NOT NULL CHECK (uuid_extract_version(observation_id)=7),
 observed_at timestamptz NOT NULL,
 approval jsonb NOT NULL CHECK (jsonb_typeof(approval)='object' AND octet_length(approval::text)<=768),
 PRIMARY KEY (game_id,locale)
);

-- The old null expiry is an explicit revocation only when approval existed.
-- A past non-null expiry is the retired timer, including already expired rows.
WITH blocked AS (
 UPDATE games g SET publication_blocked=true,revision=g.revision+1
 WHERE g.intake_managed AND g.published_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM game_intake_candidates i WHERE i.game_id=g.id AND i.review_revoked)
  AND NOT EXISTS (SELECT 1 FROM game_intake_candidates i WHERE i.game_id=g.id AND NOT i.review_revoked
   AND i.review->>'policy'='furry-and-sfw-1' AND i.review->>'furry_ref'<>'' AND i.review->>'sfw_ref'<>'')
 RETURNING id,revision
)
INSERT INTO catalog_publication_decisions (id,entity_kind,entity_id,revision,action,actor,reason)
SELECT uuidv7(),'game',id,revision,'block','r1_migration','Inherited recorded D3-A1 review revocation' FROM blocked;

-- +goose StatementBegin
CREATE FUNCTION game_intake_presentation(work uuid, historical boolean)
RETURNS TABLE (game_id uuid,locale text,name text,summary text,game_source_id uuid,observation_id uuid,observed_at timestamptz,approval jsonb)
LANGUAGE sql STABLE AS $$
 SELECT DISTINCT ON (i.profile->>'locale') g.id,i.profile->>'locale',i.profile->>'name',i.profile->>'summary',
  i.game_source_id,i.profile_observation_id,i.profile_at,i.review
 FROM games g JOIN game_intake_candidates i ON i.game_id=g.id
 JOIN game_sources s ON s.id=i.game_source_id AND s.game_id=g.id AND s.platform_code='steam' AND s.external_key=i.external_key
 WHERE g.id=work AND i.platform_code='steam' AND i.decision_version=1 AND i.reason<>'merge_suggested'
  AND i.review->>'policy'='furry-and-sfw-1' AND i.review->>'furry_ref'<>'' AND i.review->>'sfw_ref'<>''
  AND i.review->>'furry_ref'<>i.review->>'sfw_ref'
  AND i.profile_observation_id IS NOT NULL AND i.profile_at IS NOT NULL
  AND i.profile->>'locale' IN ('en','zh') AND length(btrim(i.profile->>'name')) BETWEEN 1 AND 160
  AND length(btrim(i.profile->>'summary')) BETWEEN 1 AND 2000
  AND (historical OR (s.active AND i.state='accepted' AND NOT i.review_revoked))
 ORDER BY i.profile->>'locale',i.review_revoked,(i.game_source_id=g.intake_display_source) DESC NULLS LAST,i.created_at,i.id
$$;
-- +goose StatementEnd

INSERT INTO game_approved_presentations
SELECT p.* FROM games g CROSS JOIN LATERAL game_intake_presentation(g.id,true) p WHERE g.published_at IS NOT NULL
ON CONFLICT DO NOTHING;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION game_presentation(work uuid, preferred uuid)
RETURNS jsonb LANGUAGE sql STABLE AS $$
 WITH display AS (
  SELECT coalesce(e.locale,a.locale) AS locale,coalesce(e.name,a.name) AS name,coalesce(e.summary,a.summary) AS summary
  FROM (SELECT l.locale,l.name,l.summary FROM game_localizations l WHERE l.game_id=work) e
  FULL JOIN (SELECT p.locale,p.name,p.summary FROM game_approved_presentations p WHERE p.game_id=work) a USING (locale)
 )
 SELECT jsonb_build_object('localizations',
  coalesce((SELECT jsonb_agg(jsonb_build_object('locale',locale,'name',name,'summary',summary) ORDER BY locale) FROM display),'[]'::jsonb))
$$;
-- +goose StatementEnd

CREATE OR REPLACE VIEW site_public_navigation AS SELECT id,public_id,slug,status,revision,created_at,updated_at,published_at,
 archived_at,first_published_at,content_rating,legacy_detail,primary_category,secondary_categories,navigation_tags,supported_languages
 FROM sites WHERE status='published' AND content_rating='sfw';

DROP INDEX sites_navigation_recent,sites_navigation_primary,sites_navigation_secondary,sites_navigation_tags,sites_navigation_languages;
CREATE INDEX sites_navigation_recent ON sites (first_published_at DESC NULLS LAST,public_id)
 WHERE status='published' AND content_rating='sfw';
CREATE INDEX sites_navigation_primary ON sites (primary_category)
 WHERE status='published' AND content_rating='sfw';
CREATE INDEX sites_navigation_secondary ON sites USING gin (secondary_categories)
 WHERE status='published' AND content_rating='sfw';
CREATE INDEX sites_navigation_tags ON sites USING gin (navigation_tags)
 WHERE status='published' AND content_rating='sfw';
CREATE INDEX sites_navigation_languages ON sites USING gin (supported_languages)
 WHERE status='published' AND content_rating='sfw';

CREATE TABLE site_technology_snapshots (
 binding_id uuid NOT NULL CHECK (uuid_extract_version(binding_id)=7),
 generation uuid NOT NULL CHECK (uuid_extract_version(generation)=7),
 capability text NOT NULL,
 scope_key text NOT NULL CHECK (scope_key ~ '^[0-9a-f]{64}$'),
 section text NOT NULL CHECK (section IN ('http','html','tls')),
 position bigint NOT NULL,
 state text NOT NULL CHECK (state IN ('present','withdrawn','confirmed_empty')),
 observed_at timestamptz NOT NULL,
 last_observed_at timestamptz NOT NULL,
 ruleset text NOT NULL,
 facts jsonb NOT NULL CHECK (jsonb_typeof(facts)='object' AND octet_length(facts::text)<=8192),
 PRIMARY KEY (binding_id,capability,scope_key)
);

-- Reuse the same attribution rules at migration, on Consumer writes and for
-- the live read. Binding intervals prevent a shared Target leaking evidence.
-- +goose StatementBegin
CREATE FUNCTION site_technology_scopes(gen uuid, source_filter uuid, scope_filter text, binding_filter uuid)
RETURNS SETOF site_technology_snapshots LANGUAGE sql STABLE AS $$
 SELECT DISTINCT ON (b.id,c.capability,c.scope_key) b.id,c.generation,c.capability,c.scope_key,
  c.scope#>>'{dimensions,section}',c.position,c.state,c.observed_at,coalesce(c.confirmed_at,c.received_at),c.ruleset,
  coalesce((SELECT jsonb_object_agg(f.key,f.value) FROM jsonb_each(c.facts) f
   WHERE f.key IN ('http.response','tls.certificate') OR f.key LIKE 'tech.detected.%'),'{}'::jsonb)
 FROM collection_current c JOIN collection_sources src ON src.id=c.source_id
 JOIN collection_bindings b ON b.source_id=src.id
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=c.capability
 WHERE c.generation=gen AND (source_filter IS NULL OR c.source_id=source_filter)
  AND (scope_filter IS NULL OR c.scope_key=scope_filter) AND (binding_filter IS NULL OR b.id=binding_filter)
  AND src.provider='web' AND src.resource_type='origin' AND src.target_id IS NOT NULL
  AND b.site_id IS NOT NULL AND b.resource_type='origin' AND b.purpose='primary'
  AND c.output_schema=1 AND p.request_schema=1
  AND c.scope->>'namespace'=src.namespace AND c.scope->>'egress_region'~'^[a-z0-9][a-z0-9_-]{0,34}$'
  AND p.scope=c.scope-'namespace'-'dimensions'
  AND c.scope->'dimensions'=jsonb_build_object('section',c.scope#>>'{dimensions,section}')
  AND ((c.capability='web.http' AND c.scope#>>'{dimensions,section}' IN ('http','html','tls'))
   OR (c.capability='web.tls' AND c.scope#>>'{dimensions,section}'='tls'))
  AND (c.state<>'present' OR (c.observation_id IS NOT NULL AND c.confirmed_at IS NOT NULL))
  AND coalesce(c.confirmed_at,c.received_at) BETWEEN greatest(b.effective_from,m.attached_at) AND statement_timestamp()
  AND (b.effective_until IS NULL OR coalesce(c.confirmed_at,c.received_at)<b.effective_until)
  AND (c.state<>'present' OR c.observed_at<=c.confirmed_at)
 ORDER BY b.id,c.capability,c.scope_key
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION save_site_technology(gen uuid, source_filter uuid, scope_filter text)
RETURNS void LANGUAGE sql AS $$
 INSERT INTO site_technology_snapshots
 SELECT * FROM site_technology_scopes(gen,source_filter,scope_filter,NULL)
 ON CONFLICT (binding_id,capability,scope_key) DO UPDATE SET generation=excluded.generation,
  section=excluded.section,position=excluded.position,state=excluded.state,observed_at=excluded.observed_at,
  last_observed_at=excluded.last_observed_at,ruleset=excluded.ruleset,facts=excluded.facts
 WHERE excluded.last_observed_at>site_technology_snapshots.last_observed_at
  OR (excluded.last_observed_at=site_technology_snapshots.last_observed_at
   AND (excluded.generation<>site_technology_snapshots.generation OR excluded.position>site_technology_snapshots.position))
$$;
-- +goose StatementEnd

SELECT save_site_technology(c.generation,NULL,NULL) FROM collection_consumer c WHERE c.generation IS NOT NULL;
