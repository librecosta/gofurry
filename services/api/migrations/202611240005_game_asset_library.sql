-- +goose Up
CREATE TABLE game_assets (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 game_id uuid NOT NULL,
 game_source_id uuid NOT NULL,
 platform_code text NOT NULL CHECK (platform_code ~ '^[a-z][a-z0-9_]{0,31}$'),
 external_key text NOT NULL CHECK (length(external_key) BETWEEN 1 AND 160),
 kind text NOT NULL CHECK (kind IN ('image','video')),
 category text NOT NULL CHECK (category ~ '^[a-z][a-z0-9_]{0,47}$'),
 source_family text NOT NULL CHECK (length(source_family) BETWEEN 1 AND 96),
 source_role text NOT NULL CHECK (source_role ~ '^[a-z][a-z0-9_]{0,63}$'),
 source_identity text NOT NULL CHECK (length(source_identity) BETWEEN 1 AND 160),
 FOREIGN KEY (game_source_id,game_id) REFERENCES game_sources(id,game_id),
 UNIQUE (game_source_id,external_key,source_family,source_role,source_identity),
 UNIQUE (id,game_id)
);
CREATE INDEX game_assets_game ON game_assets (game_id,id);

CREATE TABLE game_asset_versions (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 game_id uuid NOT NULL,
 asset_id uuid NOT NULL,
 binding_id uuid NOT NULL REFERENCES collection_bindings(id),
 observation_id uuid NOT NULL CHECK (uuid_extract_version(observation_id)=7),
 scope_key text NOT NULL CHECK (scope_key ~ '^[a-f0-9]{64}$'),
 egress_region text NOT NULL CHECK (egress_region ~ '^[a-z0-9][a-z0-9_-]{0,34}$'),
 locale text NOT NULL CHECK (length(locale) BETWEEN 1 AND 35),
 origin jsonb NOT NULL CHECK (jsonb_typeof(origin)='object' AND octet_length(origin::text)<=4096),
 name text NOT NULL CHECK (octet_length(name)<=256),
 original_order integer CHECK (original_order>=0),
 highlight boolean,
 truncated boolean,
 FOREIGN KEY (asset_id,game_id) REFERENCES game_assets(id,game_id),
 UNIQUE (asset_id,binding_id,observation_id,scope_key,egress_region),
 UNIQUE (id,game_id)
);
CREATE INDEX game_asset_versions_game_page ON game_asset_versions (game_id,id);
CREATE INDEX game_asset_versions_binding ON game_asset_versions (binding_id,observation_id);

CREATE TABLE game_asset_variants (
 id uuid PRIMARY KEY DEFAULT uuidv7() CHECK (uuid_extract_version(id)=7),
 version_id uuid NOT NULL REFERENCES game_asset_versions(id),
 variant_key text NOT NULL CHECK (variant_key ~ '^[a-z0-9][a-z0-9_]{0,47}$'),
 url text NOT NULL CHECK (url LIKE 'https://%' AND octet_length(url)<=2048),
 format text CHECK (format ~ '^[a-z0-9]{1,16}$'),
 source_role text NOT NULL CHECK (length(source_role) BETWEEN 1 AND 64),
 source_asset_id text NOT NULL CHECK (length(source_asset_id) BETWEEN 1 AND 160),
 source_slot text NOT NULL CHECK (length(source_slot) BETWEEN 1 AND 96),
 source_digest text NOT NULL CHECK (length(source_digest)<=128),
 source_family text NOT NULL CHECK (length(source_family)<=96),
 source_order integer CHECK (source_order>=0),
 width integer CHECK (width>0),
 height integer CHECK (height>0),
 UNIQUE (version_id,variant_key)
);

CREATE TABLE game_asset_library (
 game_id uuid NOT NULL,
 version_id uuid NOT NULL,
 selected_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 selected_by uuid,
 selection_origin text NOT NULL CHECK (selection_origin IN ('operator','media_v2')),
 PRIMARY KEY (game_id,version_id),
 CHECK ((selection_origin='operator')=(selected_by IS NOT NULL)),
 FOREIGN KEY (version_id,game_id) REFERENCES game_asset_versions(id,game_id)
);

-- Draft and published references have their own lifecycle. Legacy rows are migration
-- evidence only. They are never consulted by the public profile or a new publish.
CREATE TABLE game_asset_presentations (
 game_id uuid NOT NULL REFERENCES games(id),
 locale text NOT NULL CHECK (locale IN ('en','zh')),
 surface text NOT NULL CHECK (surface IN ('draft','published','legacy_draft','legacy_published')),
 revision bigint NOT NULL CHECK (revision>=0),
 PRIMARY KEY (game_id,locale,surface)
);

CREATE TABLE game_asset_presentation_items (
 game_id uuid NOT NULL,
 locale text NOT NULL,
 surface text NOT NULL,
 slot text NOT NULL CHECK (slot IN ('cover','gallery','artwork')),
 position bigint NOT NULL CHECK (position>=0),
 version_id uuid NOT NULL,
 variant_key text,
 PRIMARY KEY (game_id,locale,surface,slot,position),
 CHECK ((slot<>'cover' OR position=0) AND (slot<>'artwork' OR surface IN ('legacy_draft','legacy_published'))),
 FOREIGN KEY (game_id,locale,surface) REFERENCES game_asset_presentations(game_id,locale,surface) ON DELETE CASCADE,
 FOREIGN KEY (version_id,game_id) REFERENCES game_asset_versions(id,game_id),
 FOREIGN KEY (version_id,variant_key) REFERENCES game_asset_variants(version_id,variant_key)
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM game_assets) OR EXISTS (SELECT 1 FROM game_asset_presentations) THEN
  RAISE EXCEPTION 'unified asset library contains data';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE game_asset_presentation_items, game_asset_presentations, game_asset_library,
 game_asset_variants, game_asset_versions, game_assets;
