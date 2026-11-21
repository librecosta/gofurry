-- +goose Up
ALTER TABLE sites
 DROP CONSTRAINT sites_featured_cover_asset_key_check,
 ADD CHECK (featured_cover_asset_key ~ '^sites/covers/[0-9a-f]{64}[.](webp|avif)$'),
 DROP CONSTRAINT sites_navigation_tags_check,
 ADD CHECK (cardinality(navigation_tags)<=8 AND navigation_distinct_codes(navigation_tags)
  AND navigation_tags <@ ARRAY['forum','portfolio','fursuit','tutorial','wiki','directory','podcast','webcomic','roleplay','modding','convention']);

ALTER TABLE catalog_publication_decisions
 DROP CONSTRAINT catalog_publication_decisions_entity_kind_check,
 ADD CHECK (entity_kind IN ('site','game','collection'));
