-- +goose Up
CREATE TABLE management_operations (
 request_id uuid PRIMARY KEY,
 actor_id uuid NOT NULL,
 actor_name text NOT NULL CHECK(length(actor_name) BETWEEN 1 AND 80),
 actor_role text NOT NULL CHECK(actor_role IN ('owner','operator')),
 action text NOT NULL CHECK(length(action) BETWEEN 1 AND 64),
 target text NOT NULL CHECK(length(target) BETWEEN 1 AND 64),
 revision bigint NOT NULL CHECK(revision>=0),
 occurred_at timestamptz NOT NULL DEFAULT transaction_timestamp()
);

CREATE INDEX intake_operations_recent ON game_intake_candidates (updated_at DESC,id);
CREATE INDEX intake_operations_accepted ON game_intake_candidates (updated_at DESC,id) WHERE state='accepted';
CREATE INDEX collection_operations_confirmed ON collection_current (generation,confirmed_at DESC) WHERE confirmed_at IS NOT NULL;

-- +goose Down
DROP INDEX collection_operations_confirmed;
DROP INDEX intake_operations_accepted;
DROP INDEX intake_operations_recent;
DROP TABLE management_operations;
