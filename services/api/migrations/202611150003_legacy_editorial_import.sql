-- +goose Up
CREATE TABLE legacy_import_batches (
 plan_sha256 text PRIMARY KEY CHECK (plan_sha256 ~ '^[a-f0-9]{64}$'),
 legacy_deployment text NOT NULL CHECK (legacy_deployment = 'gofurry_nav_site_cn'),
 backup_sha256 text NOT NULL CHECK (backup_sha256 ~ '^[a-f0-9]{64}$'),
 content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[a-f0-9]{64}$'),
 decisions_sha256 text NOT NULL CHECK (decisions_sha256 ~ '^[a-f0-9]{64}$'),
 rule_version integer NOT NULL CHECK (rule_version = 1),
 executed_by name NOT NULL DEFAULT current_user,
 started_at timestamptz NOT NULL DEFAULT transaction_timestamp()
);

CREATE TABLE legacy_site_ids (
 legacy_deployment text NOT NULL,
 legacy_id bigint NOT NULL CHECK (legacy_id > 0),
 target_id uuid NOT NULL REFERENCES sites(id) ON DELETE RESTRICT,
 public_id_at_link text NOT NULL,
 first_plan_sha256 text NOT NULL,
 origin text NOT NULL CHECK (origin IN ('created','linked','alias')),
 decision_sha256 text NOT NULL,
 verified_at timestamptz,
 verified_by text,
 PRIMARY KEY (legacy_deployment,legacy_id),
 CHECK ((verified_at IS NULL)=(verified_by IS NULL))
);
CREATE INDEX legacy_site_target ON legacy_site_ids (legacy_deployment,target_id);

CREATE TABLE legacy_game_ids (
 legacy_deployment text NOT NULL,
 legacy_id bigint NOT NULL CHECK (legacy_id > 0),
 target_id uuid NOT NULL REFERENCES games(id) ON DELETE RESTRICT,
 public_id_at_link text NOT NULL,
 first_plan_sha256 text NOT NULL,
 origin text NOT NULL CHECK (origin IN ('created','linked','alias')),
 decision_sha256 text NOT NULL,
 verified_at timestamptz,
 verified_by text,
 PRIMARY KEY (legacy_deployment,legacy_id),
 CHECK ((verified_at IS NULL)=(verified_by IS NULL))
);
CREATE INDEX legacy_game_target ON legacy_game_ids (legacy_deployment,target_id);

CREATE TABLE legacy_editorial_ids (
 legacy_deployment text NOT NULL,
 kind text NOT NULL CHECK (kind IN ('collection','tag_category','tag')),
 legacy_id bigint NOT NULL CHECK (legacy_id > 0),
 target_id uuid NOT NULL CHECK (uuid_extract_version(target_id)=7),
 public_id_at_link text NOT NULL,
 first_plan_sha256 text NOT NULL,
 origin text NOT NULL CHECK (origin IN ('created','linked')),
 decision_sha256 text NOT NULL,
 verified_at timestamptz,
 verified_by text,
 PRIMARY KEY (legacy_deployment,kind,legacy_id),
 UNIQUE (legacy_deployment,kind,target_id),
 CHECK ((verified_at IS NULL)=(verified_by IS NULL))
);

CREATE VIEW legacy_editorial_mappings AS
SELECT 'site'::text AS kind,legacy_deployment,legacy_id,target_id,public_id_at_link,origin,first_plan_sha256 FROM legacy_site_ids
UNION ALL SELECT 'game',legacy_deployment,legacy_id,target_id,public_id_at_link,origin,first_plan_sha256 FROM legacy_game_ids
UNION ALL SELECT kind,legacy_deployment,legacy_id,target_id,public_id_at_link,origin,first_plan_sha256 FROM legacy_editorial_ids;

