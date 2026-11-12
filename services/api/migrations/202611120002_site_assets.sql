-- +goose Up
ALTER TABLE sites
 ADD COLUMN logo_asset_key text CHECK (logo_asset_key ~ '^sites/logos/[0-9a-f]{64}[.]webp$'),
 ADD COLUMN featured_cover_asset_key text CHECK (featured_cover_asset_key ~ '^sites/covers/[0-9a-f]{64}[.]avif$');
