-- +goose Up
ALTER TABLE games ADD COLUMN first_published_at timestamptz;
UPDATE games g SET first_published_at=coalesce(
 (SELECT min(d.decided_at) FROM catalog_publication_decisions d
  WHERE d.entity_kind='game' AND d.entity_id=g.id AND d.action='publish'), g.published_at)
WHERE g.published_at IS NOT NULL;
CREATE INDEX games_channel_added ON games (first_published_at DESC NULLS LAST,public_id)
 WHERE status='published' AND NOT publication_blocked;

CREATE TABLE game_collections (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id)=7),
 public_id text COLLATE "C" NOT NULL UNIQUE CHECK (public_id ~ '^[0123456789abcdefghjkmnpqrstvwxyz]{16}$'),
 slug text NOT NULL CHECK (length(slug) BETWEEN 1 AND 80 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 localizations jsonb NOT NULL CHECK (jsonb_typeof(localizations)='array' AND jsonb_array_length(localizations)<=2 AND octet_length(localizations::text)<=19000),
 status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 first_published_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 CHECK (status<>'published' OR (first_published_at IS NOT NULL AND localizations<>'[]'))
);
CREATE INDEX game_collections_public ON game_collections (first_published_at DESC,public_id) WHERE status='published';

CREATE TABLE game_collection_members (
 collection_id uuid NOT NULL CHECK (uuid_extract_version(collection_id)=7),
 game_id uuid NOT NULL CHECK (uuid_extract_version(game_id)=7),
 position integer NOT NULL CHECK (position BETWEEN 1 AND 512),
 PRIMARY KEY (collection_id,game_id),
 UNIQUE (collection_id,position)
);

CREATE TABLE game_channel_features (
 slot text PRIMARY KEY CHECK (slot IN ('primary','secondary_1','secondary_2','collection_1','collection_2','collection_3')),
 entity_id uuid CHECK (uuid_extract_version(entity_id)=7),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0)
);
CREATE UNIQUE INDEX game_channel_features_game ON game_channel_features (entity_id) WHERE slot IN ('primary','secondary_1','secondary_2');
CREATE UNIQUE INDEX game_channel_features_collection ON game_channel_features (entity_id) WHERE slot IN ('collection_1','collection_2','collection_3');
INSERT INTO game_channel_features (slot) VALUES ('primary'),('secondary_1'),('secondary_2'),('collection_1'),('collection_2'),('collection_3');

-- Compact derivative of a scoped Steam observation; game_source_metadata owns the evidence.
CREATE TABLE game_channel_steam (
 game_source_id uuid NOT NULL,
 locale text NOT NULL,
 store_country text NOT NULL,
 egress_region text NOT NULL,
 section text NOT NULL,
 game_id uuid NOT NULL,
 genres text[] NOT NULL DEFAULT '{}',
 systems text[] NOT NULL DEFAULT '{}',
 developers text[] NOT NULL DEFAULT '{}',
 coming_soon boolean,
 release_text text NOT NULL DEFAULT '',
 release_day date,
 precision text NOT NULL DEFAULT 'unknown',
 cover text,
 observation_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 PRIMARY KEY (game_source_id,locale,store_country,egress_region,section)
);
CREATE INDEX game_channel_steam_game ON game_channel_steam (game_id,section,game_source_id);

