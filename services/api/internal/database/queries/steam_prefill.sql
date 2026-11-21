-- name: ExistingSteamGame :one
SELECT g.public_id FROM game_sources s JOIN games g ON g.id=s.game_id
WHERE s.platform_code='steam' AND s.external_key=sqlc.arg(app_id);

-- name: ListSteamEditorialPrefills :many
SELECT * FROM steam_editorial_prefills WHERE namespace=$1 AND app_id=$2 ORDER BY locale;

-- name: LockSteamPrefillAdmission :exec
SELECT pg_advisory_xact_lock(hashtextextended('steam-editorial-prefill:' || sqlc.arg(namespace)::text,0));

-- name: CountRecentSteamPrefillStarts :one
SELECT count(*) FROM steam_editorial_prefills WHERE namespace=$1 AND started_at>=statement_timestamp()-interval '1 hour';

-- name: AdmitSteamPrefill :exec
INSERT INTO steam_editorial_prefills (namespace,app_id,locale,generation) VALUES ($1,$2,$3,$4)
ON CONFLICT (namespace,app_id,locale) DO NOTHING;

-- name: ReplaceSteamPrefill :one
UPDATE steam_editorial_prefills SET generation=sqlc.arg(new_generation),started_at=statement_timestamp(),
 job_id=NULL,target_id=NULL,submission_code=''
WHERE namespace=$1 AND app_id=$2 AND locale=$3 AND generation=sqlc.arg(expected_generation)
 AND started_at<=statement_timestamp()-interval '1 minute'
RETURNING *;

-- name: SaveSteamPrefillJob :exec
UPDATE steam_editorial_prefills SET job_id=$5,target_id=$6,submission_code=''
WHERE namespace=$1 AND app_id=$2 AND locale=$3 AND generation=$4 AND job_id IS NULL;

-- name: SaveSteamPrefillFacts :exec
UPDATE steam_editorial_prefills SET text_observation_id=$5,name=$6,summary=$7,facts=$8,media_observation_id=$9,media=$10
WHERE namespace=$1 AND app_id=$2 AND locale=$3 AND generation=$4
 AND (text_observation_id,media_observation_id,name,summary,facts,media) IS DISTINCT FROM ($5,$9,$6,$7,$8,$10);

-- name: SaveSteamPrefillSubmissionCode :exec
UPDATE steam_editorial_prefills SET submission_code=$5
WHERE namespace=$1 AND app_id=$2 AND locale=$3 AND generation=$4 AND job_id IS NULL;