CREATE TABLE legacy_import_receipts (
 plan_sha256 text NOT NULL,
 source_key text NOT NULL CHECK(length(source_key) BETWEEN 1 AND 160),
 phase text NOT NULL CHECK (phase IN ('identity','relations','logo','lifecycle')),
 intent_sha256 text NOT NULL CHECK(intent_sha256 ~ '^[a-f0-9]{64}$'),
 decision_sha256 text NOT NULL CHECK(decision_sha256 ~ '^[a-f0-9]{64}$'),
 outcome text NOT NULL CHECK(outcome IN ('created','linked','alias','verified_existing','applied')),
 target_id uuid NOT NULL CHECK(uuid_extract_version(target_id)=7),
 public_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 post_sha256 text NOT NULL CHECK(post_sha256 ~ '^[a-f0-9]{64}$'),
 committed_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 PRIMARY KEY(plan_sha256,source_key,phase)
);
CREATE INDEX legacy_receipt_progress ON legacy_import_receipts (source_key,phase,target_id,revision DESC,committed_at DESC);

CREATE TABLE game_editorial_tag_categories (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 code text NOT NULL UNIQUE CHECK (length(code)<=64 AND code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 localizations jsonb NOT NULL CHECK (jsonb_typeof(localizations)='array' AND jsonb_array_length(localizations)<=2 AND octet_length(localizations::text)<=19000),
 sort_order integer NOT NULL,
 archived_at timestamptz
);
CREATE TABLE game_editorial_tags (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 category_id uuid NOT NULL CHECK (uuid_extract_version(category_id)=7),
 code text NOT NULL UNIQUE CHECK (length(code)<=64 AND code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 localizations jsonb NOT NULL CHECK (jsonb_typeof(localizations)='array' AND jsonb_array_length(localizations)<=2 AND octet_length(localizations::text)<=19000),
 archived_at timestamptz
);
CREATE INDEX game_editorial_tags_category ON game_editorial_tags(category_id,id);
CREATE TABLE game_editorial_tag_members (
 game_id uuid NOT NULL CHECK(uuid_extract_version(game_id)=7),
 tag_id uuid NOT NULL CHECK(uuid_extract_version(tag_id)=7),
 role text NOT NULL CHECK(role IN ('normal','primary','secondary')),
 PRIMARY KEY(game_id,tag_id)
);
CREATE UNIQUE INDEX game_editorial_tag_roles ON game_editorial_tag_members (game_id,role) WHERE role IN ('primary','secondary');
CREATE INDEX game_editorial_tag_matches ON game_editorial_tag_members (tag_id,game_id);
CREATE TABLE game_collection_tag_rules (
 collection_id uuid NOT NULL CHECK(uuid_extract_version(collection_id)=7),
 tag_id uuid NOT NULL CHECK(uuid_extract_version(tag_id)=7),
 PRIMARY KEY(collection_id,tag_id)
);
CREATE INDEX game_collection_tag_matches ON game_collection_tag_rules (tag_id,collection_id);
CREATE TABLE game_collection_exclusions (
 collection_id uuid NOT NULL CHECK(uuid_extract_version(collection_id)=7),
 game_id uuid NOT NULL CHECK(uuid_extract_version(game_id)=7),
 PRIMARY KEY(collection_id,game_id)
);

-- Separate derived membership revisions from the editorial preimage. Both
-- contribute to the public cursor and to Management's optimistic lock.
ALTER TABLE game_collections ADD COLUMN derived_membership_revision bigint NOT NULL DEFAULT 0
 CHECK (derived_membership_revision BETWEEN 0 AND revision);

CREATE VIEW game_collection_effective_members AS
SELECT m.collection_id,m.game_id,min(m.position)::integer AS position
FROM (
 SELECT collection_id,game_id,position FROM game_collection_members
 UNION ALL
 SELECT r.collection_id,a.game_id,NULL::integer
 FROM game_collection_tag_rules r
 JOIN game_editorial_tags t ON t.id=r.tag_id AND t.archived_at IS NULL
 JOIN game_editorial_tag_categories c ON c.id=t.category_id AND c.archived_at IS NULL
 JOIN game_editorial_tag_members a ON a.tag_id=r.tag_id
) m
WHERE NOT EXISTS (SELECT 1 FROM game_collection_exclusions x
 WHERE x.collection_id=m.collection_id AND x.game_id=m.game_id)
GROUP BY m.collection_id,m.game_id;

-- +goose StatementBegin
CREATE FUNCTION bump_editorial_collection_revisions() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ids uuid[];
BEGIN
 IF TG_TABLE_NAME IN ('game_collection_tag_rules','game_collection_exclusions') THEN
  ids:=ARRAY[CASE WHEN TG_OP<>'DELETE' THEN NEW.collection_id END,CASE WHEN TG_OP<>'INSERT' THEN OLD.collection_id END];
 ELSIF TG_TABLE_NAME='game_editorial_tag_members' THEN
  SELECT array_agg(DISTINCT collection_id) INTO ids FROM game_collection_tag_rules
   WHERE tag_id IN (CASE WHEN TG_OP<>'DELETE' THEN NEW.tag_id END,CASE WHEN TG_OP<>'INSERT' THEN OLD.tag_id END);
 ELSIF TG_TABLE_NAME='game_editorial_tags' THEN
  SELECT array_agg(DISTINCT collection_id) INTO ids FROM game_collection_tag_rules WHERE tag_id=NEW.id;
 ELSE
  SELECT array_agg(DISTINCT r.collection_id) INTO ids FROM game_collection_tag_rules r
   JOIN game_editorial_tags t ON t.id=r.tag_id WHERE t.category_id=NEW.id;
 END IF;
 PERFORM id FROM game_collections WHERE id=ANY(ids) ORDER BY id FOR UPDATE;
 UPDATE game_collections SET revision=revision+1,updated_at=transaction_timestamp(),
  derived_membership_revision=derived_membership_revision+
   CASE WHEN TG_TABLE_NAME IN ('game_collection_tag_rules','game_collection_exclusions') THEN 0 ELSE 1 END
 WHERE id=ANY(ids);
 RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER editorial_rules_revision AFTER INSERT OR UPDATE OR DELETE ON game_collection_tag_rules
 FOR EACH ROW EXECUTE FUNCTION bump_editorial_collection_revisions();
CREATE TRIGGER editorial_exclusions_revision AFTER INSERT OR UPDATE OR DELETE ON game_collection_exclusions
 FOR EACH ROW EXECUTE FUNCTION bump_editorial_collection_revisions();
CREATE TRIGGER editorial_assignments_revision AFTER INSERT OR UPDATE OR DELETE ON game_editorial_tag_members
 FOR EACH ROW EXECUTE FUNCTION bump_editorial_collection_revisions();
CREATE TRIGGER editorial_categories_revision AFTER UPDATE OF archived_at ON game_editorial_tag_categories
 FOR EACH ROW WHEN (OLD.archived_at IS DISTINCT FROM NEW.archived_at) EXECUTE FUNCTION bump_editorial_collection_revisions();
CREATE TRIGGER editorial_tags_revision AFTER UPDATE OF archived_at,category_id ON game_editorial_tags
 FOR EACH ROW WHEN ((OLD.archived_at,OLD.category_id) IS DISTINCT FROM (NEW.archived_at,NEW.category_id))
 EXECUTE FUNCTION bump_editorial_collection_revisions();

-- +goose Down
DROP TRIGGER editorial_tags_revision ON game_editorial_tags;
DROP TRIGGER editorial_categories_revision ON game_editorial_tag_categories;
DROP TRIGGER editorial_assignments_revision ON game_editorial_tag_members;
DROP TRIGGER editorial_exclusions_revision ON game_collection_exclusions;
DROP TRIGGER editorial_rules_revision ON game_collection_tag_rules;
DROP FUNCTION bump_editorial_collection_revisions();
DROP VIEW game_collection_effective_members;
ALTER TABLE game_collections DROP COLUMN derived_membership_revision;
DROP TABLE game_collection_exclusions,game_collection_tag_rules,game_editorial_tag_members,game_editorial_tags,game_editorial_tag_categories;
DROP VIEW legacy_editorial_mappings;
DROP TABLE legacy_import_receipts,legacy_editorial_ids,legacy_site_ids,legacy_game_ids,legacy_import_batches;
