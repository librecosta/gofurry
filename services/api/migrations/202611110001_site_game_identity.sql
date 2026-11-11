-- +goose Up
CREATE TABLE sites (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 public_id text COLLATE "C" NOT NULL CONSTRAINT sites_public_id_key UNIQUE CHECK (public_id ~ '^[0123456789abcdefghjkmnpqrstvwxyz]{16}$'),
 slug text NOT NULL CHECK (length(slug) BETWEEN 1 AND 80 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 published_at timestamptz,
 archived_at timestamptz,
 CHECK (status<>'published' OR published_at IS NOT NULL),
 CHECK ((status='archived')=(archived_at IS NOT NULL))
);
CREATE INDEX sites_published_public_id ON sites (public_id) WHERE status='published';

CREATE TABLE site_localizations (
 site_id uuid NOT NULL REFERENCES sites(id) ON DELETE RESTRICT,
 locale text NOT NULL CHECK (locale IN ('zh','en')),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
 summary text NOT NULL CHECK (length(summary)<=2000),
 PRIMARY KEY (site_id,locale)
);

CREATE TABLE site_addresses (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 site_id uuid NOT NULL REFERENCES sites(id) ON DELETE RESTRICT,
 url text NOT NULL CHECK (length(url)<=2048 AND url ~ '^https?://'),
 kind text NOT NULL CHECK (kind IN ('primary','alternate')),
 active boolean NOT NULL DEFAULT true,
 UNIQUE (site_id,url)
);
CREATE UNIQUE INDEX site_addresses_active_primary ON site_addresses (site_id) WHERE active AND kind='primary';

CREATE TABLE games (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 public_id text COLLATE "C" NOT NULL CONSTRAINT games_public_id_key UNIQUE CHECK (public_id ~ '^[0123456789abcdefghjkmnpqrstvwxyz]{16}$'),
 slug text NOT NULL CHECK (length(slug) BETWEEN 1 AND 80 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 published_at timestamptz,
 archived_at timestamptz,
 CHECK (status<>'published' OR published_at IS NOT NULL),
 CHECK ((status='archived')=(archived_at IS NOT NULL))
);
CREATE INDEX games_published_public_id ON games (public_id) WHERE status='published';

CREATE TABLE game_localizations (
 game_id uuid NOT NULL REFERENCES games(id) ON DELETE RESTRICT,
 locale text NOT NULL CHECK (locale IN ('zh','en')),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
 summary text NOT NULL CHECK (length(summary)<=2000),
 PRIMARY KEY (game_id,locale)
);

CREATE TABLE game_sources (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 game_id uuid NOT NULL REFERENCES games(id) ON DELETE RESTRICT,
 platform_code text NOT NULL CHECK (platform_code ~ '^[a-z][a-z0-9_]{0,31}$'),
 external_key text NOT NULL CHECK (length(external_key) BETWEEN 1 AND 160),
 source_url text NOT NULL CHECK (length(source_url)<=2048 AND (source_url='' OR source_url ~ '^https?://')),
 active boolean NOT NULL DEFAULT true,
 UNIQUE (platform_code,external_key),
 CHECK (platform_code<>'steam' OR CASE WHEN external_key ~ '^[1-9][0-9]{0,9}$'
   THEN external_key::bigint<=4294967295 AND source_url='https://store.steampowered.com/app/'||external_key||'/'
   ELSE false END)
);
CREATE INDEX game_sources_game_id ON game_sources (game_id);

-- +goose StatementBegin
CREATE FUNCTION preserve_catalog_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.public_id IS DISTINCT FROM OLD.public_id THEN
  RAISE EXCEPTION 'catalog identity is immutable' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER sites_identity BEFORE UPDATE OF id, public_id ON sites FOR EACH ROW EXECUTE FUNCTION preserve_catalog_identity();
CREATE TRIGGER games_identity BEFORE UPDATE OF id, public_id ON games FOR EACH ROW EXECUTE FUNCTION preserve_catalog_identity();

-- +goose Down
DROP TABLE game_sources;
DROP TABLE game_localizations;
DROP TABLE games;
DROP TABLE site_addresses;
DROP TABLE site_localizations;
DROP TABLE sites;
DROP FUNCTION preserve_catalog_identity();
