-- +goose Up
SET LOCAL lock_timeout = '3s';

ALTER TABLE collection_bindings
 DROP CONSTRAINT collection_bindings_site_id_fkey,
 DROP CONSTRAINT collection_bindings_game_id_fkey,
 DROP CONSTRAINT collection_bindings_source_id_resource_type_fkey,
 DROP CONSTRAINT collection_bindings_address_id_site_id_fkey,
 DROP CONSTRAINT collection_bindings_game_source_id_game_id_fkey;
ALTER TABLE collection_plans DROP CONSTRAINT collection_plans_source_id_fkey;
ALTER TABLE collection_plan_bindings
 DROP CONSTRAINT collection_plan_bindings_plan_id_source_id_fkey,
 DROP CONSTRAINT collection_plan_bindings_binding_id_source_id_fkey;
ALTER TABLE collection_gaps DROP CONSTRAINT collection_gaps_namespace_fkey;
ALTER TABLE collection_current DROP CONSTRAINT collection_current_source_id_fkey;
ALTER TABLE site_evidence_changes
 DROP CONSTRAINT site_evidence_changes_site_id_fkey,
 DROP CONSTRAINT site_evidence_changes_binding_id_fkey;
ALTER TABLE site_daily_coverage
 DROP CONSTRAINT site_daily_coverage_site_id_fkey,
 DROP CONSTRAINT site_daily_coverage_source_id_fkey,
 DROP CONSTRAINT site_daily_coverage_binding_id_fkey;
ALTER TABLE game_evidence_news DROP CONSTRAINT game_evidence_news_source_id_fkey;
ALTER TABLE game_evidence_players DROP CONSTRAINT game_evidence_players_source_id_fkey;

CREATE TABLE site_monthly_coverage (
 site_id uuid NOT NULL,
 binding_id uuid NOT NULL,
 source_id uuid NOT NULL,
 target_id uuid NOT NULL,
 source_key text NOT NULL CHECK (length(source_key) BETWEEN 1 AND 512),
 binding_purpose text NOT NULL CHECK (binding_purpose IN ('primary','alternate','domain')),
 binding_from timestamptz NOT NULL,
 binding_until timestamptz CHECK (binding_until>=binding_from),
 capability text NOT NULL CHECK (capability IN ('web.http','web.dns','web.tls','web.rdap')),
 output_schema integer NOT NULL CHECK (output_schema=1),
 scope_key text NOT NULL CHECK (scope_key ~ '^[a-f0-9]{64}$'),
 month date NOT NULL CHECK (extract(day FROM month)=1),
 day_mask bigint NOT NULL CHECK (day_mask>0 AND day_mask<(1::bigint<<((month+interval '1 month')::date-month))),
 first_confirmed_at timestamptz NOT NULL,
 last_confirmed_at timestamptz NOT NULL,
 CHECK (first_confirmed_at<=last_confirmed_at AND (first_confirmed_at AT TIME ZONE 'UTC')::date>=month
        AND (last_confirmed_at AT TIME ZONE 'UTC')::date<(month+interval '1 month')::date),
 PRIMARY KEY (site_id,month,binding_id,source_id,capability,output_schema,scope_key)
);
CREATE INDEX site_monthly_coverage_binding ON site_monthly_coverage (site_id,binding_id,target_id,month DESC);

CREATE TABLE site_yearbooks (
 site_id uuid NOT NULL CHECK (uuid_extract_version(site_id)=7),
 year integer NOT NULL CHECK (year BETWEEN 2000 AND 9998),
 public_id text COLLATE "C" NOT NULL CHECK (public_id ~ '^[0123456789abcdefghjkmnpqrstvwxyz]{16}$'),
 stored_slug text NOT NULL CHECK (length(stored_slug)<=80 AND stored_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','hidden')),
 latest_revision integer NOT NULL DEFAULT 0 CHECK (latest_revision>=0),
 published_revision integer,
 published_digest text CHECK (published_digest ~ '^[a-f0-9]{64}$'),
 published_at timestamptz,
 checked_at timestamptz NOT NULL,
 CHECK ((published_revision IS NULL AND published_digest IS NULL AND published_at IS NULL)
        OR (published_revision BETWEEN 1 AND latest_revision AND published_digest IS NOT NULL AND published_at IS NOT NULL)),
 CHECK (status<>'published' OR published_revision IS NOT NULL),
 PRIMARY KEY (site_id,year),
 UNIQUE (public_id,year)
);
CREATE TABLE site_yearbook_revisions (
 site_id uuid NOT NULL,
 year integer NOT NULL CHECK (year BETWEEN 2000 AND 9998),
 revision integer NOT NULL CHECK (revision>0),
 digest text NOT NULL CHECK (digest ~ '^[a-f0-9]{64}$'),
 algorithm integer NOT NULL CHECK (algorithm>0),
 document jsonb NOT NULL CHECK (jsonb_typeof(document)='object' AND octet_length(document::text)<=65536),
 input_as_of timestamptz NOT NULL,
 generated_at timestamptz NOT NULL,
 first_published_at timestamptz,
 PRIMARY KEY (site_id,year,revision)
);
