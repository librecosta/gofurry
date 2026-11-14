-- name: SteamEligibilityGames :many
SELECT DISTINCT i.id AS candidate_id,i.game_id,i.review,i.review_revoked,c.observation_id,c.observed_at
FROM collection_current c
JOIN collection_sources src ON src.id=c.source_id AND src.provider='steam' AND src.resource_type='app'
JOIN collection_bindings b ON b.source_id=src.id AND b.purpose='steam' AND b.effective_until IS NULL
JOIN collection_plan_bindings m ON m.binding_id=b.id AND m.source_id=src.id
JOIN collection_plans p ON p.id=m.plan_id AND p.capability=c.capability AND p.scope=c.scope-'namespace'-'dimensions'
JOIN game_intake_candidates i ON i.game_source_id=b.game_source_id AND i.game_id=b.game_id AND i.external_key=src.external_key
 AND i.namespace=src.namespace AND i.platform_code='steam' AND i.state='accepted' AND i.reason<>'merge_suggested'
JOIN games g ON g.id=i.game_id AND g.intake_managed AND g.intake_display_source=i.game_source_id
JOIN game_sources gs ON gs.id=i.game_source_id AND gs.game_id=g.id AND gs.platform_code='steam' AND gs.external_key=src.external_key AND gs.active
WHERE c.generation=sqlc.arg(generation) AND c.capability='steam.details'
 AND (sqlc.narg(source_id)::uuid IS NULL OR c.source_id=sqlc.narg(source_id))
 AND (sqlc.narg(scope_key)::text IS NULL OR c.scope_key=sqlc.narg(scope_key))
 AND c.observation_id IS NOT NULL AND (c.data#>>'{content_rating,esrb}'='AO' OR c.data#>'{content_rating,descriptors}' @> '[3]'::jsonb)
 AND c.scope->>'namespace'=src.namespace AND c.scope->>'locale'='en' AND c.scope->>'region'='us'
 AND c.scope->>'egress_region'=i.egress_region AND c.scope#>>'{dimensions,section}'='details'
 AND c.output_schema=1 AND c.ruleset='gofurry-steam-1' AND c.state='present'
 AND c.observed_at>=greatest(b.effective_from,m.attached_at)
 AND c.observed_at>coalesce((SELECT CASE WHEN a.approval->>'policy'='steam-eligibility-1' THEN (a.approval->>'decided_at')::timestamptz ELSE a.observed_at END
   FROM game_approved_presentations a WHERE a.game_id=g.id AND a.locale='en'),'-infinity')
 AND NOT EXISTS (SELECT 1 FROM catalog_publication_decisions d WHERE d.entity_kind='game' AND d.entity_id=g.id
   AND d.action='clear_block' AND d.decided_at>=c.observed_at)
ORDER BY i.game_id,i.id;

-- name: SteamAdultDispositionCleared :one
SELECT EXISTS (SELECT 1 FROM catalog_publication_decisions WHERE entity_kind='game' AND entity_id=$1
 AND action='clear_block' AND decided_at>=$2)::boolean;
