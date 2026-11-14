-- +goose Up
CREATE TABLE steam_discovery_state (
 namespace text PRIMARY KEY CHECK (namespace ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
 paused boolean NOT NULL DEFAULT false,
 backfill boolean NOT NULL DEFAULT false,
 watermark bigint NOT NULL DEFAULT 0,
 window_started bigint NOT NULL DEFAULT 0,
 modified_since bigint NOT NULL DEFAULT 0,
 last_appid bigint NOT NULL DEFAULT 0,
 page_limit integer NOT NULL DEFAULT 0,
 egress_region text NOT NULL DEFAULT '',
 target_id uuid,
 intent_id uuid,
 intent_at timestamptz,
 writer_binding text NOT NULL DEFAULT '',
 job_id uuid,
 next_at timestamptz NOT NULL DEFAULT '-infinity',
 budget_day bigint NOT NULL DEFAULT 0,
 jobs_today integer NOT NULL DEFAULT 0 CHECK (jobs_today>=0),
 selected_today integer NOT NULL DEFAULT 0 CHECK (selected_today>=0),
 explored_today integer NOT NULL DEFAULT 0 CHECK (explored_today>=0),
 admission_day bigint NOT NULL DEFAULT 0,
 admitted_today integer NOT NULL DEFAULT 0,
 pages bigint NOT NULL DEFAULT 0,
 rows_seen bigint NOT NULL DEFAULT 0,
 windows bigint NOT NULL DEFAULT 0,
 admitted bigint NOT NULL DEFAULT 0,
 pending jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(pending)='array' AND jsonb_array_length(pending)<=512 AND octet_length(pending::text)<=65536),
 last_code text NOT NULL DEFAULT '',
 CHECK (watermark BETWEEN 0 AND 4294967295 AND modified_since BETWEEN 0 AND 4294967295 AND last_appid BETWEEN 0 AND 4294967295)
);

ALTER TABLE game_intake_candidates ADD COLUMN discovery_modified bigint NOT NULL DEFAULT 0,
 ADD COLUMN job_generation bigint NOT NULL DEFAULT 0 CHECK (job_generation>=0),
 ADD COLUMN automation jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(automation)='object' AND octet_length(automation::text)<=768);

CREATE TABLE steam_eligibility_ai_budget (
 utc_day date PRIMARY KEY,
 calls integer NOT NULL CHECK (calls>0)
);

ALTER TABLE game_source_metadata ADD CHECK (section<>'details' OR NOT (payload ? 'content_rating') OR jsonb_typeof(payload->'content_rating')='object');
-- Accept the vetted additive Storefront field in the existing typed payload.
-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
 SELECT conname INTO STRICT c FROM pg_constraint
 WHERE conrelid='game_source_metadata'::regclass AND pg_get_constraintdef(oid) LIKE '%supported_languages%';
 EXECUTE format('ALTER TABLE game_source_metadata DROP CONSTRAINT %I',c);
END
$$;
-- +goose StatementEnd
ALTER TABLE game_source_metadata ADD CHECK (payload IS NULL OR (jsonb_typeof(payload)='object' AND octet_length(payload::text)<=32768 AND
 CASE section WHEN 'details' THEN
 jsonb_typeof(payload->'name')='string' AND length(payload->>'name')>0 AND
 payload-ARRAY['name','type','description','supported_languages','developers','publishers','platforms','categories','genres','release','localized','content_rating']='{}'::jsonb
 ELSE payload-ARRAY['header','capsule','screenshots','movies']='{}'::jsonb AND payload<>'{}'::jsonb END));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION game_intake_presentation(work uuid, historical boolean)
RETURNS TABLE (game_id uuid,locale text,name text,summary text,game_source_id uuid,observation_id uuid,observed_at timestamptz,approval jsonb)
LANGUAGE sql STABLE AS $$
 SELECT DISTINCT ON (i.profile->>'locale') g.id,i.profile->>'locale',i.profile->>'name',i.profile->>'summary',
  i.game_source_id,i.profile_observation_id,i.profile_at,
  CASE WHEN i.review<>'{}' THEN i.review ELSE i.automation END
 FROM games g JOIN game_intake_candidates i ON i.game_id=g.id
 JOIN game_sources s ON s.id=i.game_source_id AND s.game_id=g.id AND s.platform_code='steam' AND s.external_key=i.external_key
 WHERE g.id=work AND i.platform_code='steam' AND i.decision_version=1 AND i.reason<>'merge_suggested'
  AND ((i.review->>'policy'='furry-and-sfw-1' AND i.review->>'furry_ref'<>'' AND i.review->>'sfw_ref'<>''
   AND i.review->>'furry_ref'<>i.review->>'sfw_ref')
   OR (i.review='{}' AND i.automation->>'policy'='steam-eligibility-1' AND i.automation->>'furry_relevance'='relevant'
    AND i.automation->>'adult_content'='sfw' AND i.automation->>'app_type'='game'
    AND i.automation->>'observation_id'=i.profile_observation_id::text))
  AND i.profile_observation_id IS NOT NULL AND i.profile_at IS NOT NULL
  AND i.profile->>'locale' IN ('en','zh') AND length(btrim(i.profile->>'name')) BETWEEN 1 AND 160
  AND length(btrim(i.profile->>'summary')) BETWEEN 1 AND 2000
  AND (historical OR (s.active AND i.state='accepted' AND NOT i.review_revoked))
 ORDER BY i.profile->>'locale',i.review_revoked,(i.game_source_id=g.intake_display_source) DESC NULLS LAST,i.created_at,i.id
$$;
-- +goose StatementEnd
