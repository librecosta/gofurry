-- +goose Up
ALTER TABLE collection_plans
 ADD COLUMN interval_override_seconds integer NOT NULL DEFAULT 0 CHECK (interval_override_seconds=0 OR interval_override_seconds BETWEEN 900 AND 2592000),
 ADD COLUMN execution_policy jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(execution_policy)='object' AND octet_length(execution_policy::text)<=512),
 ADD COLUMN config_revision bigint NOT NULL DEFAULT 1 CHECK (config_revision>0),
 ADD COLUMN synced_config_revision bigint NOT NULL DEFAULT 1 CHECK (synced_config_revision BETWEEN 1 AND config_revision),
 ADD COLUMN config_sync_error text NOT NULL DEFAULT '' CHECK (length(config_sync_error)<=64);

-- +goose Down
ALTER TABLE collection_plans DROP COLUMN config_sync_error, DROP COLUMN synced_config_revision, DROP COLUMN config_revision,
 DROP COLUMN execution_policy, DROP COLUMN interval_override_seconds;
