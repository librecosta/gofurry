-- +goose Up
ALTER TABLE site_localizations ADD COLUMN body text NOT NULL DEFAULT ''
 CHECK (octet_length(body)<=10240 AND body !~ '<[!/?A-Za-z]');

ALTER TABLE collection_gaps ADD COLUMN prior_generation uuid
 CHECK (prior_generation IS NULL OR uuid_extract_version(prior_generation)=7);
