-- name: SetSiteAssets :one
UPDATE sites SET logo_asset_key=$2,featured_cover_asset_key=$3,
 revision=revision+1,updated_at=transaction_timestamp()
WHERE id=$1 RETURNING revision;
