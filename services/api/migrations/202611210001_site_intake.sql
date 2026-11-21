-- +goose Up
LOCK TABLE sites IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM sites WHERE cardinality(supported_languages)>0) THEN
  RAISE EXCEPTION 'Export and review existing site supported_languages before applying this migration';
 END IF;
END;
$$;
-- +goose StatementEnd

DROP VIEW site_public_navigation,site_public_history;
DROP INDEX sites_navigation_languages;
ALTER TABLE sites DROP COLUMN supported_languages;

CREATE VIEW site_public_history AS
 SELECT id,public_id,slug,status,revision,created_at,updated_at,published_at,archived_at,
  first_published_at,content_rating,legacy_detail,primary_category,secondary_categories,navigation_tags
 FROM sites WHERE (content_rating='sfw' OR (content_rating='unreviewed' AND legacy_detail))
  AND status IN ('published','archived');
CREATE VIEW site_public_navigation AS
 SELECT id,public_id,slug,status,revision,created_at,updated_at,published_at,archived_at,
  first_published_at,content_rating,legacy_detail,primary_category,secondary_categories,navigation_tags
 FROM sites WHERE status='published' AND content_rating='sfw';

CREATE INDEX site_addresses_active_url ON site_addresses (url) WHERE active;
