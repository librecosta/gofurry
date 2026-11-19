-- +goose Up
ALTER TABLE games ADD COLUMN content_rating text NOT NULL DEFAULT 'unreviewed'
 CHECK (content_rating IN ('unreviewed','sfw','mixed','adult'));
ALTER TABLE catalog_publication_decisions ADD COLUMN management_request_id uuid,
 ADD COLUMN content_rating text CHECK (content_rating IN ('unreviewed','sfw','mixed','adult'));

ALTER TABLE catalog_publication_decisions DROP CONSTRAINT catalog_publication_decisions_action_check,
 ADD CHECK (action IN ('publish','unpublish','archive','restore','block','clear_block','rating'));

CREATE TABLE game_published_localizations (
 game_id uuid NOT NULL,
 locale text NOT NULL CHECK (locale IN ('en','zh')),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
 summary text NOT NULL CHECK (length(summary)<=2000),
 text_source_id uuid,
 text_observation_id uuid,
 media_source_id uuid,
 media jsonb NOT NULL DEFAULT '{}',
 media_observed_at timestamptz,
 media_store_country text NOT NULL DEFAULT '',
 cover_url text NOT NULL DEFAULT '',
 revision bigint NOT NULL CHECK (revision>0),
 published_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 PRIMARY KEY (game_id,locale)
);

-- Retain superseded source text and its attribution for internal review.
CREATE TABLE game_source_text_history (
 game_id uuid NOT NULL,
 locale text NOT NULL,
 name text NOT NULL,
 summary text NOT NULL,
 game_source_id uuid NOT NULL,
 observation_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 store_country text,
 egress_region text,
 PRIMARY KEY (game_id,locale)
);
INSERT INTO game_source_text_history
SELECT game_id,locale,name,summary,game_source_id,observation_id,observed_at,steam_store_country,steam_egress_region
FROM game_approved_presentations;

-- The old publish operation recorded neither a confirmed Game rating nor the
-- exact reviewed version. Even receipt-backed text may have changed since
-- publication. Keep the old decisions and require fresh review of the text.
WITH withdrawn AS (
 UPDATE games SET status='draft',revision=revision+1,updated_at=transaction_timestamp()
 WHERE status='published'
 RETURNING id,revision,publication_blocked
)
INSERT INTO catalog_publication_decisions (id,entity_kind,entity_id,revision,action,actor,reason)
SELECT uuidv7(),'game',id,revision,'unpublish','schema-migration',
 CASE WHEN publication_blocked THEN 'Legacy restricted publication withdrawn during Game review cutover'
 ELSE 'Legacy publication has no verifiable reviewed Game snapshot and rating' END
FROM withdrawn;

ALTER TABLE games ADD CONSTRAINT games_published_sfw CHECK (status<>'published' OR content_rating='sfw');

CREATE OR REPLACE VIEW game_channel_public AS
SELECT g.id,g.public_id,g.slug,g.first_published_at,g.intake_display_source
FROM games g WHERE g.status='published' AND g.content_rating='sfw'
 AND EXISTS (SELECT 1 FROM game_published_localizations p WHERE p.game_id=g.id);

CREATE OR REPLACE VIEW game_channel_locales AS
SELECT game_id,locale,name,summary FROM game_published_localizations;

-- +goose StatementBegin
CREATE FUNCTION game_published_text(work uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT coalesce(jsonb_agg(jsonb_build_object('locale',locale,'name',name,'summary',summary) ORDER BY locale),'[]')
 FROM game_published_localizations WHERE game_id=work
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION game_published_cover(work uuid, language text) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT cover_url FROM game_published_localizations
 WHERE game_id=work AND cover_url<>''
 ORDER BY (locale=CASE language WHEN 'zh-cn' THEN 'zh' ELSE language END) DESC,locale LIMIT 1
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION game_draft_text(work uuid)
RETURNS TABLE (locale text,name text,summary text,source_id uuid,observation_id uuid)
LANGUAGE sql STABLE AS $$
 SELECT DISTINCT ON (r.locale) r.locale,r.name,r.summary,r.source_id,r.observation_id
 FROM (
  SELECT l.locale,l.name,l.summary,NULL::uuid AS source_id,NULL::uuid AS observation_id,0 AS priority,NULL::timestamptz AS observed_at
  FROM game_localizations l WHERE l.game_id=work
  UNION ALL
  SELECT i.profile->>'locale',left(btrim(i.profile->>'name'),160),left(coalesce(i.profile->>'summary',''),2000),
   i.game_source_id,i.profile_observation_id,1,i.profile_at
  FROM game_intake_candidates i JOIN game_sources s ON s.id=i.game_source_id AND s.game_id=i.game_id AND s.active
  WHERE i.game_id=work AND i.state='accepted' AND i.profile_observation_id IS NOT NULL
  UNION ALL
  SELECT CASE m.locale WHEN 'zh-cn' THEN 'zh' ELSE 'en' END,left(btrim(m.payload->>'name'),160),
   left(coalesce(m.payload->>'description',''),2000),s.id,m.observation_id,1,m.observed_at
  FROM game_sources s JOIN game_source_metadata m ON m.game_source_id=s.id
  WHERE s.game_id=work AND s.active AND m.section='details' AND m.payload->>'type'='game'
   AND (m.locale='en' OR (m.locale='zh-cn' AND m.payload->>'localized'='true'))
  UNION ALL
  SELECT h.locale,h.name,h.summary,s.id,h.observation_id,2,h.observed_at
  FROM game_source_text_history h JOIN game_sources s ON s.id=h.game_source_id AND s.game_id=h.game_id AND s.active
  WHERE h.game_id=work
 ) r JOIN games g ON g.id=work
 WHERE r.locale IN ('en','zh') AND r.name<>''
 ORDER BY r.locale,r.priority,(r.source_id=g.intake_display_source) DESC NULLS LAST,r.observed_at DESC NULLS LAST,r.source_id,r.observation_id
$$;
-- +goose StatementEnd

UPDATE game_intake_candidates SET state='discovered',reason='',next_at=transaction_timestamp(),failures=0,
 lease_id=NULL,lease_until=NULL,updated_at=transaction_timestamp()
WHERE game_id IS NULL AND reason IN ('eligibility_uncertain','irrelevant_game','adult_game');
UPDATE game_intake_candidates SET state='deferred',reason='related_source_unavailable',next_at=NULL,
 lease_id=NULL,lease_until=NULL WHERE game_id IS NULL AND relation<>'{}' AND state IN ('discovered','resolving','deferred');

DROP FUNCTION game_presentation(uuid,uuid);
DROP FUNCTION game_intake_presentation(uuid,boolean);
DROP FUNCTION game_channel_cover(uuid,uuid,text,text,text);
DROP TABLE game_approved_presentations;
DROP TABLE steam_eligibility_ai_budget;
DROP INDEX game_intake_relation,game_intake_abandoned;
CREATE INDEX game_intake_abandoned ON game_intake_candidates (updated_at)
 WHERE state='deferred' AND next_at IS NULL AND job_started_at IS NULL AND target_id IS NULL AND relation='{}';
ALTER TABLE game_intake_candidates DROP COLUMN review, DROP COLUMN review_until, DROP COLUMN review_revoked,
 DROP COLUMN automation, DROP COLUMN published_at, DROP COLUMN decision_version, DROP COLUMN relation_attached;
ALTER TABLE games DROP COLUMN intake_managed, DROP COLUMN publication_blocked;

CREATE INDEX games_channel_added ON games (first_published_at DESC NULLS LAST,public_id)
 WHERE status='published' AND content_rating='sfw';
