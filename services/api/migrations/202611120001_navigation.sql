-- +goose Up
CREATE FUNCTION navigation_distinct_codes(codes text[]) RETURNS boolean
 LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
 RETURN cardinality(codes)=(SELECT count(DISTINCT code) FROM unnest(codes) AS item(code));

ALTER TABLE sites
 ADD COLUMN first_published_at timestamptz,
 ADD COLUMN content_rating text NOT NULL DEFAULT 'unreviewed' CHECK (content_rating IN ('unreviewed','sfw','mixed','adult')),
 ADD COLUMN legacy_detail boolean NOT NULL DEFAULT false,
 ADD COLUMN primary_category text CHECK (primary_category IN ('community','gallery','literature','resources','marketplace','knowledge','gaming','events')),
 ADD COLUMN secondary_categories text[] NOT NULL DEFAULT '{}',
 ADD COLUMN navigation_tags text[] NOT NULL DEFAULT '{}',
 ADD COLUMN supported_languages text[] NOT NULL DEFAULT '{}',
 ADD CHECK (NOT legacy_detail OR content_rating='unreviewed'),
 ADD CHECK (cardinality(secondary_categories)<=2 AND navigation_distinct_codes(secondary_categories)
  AND (primary_category IS NOT NULL OR cardinality(secondary_categories)=0)
  AND secondary_categories <@ ARRAY['community','gallery','literature','resources','marketplace','knowledge','gaming','events']
  AND (primary_category IS NULL OR NOT (primary_category=ANY(secondary_categories)))),
 ADD CHECK (cardinality(navigation_tags)<=8 AND navigation_distinct_codes(navigation_tags)
  AND navigation_tags <@ ARRAY['forum','portfolio','fursuit','tutorial','wiki','directory','podcast','webcomic','roleplay','modding','convention','translation']),
 ADD CHECK (cardinality(supported_languages)<=8 AND navigation_distinct_codes(supported_languages)
  AND array_to_string(supported_languages,',') ~ '^([a-z]{2,3}(,[a-z]{2,3})*)?$');

-- Keep existing D1 details and published archives reachable pending review.
-- Never infer a rating, category, supported language or first publication date.
UPDATE sites SET legacy_detail=true WHERE published_at IS NOT NULL AND status IN ('published','archived');

CREATE VIEW site_public_history AS
 SELECT * FROM sites WHERE (content_rating='sfw' OR (content_rating='unreviewed' AND legacy_detail))
  AND status IN ('published','archived');
CREATE VIEW site_public_navigation AS
 SELECT * FROM sites WHERE status='published' AND content_rating='sfw' AND primary_category IS NOT NULL;

CREATE INDEX sites_navigation_recent ON sites (first_published_at DESC NULLS LAST,public_id)
 WHERE status='published' AND content_rating='sfw' AND primary_category IS NOT NULL;
CREATE INDEX sites_navigation_primary ON sites (primary_category)
 WHERE status='published' AND content_rating='sfw' AND primary_category IS NOT NULL;
CREATE INDEX sites_navigation_secondary ON sites USING gin (secondary_categories)
 WHERE status='published' AND content_rating='sfw' AND primary_category IS NOT NULL;
CREATE INDEX sites_navigation_tags ON sites USING gin (navigation_tags)
 WHERE status='published' AND content_rating='sfw' AND primary_category IS NOT NULL;
CREATE INDEX sites_navigation_languages ON sites USING gin (supported_languages)
 WHERE status='published' AND content_rating='sfw' AND primary_category IS NOT NULL;

CREATE TABLE site_navigation_reviews (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 site_id uuid NOT NULL CHECK (uuid_extract_version(site_id)=7),
 site_revision bigint NOT NULL CHECK (site_revision>0),
 previous_rating text NOT NULL CHECK (previous_rating IN ('unreviewed','sfw','mixed','adult')),
 rating text NOT NULL CHECK (rating IN ('unreviewed','sfw','mixed','adult')),
 actor text NOT NULL CHECK (actor ~ '^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,79}$'),
 reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 400),
 reviewed_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE (site_id,site_revision)
);

CREATE TABLE site_navigation_features (
 slot text PRIMARY KEY CHECK (slot IN ('primary','secondary_1','secondary_2')),
 site_id uuid UNIQUE CHECK (uuid_extract_version(site_id)=7),
 revision bigint NOT NULL DEFAULT 0 CHECK (revision>=0),
 actor text,
 reason text,
 updated_at timestamptz,
 CHECK ((revision=0 AND site_id IS NULL AND actor IS NULL AND reason IS NULL AND updated_at IS NULL)
  OR (revision>0 AND actor IS NOT NULL AND reason IS NOT NULL AND updated_at IS NOT NULL))
);
INSERT INTO site_navigation_features (slot) VALUES ('primary'),('secondary_1'),('secondary_2');

CREATE TABLE site_interaction_days (
 site_id uuid NOT NULL CHECK (uuid_extract_version(site_id)=7),
 utc_day date NOT NULL,
 detail_views integer NOT NULL DEFAULT 0 CHECK (detail_views BETWEEN 0 AND 1000000000),
 outbound_clicks integer NOT NULL DEFAULT 0 CHECK (outbound_clicks BETWEEN 0 AND 1000000000),
 CHECK (detail_views>0 OR outbound_clicks>0),
 PRIMARY KEY (site_id,utc_day)
);
CREATE INDEX site_interaction_window ON site_interaction_days (utc_day,site_id);

CREATE TABLE site_navigation_heat_state (
 singleton boolean PRIMARY KEY CHECK (singleton),
 algorithm integer NOT NULL CHECK (algorithm>0),
 window_end date NOT NULL,
 computed_at timestamptz NOT NULL,
 aggregate_cutoff timestamptz NOT NULL
);
CREATE TABLE site_navigation_heat (
 site_id uuid PRIMARY KEY CHECK (uuid_extract_version(site_id)=7),
 score bigint NOT NULL CHECK (score>0)
);
CREATE INDEX site_navigation_heat_rank ON site_navigation_heat (score DESC,site_id);
