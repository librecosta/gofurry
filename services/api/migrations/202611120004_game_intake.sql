-- +goose Up
ALTER TABLE games ADD COLUMN intake_managed boolean NOT NULL DEFAULT false;
ALTER TABLE games ADD COLUMN intake_display_source uuid CHECK (uuid_extract_version(intake_display_source)=7);

CREATE TABLE game_intake_candidates (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 platform_code text NOT NULL CHECK (platform_code IN ('steam','itch_io')),
 external_key text NOT NULL CHECK (length(external_key) BETWEEN 1 AND 160),
 discovered_via text NOT NULL CHECK (discovered_via IN ('operator','discovery','reviewed_link')),
 state text NOT NULL DEFAULT 'discovered' CHECK (state IN ('discovered','resolving','accepted','deferred','rejected')),
 reason text NOT NULL DEFAULT '' CHECK (reason ~ '^[a-z_]{0,48}$'),
 next_at timestamptz DEFAULT transaction_timestamp(),
 lease_id uuid CHECK (uuid_extract_version(lease_id)=7),
 lease_until timestamptz,
 failures integer NOT NULL DEFAULT 0 CHECK (failures BETWEEN 0 AND 8),
 namespace text NOT NULL DEFAULT '' CHECK (length(namespace)<=63),
 egress_region text NOT NULL DEFAULT '' CHECK (length(egress_region)<=35),
 target_id uuid CHECK (uuid_extract_version(target_id)=7),
 job_started_at timestamptz,
 job_writer_binding text NOT NULL DEFAULT '' CHECK (job_writer_binding='' OR job_writer_binding ~ '^[0-9a-f]{64}$'),
 job_round integer NOT NULL DEFAULT 1 CHECK (job_round BETWEEN 1 AND 2),
 job_id uuid CHECK (uuid_extract_version(job_id)=7),
 attempt_id uuid CHECK (uuid_extract_version(attempt_id)=7),
 observation_id uuid CHECK (uuid_extract_version(observation_id)=7),
 observation_hash text NOT NULL DEFAULT '' CHECK (observation_hash='' OR observation_hash ~ '^[0-9a-f]{64}$'),
 game_id uuid CHECK (uuid_extract_version(game_id)=7),
 game_source_id uuid CHECK (uuid_extract_version(game_source_id)=7),
 profile_scope_key text NOT NULL DEFAULT '' CHECK (profile_scope_key='' OR profile_scope_key ~ '^[0-9a-f]{64}$'),
 profile jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(profile)='object' AND octet_length(profile::text)<=3600),
 profile_at timestamptz,
 profile_observation_id uuid CHECK (uuid_extract_version(profile_observation_id)=7),
 work_title text NOT NULL DEFAULT '' CHECK (octet_length(work_title)<=192),
 relation jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(relation)='object' AND octet_length(relation::text)<=1600),
 relation_attached boolean NOT NULL DEFAULT false,
 decision_version integer NOT NULL DEFAULT 0 CHECK (decision_version BETWEEN 0 AND 1),
 review jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(review)='object' AND octet_length(review::text)<=768),
 review_until timestamptz,
 published_at timestamptz,
 decided_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE (platform_code,external_key),
 CHECK ((state='resolving')=(lease_id IS NOT NULL AND lease_until IS NOT NULL)),
 CHECK ((lease_id IS NULL)=(lease_until IS NULL)),
 CHECK (state<>'accepted' OR (game_id IS NOT NULL AND game_source_id IS NOT NULL)),
 CHECK (job_id IS NULL OR (target_id IS NOT NULL AND job_started_at IS NOT NULL)),
 CHECK (attempt_id IS NULL OR job_id IS NOT NULL)
);
CREATE INDEX game_intake_due ON game_intake_candidates (next_at,id)
 WHERE state IN ('discovered','deferred','resolving') AND next_at IS NOT NULL;
CREATE INDEX game_intake_lease ON game_intake_candidates (lease_until) WHERE state='resolving';
CREATE INDEX game_intake_game ON game_intake_candidates (game_id) WHERE state='accepted';
CREATE INDEX game_intake_title ON game_intake_candidates (work_title) WHERE state='accepted' AND work_title<>'';
CREATE INDEX game_intake_relation ON game_intake_candidates ((relation->>'platform'),(relation->>'key'))
 WHERE state='deferred' AND reason='related_source_unavailable' AND next_at IS NULL;
CREATE INDEX game_intake_abandoned ON game_intake_candidates (updated_at)
 WHERE state='deferred' AND next_at IS NULL AND job_started_at IS NULL AND target_id IS NULL
   AND review='{}' AND relation='{}';

-- The same read policy is used by publication, detail and pagination. A
-- candidate supplies a locale only while its approved source remains usable.
-- +goose StatementBegin
CREATE FUNCTION game_presentation(work uuid, preferred uuid)
RETURNS jsonb LANGUAGE sql STABLE AS $$
 WITH available AS (
   SELECT DISTINCT ON (i.profile->>'locale')
     i.profile->>'locale' AS locale,i.profile->>'name' AS name,i.profile->>'summary' AS summary
   FROM game_intake_candidates i JOIN game_sources s ON s.id=i.game_source_id AND s.game_id=i.game_id
   WHERE i.game_id=work AND i.state='accepted' AND s.active AND s.platform_code='steam'
     AND s.source_url<>'' AND i.review_until>statement_timestamp()
     AND i.review->>'furry_ref'<>'' AND i.review->>'sfw_ref'<>''
     AND i.review->>'policy'='furry-and-sfw-1'
     AND i.decision_version=1 AND i.profile_observation_id IS NOT NULL
     AND i.profile_at>statement_timestamp()-interval '30 days'
     AND i.profile->>'locale' IN ('en','zh') AND i.profile->>'name'<>'' AND i.profile->>'summary'<>''
   ORDER BY i.profile->>'locale', (i.game_source_id=preferred) DESC NULLS LAST, i.created_at,i.id
 ), display AS (
   SELECT coalesce(e.locale,a.locale) AS locale,coalesce(e.name,a.name) AS name,coalesce(e.summary,a.summary) AS summary
   FROM (SELECT l.locale,l.name,l.summary FROM game_localizations l WHERE l.game_id=work) e
   FULL JOIN available a USING (locale)
 )
 SELECT jsonb_build_object('approved',EXISTS(SELECT 1 FROM available),'localizations',
   coalesce((SELECT jsonb_agg(jsonb_build_object('locale',locale,'name',name,'summary',summary) ORDER BY locale) FROM display),'[]'::jsonb))
$$;
-- +goose StatementEnd