-- +goose StatementBegin
CREATE FUNCTION game_channel_day(raw text) RETURNS date LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE parts text[]; y integer; m integer; d integer;
BEGIN
 parts := regexp_match(raw,'^([0-9]{4})[-/]([0-9]{1,2})[-/]([0-9]{1,2})$');
 IF parts IS NULL THEN parts := regexp_match(raw,'^([0-9]{4})年([0-9]{1,2})月([0-9]{1,2})日$'); END IF;
 IF parts IS NOT NULL THEN y := parts[1]::int; m := parts[2]::int; d := parts[3]::int;
 ELSE
  parts := regexp_match(lower(raw),'^([0-9]{1,2}) ([a-z]+),? ([0-9]{4})$');
  IF parts IS NOT NULL THEN d := parts[1]::int; y := parts[3]::int;
  ELSE
   parts := regexp_match(lower(raw),'^([a-z]+) ([0-9]{1,2}),? ([0-9]{4})$');
   IF parts IS NULL THEN RETURN NULL; END IF;
   d := parts[2]::int; y := parts[3]::int; parts[2] := parts[1];
  END IF;
  SELECT idx::int INTO m FROM unnest(ARRAY['january','february','march','april','may','june',
   'july','august','september','october','november','december']) WITH ORDINALITY months(name,idx)
   WHERE parts[2] IN (name,left(name,3)) OR (idx=9 AND parts[2]='sept');
 END IF;
 IF m IS NULL OR y<1900 THEN RETURN NULL; END IF;
 RETURN make_date(y,m,d);
