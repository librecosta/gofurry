-- +goose Up
DROP INDEX collection_current_channel_price;
CREATE INDEX collection_current_channel_price ON collection_current ((scope->>'region'),confirmed_at DESC)
 WHERE capability='steam.details' AND output_schema IN (1,2) AND state='present' AND scope#>>'{dimensions,section}'='price';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION game_channel_quotes(country text, currency text, egress text)
RETURNS TABLE (game_id uuid, game_source_id uuid, locale text, confirmed_at timestamptz, price jsonb)
LANGUAGE sql STABLE AS $$
 SELECT DISTINCT ON (b.game_id,s.id,c.scope->>'locale') b.game_id,s.id,c.scope->>'locale',c.confirmed_at,jsonb_build_object(
 'platform','steam','app_id',s.external_key,'locale',c.scope->>'locale','store_country',country,
 'egress_region',egress,'state',c.data->>'state','amounts',c.data->'amounts','confirmed_at',c.confirmed_at,
 'observed_at',c.observed_at,'observation_id',c.observation_id)
 FROM collection_current c
 JOIN collection_consumer cn ON cn.generation=c.generation
 JOIN collection_sources src ON src.id=c.source_id AND src.namespace=cn.namespace AND src.provider='steam' AND src.resource_type='app'
 JOIN collection_bindings b ON b.source_id=src.id AND b.effective_until IS NULL AND b.purpose='steam'
 JOIN game_sources s ON s.id=b.game_source_id AND s.game_id=b.game_id AND s.active AND s.platform_code='steam' AND s.external_key=src.external_key
 WHERE c.capability='steam.details' AND c.output_schema IN (1,2) AND c.state='present' AND c.observation_id IS NOT NULL
 AND c.scope#>>'{dimensions,section}'='price' AND c.scope->>'region'=country AND c.scope->>'egress_region'=egress
 AND c.scope->>'locale' IN ('en','zh-cn') AND c.scope->>'namespace'=src.namespace AND c.data->>'app_id'=s.external_key
 AND c.data->>'tier'='observed_storefront' AND c.data->>'source_url'='https://store.steampowered.com/api/appdetails'
 AND (c.data->>'state'='free' OR (c.data->>'state' IN ('priced','zero_price') AND c.data#>>'{amounts,currency}'=currency))
 AND c.confirmed_at BETWEEN greatest(b.effective_from,statement_timestamp()-interval '48 hours') AND statement_timestamp()
 AND c.observed_at<=c.confirmed_at
 AND NOT EXISTS (SELECT 1 FROM collection_gaps gap WHERE gap.namespace=src.namespace AND gap.prior_generation=cn.generation)
 AND EXISTS (SELECT 1 FROM collection_plan_bindings m JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=m.source_id
  WHERE m.binding_id=b.id AND m.source_id=src.id AND p.capability=c.capability AND p.output_schema=c.output_schema
   AND p.scope=c.scope-'namespace'-'dimensions' AND m.attached_at<=c.confirmed_at)
 ORDER BY b.game_id,s.id,c.scope->>'locale',c.confirmed_at DESC,c.output_schema DESC,c.observation_id
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION game_insight_owners(src_id uuid, cap text, evidence_at timestamptz)
RETURNS TABLE (game_source_id uuid, binding_id uuid, scope jsonb, namespace text, app_id text)
LANGUAGE sql STABLE AS $$
 SELECT s.id,b.id,p.scope,src.namespace,src.external_key
 FROM collection_sources src
 JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam'
 JOIN game_sources s ON s.id=b.game_source_id AND s.game_id=b.game_id AND s.platform_code='steam' AND s.external_key=src.external_key
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=cap AND p.output_schema=1
 WHERE src.id=src_id AND src.target_id IS NOT NULL AND src.provider='steam' AND src.resource_type='app'
   AND evidence_at>=greatest(b.effective_from,m.attached_at) AND (b.effective_until IS NULL OR evidence_at<b.effective_until)
$$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX collection_current_channel_price;
CREATE INDEX collection_current_channel_price ON collection_current ((scope->>'region'),confirmed_at DESC)
 WHERE capability='steam.details' AND output_schema=1 AND state='present' AND scope#>>'{dimensions,section}'='price';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION game_channel_quotes(country text, currency text, egress text)
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
CREATE OR REPLACE FUNCTION game_insight_owners(src_id uuid, cap text, evidence_at timestamptz)
RETURNS TABLE (game_source_id uuid, binding_id uuid, scope jsonb, namespace text, app_id text)
LANGUAGE sql STABLE AS $$
 SELECT s.id,b.id,p.scope,src.namespace,src.external_key
 FROM collection_sources src
 JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam'
 JOIN game_sources s ON s.id=b.game_source_id AND s.game_id=b.game_id AND s.platform_code='steam' AND s.external_key=src.external_key
 JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
 JOIN collection_plans p ON p.id=m.plan_id AND p.source_id=src.id AND p.capability=cap AND p.request_schema=1
 WHERE src.id=src_id AND src.target_id IS NOT NULL AND src.provider='steam' AND src.resource_type='app'
   AND evidence_at>=greatest(b.effective_from,m.attached_at) AND (b.effective_until IS NULL OR evidence_at<b.effective_until)
$$;
-- +goose StatementEnd