EXCEPTION WHEN datetime_field_overflow THEN RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION game_channel_derive(metadata game_source_metadata)
RETURNS SETOF game_channel_steam LANGUAGE sql STABLE AS $$
 SELECT s.id,m.locale,m.store_country,m.egress_region,m.section,s.game_id,
  ARRAY(SELECT DISTINCT code FROM jsonb_array_elements(coalesce(m.payload->'genres','[]')) item
        CROSS JOIN LATERAL (SELECT CASE item->>'id'
         WHEN '1' THEN 'action' WHEN '2' THEN 'strategy' WHEN '3' THEN 'rpg' WHEN '4' THEN 'casual'
         WHEN '9' THEN 'racing' WHEN '18' THEN 'sports' WHEN '23' THEN 'indie'
         WHEN '25' THEN 'adventure' WHEN '28' THEN 'simulation' WHEN '37' THEN 'free_to_play' END AS code) x
        WHERE code IS NOT NULL ORDER BY code),
  ARRAY(SELECT os FROM unnest(ARRAY['windows','mac','linux']) os WHERE m.payload->'platforms'->os='true'::jsonb),
  ARRAY(SELECT jsonb_array_elements_text(coalesce(m.payload->'developers','[]'))),
  CASE m.payload#>'{release,coming_soon}' WHEN 'true'::jsonb THEN true WHEN 'false'::jsonb THEN false END,
  date_text, release_day,
  CASE WHEN release_day IS NOT NULL THEN 'day'
   WHEN date_text ~* '^(Q[1-4] [0-9]{4}|[0-9]{4} Q[1-4]|[0-9]{4}年第?[一二三四1-4]季度)$' THEN 'quarter'
   WHEN date_text ~* '^([0-9]{4}[-/年](0?[1-9]|1[012])月?|(jan(uary)?|feb(ruary)?|mar(ch)?|apr(il)?|may|jun(e)?|jul(y)?|aug(ust)?|sep(t(ember)?)?|oct(ober)?|nov(ember)?|dec(ember)?) [0-9]{4})$' THEN 'month'
   WHEN date_text ~ '^[0-9]{4}年?$' THEN 'year'
   WHEN date_text ~* '^(to be announced|tba|coming soon|待公布|即将推出)$' THEN 'tba' ELSE 'unknown' END,
  coalesce(nullif(m.payload->>'header',''),nullif(m.payload->>'capsule','')),m.observation_id,m.observed_at
 FROM (SELECT (metadata).*) m JOIN game_sources s ON s.id=m.game_source_id AND s.platform_code='steam'
 CROSS JOIN LATERAL (SELECT btrim(coalesce(m.payload#>>'{release,date_text}','')) AS date_text) r
 CROSS JOIN LATERAL (SELECT game_channel_day(r.date_text) AS release_day) d
 WHERE m.payload IS NOT NULL
$$;
-- +goose StatementEnd

INSERT INTO game_channel_steam SELECT derived.* FROM game_source_metadata m CROSS JOIN LATERAL game_channel_derive(m) derived;

-- +goose StatementBegin
CREATE FUNCTION refresh_game_channel_steam() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.payload IS NULL OR (TG_OP='UPDATE' AND NEW.payload IS NOT DISTINCT FROM OLD.payload) THEN RETURN NEW; END IF;
 INSERT INTO game_channel_steam AS prev SELECT * FROM game_channel_derive(NEW)
 ON CONFLICT (game_source_id,locale,store_country,egress_region,section) DO UPDATE SET
  genres=excluded.genres,systems=excluded.systems,developers=excluded.developers,
  coming_soon=excluded.coming_soon,release_text=excluded.release_text,release_day=excluded.release_day,
  precision=excluded.precision,cover=coalesce(excluded.cover,prev.cover),
  observation_id=excluded.observation_id,observed_at=excluded.observed_at;
 RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER game_channel_refresh AFTER INSERT OR UPDATE ON game_source_metadata
 FOR EACH ROW EXECUTE FUNCTION refresh_game_channel_steam();

CREATE INDEX collection_current_channel_price ON collection_current ((scope->>'region'),confirmed_at DESC)
 WHERE capability='steam.details' AND output_schema=1 AND state='present' AND scope#>>'{dimensions,section}'='price';
CREATE INDEX game_evidence_news_channel ON game_evidence_news (published_at DESC,gid DESC);



CREATE VIEW game_channel_public AS
SELECT g.id,g.public_id,g.slug,g.first_published_at,g.intake_display_source
FROM games g WHERE g.status='published' AND NOT g.publication_blocked
AND (EXISTS (SELECT 1 FROM game_localizations l WHERE l.game_id=g.id)
 OR EXISTS (SELECT 1 FROM game_approved_presentations a WHERE a.game_id=g.id));

-- +goose StatementBegin
CREATE FUNCTION game_channel_facts(work uuid, preferred uuid, language text, country text, egress text)
RETURNS TABLE (genres text[], systems text[], developers text[], release_state text, release_day date,
 release jsonb) LANGUAGE sql STABLE AS $$
 SELECT coalesce(d.genres,'{}'),coalesce(d.systems,'{}'),coalesce(d.developers,'{}'),
  CASE d.coming_soon WHEN true THEN 'upcoming' WHEN false THEN 'released' ELSE 'unknown' END,
  d.release_day,
  CASE WHEN d.game_source_id IS NOT NULL THEN jsonb_build_object(
   'platform','steam','app_id',d.external_key,'text',d.release_text,'precision',d.precision,'day',d.release_day,
   'store_country',d.store_country,'locale',d.locale,'observed_at',d.observed_at,'observation_id',d.observation_id) END
 FROM (VALUES (1)) seed(x)
 LEFT JOIN LATERAL (
  SELECT m.*,s.external_key FROM game_channel_steam m JOIN game_sources s ON s.id=m.game_source_id AND s.game_id=work
  WHERE m.game_id=work AND m.section='details'
  ORDER BY (m.game_source_id=preferred) DESC NULLS LAST,s.active DESC,
   (m.egress_region=egress AND m.store_country=country) DESC,(m.locale=language) DESC,m.observed_at DESC,m.game_source_id,m.locale,m.store_country,m.egress_region LIMIT 1
 ) d ON true
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION game_channel_quotes(country text, currency text, egress text)
RETURNS TABLE (game_id uuid, game_source_id uuid, locale text, confirmed_at timestamptz, price jsonb)
LANGUAGE sql STABLE AS $$
 SELECT b.game_id,s.id,c.scope->>'locale',c.confirmed_at,jsonb_build_object(
 'platform','steam','app_id',s.external_key,'locale',c.scope->>'locale','store_country',country,
 'egress_region',egress,'state',c.data->>'state','amounts',c.data->'amounts','confirmed_at',c.confirmed_at,
 'observed_at',c.observed_at,'observation_id',c.observation_id)
 FROM collection_current c
 JOIN collection_consumer cn ON cn.generation=c.generation
 JOIN collection_sources src ON src.id=c.source_id AND src.namespace=cn.namespace AND src.provider='steam' AND src.resource_type='app'
 JOIN collection_bindings b ON b.source_id=src.id AND b.effective_until IS NULL AND b.purpose='steam'
 JOIN game_sources s ON s.id=b.game_source_id AND s.game_id=b.game_id AND s.active AND s.platform_code='steam' AND s.external_key=src.external_key
 WHERE c.capability='steam.details' AND c.output_schema=1 AND c.state='present' AND c.observation_id IS NOT NULL
 AND c.scope#>>'{dimensions,section}'='price' AND c.scope->>'region'=country AND c.scope->>'egress_region'=egress
 AND c.scope->>'locale' IN ('en','zh-cn') AND c.scope->>'namespace'=src.namespace AND c.data->>'app_id'=s.external_key
 AND c.data->>'tier'='observed_storefront' AND c.data->>'source_url'='https://store.steampowered.com/api/appdetails'
 AND (c.data->>'state'='free' OR (c.data->>'state' IN ('priced','zero_price') AND c.data#>>'{amounts,currency}'=currency))
 AND c.confirmed_at BETWEEN greatest(b.effective_from,statement_timestamp()-interval '48 hours') AND statement_timestamp()
 AND c.observed_at<=c.confirmed_at
 AND NOT EXISTS (SELECT 1 FROM collection_gaps gap WHERE gap.namespace=src.namespace AND gap.prior_generation=cn.generation)
 AND EXISTS (SELECT 1 FROM collection_plan_bindings m JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id
  WHERE m.binding_id=b.id AND m.source_id=src.id AND p.capability=c.capability AND p.request_schema=c.output_schema
   AND p.scope=c.scope-'namespace'-'dimensions' AND m.attached_at<=c.confirmed_at)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION game_channel_card(work uuid, pid text, slug text, first_at timestamptz, languages jsonb,
 genres text[], systems text[], phase text, release jsonb, cover text, price jsonb) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('public_id',pid,'slug',slug,'first_published_at',first_at,'localizations',languages,'genres',genres,
 'systems',systems,'release_state',phase,'release',release,'cover_url',cover,'price_summary',price,
 'platforms',coalesce((SELECT jsonb_agg(to_jsonb(s) ORDER BY s.platform_code,s.external_key) FROM (
  SELECT platform_code,external_key,nullif(source_url,'') AS source_url FROM game_sources
  WHERE game_id=work AND active ORDER BY platform_code,external_key LIMIT 16) s),'[]'))
$$;
-- +goose StatementEnd


CREATE VIEW game_channel_locales AS
 SELECT a.game_id,a.locale,a.name,a.summary FROM game_approved_presentations a
 WHERE NOT EXISTS (SELECT 1 FROM game_localizations l WHERE l.game_id=a.game_id AND l.locale=a.locale)
 UNION ALL SELECT game_id,locale,name,summary FROM game_localizations;

-- +goose StatementBegin
CREATE FUNCTION game_channel_cover(work uuid, preferred uuid, language text, country text, egress text)
RETURNS text LANGUAGE sql STABLE AS $$
 SELECT m.cover FROM game_channel_steam m
 JOIN game_sources s ON s.id=m.game_source_id AND s.game_id=work
 WHERE m.game_id=work AND m.section='media' AND m.cover IS NOT NULL
 ORDER BY (m.game_source_id=preferred) DESC NULLS LAST,s.active DESC,
  (m.egress_region=egress AND m.store_country=country) DESC,(m.locale=language) DESC,
  m.observed_at DESC,m.game_source_id,m.locale,m.store_country,m.egress_region LIMIT 1
$$;
-- +goose StatementEnd
