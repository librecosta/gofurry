-- name: Wallpaper :one
SELECT * FROM start_wallpapers WHERE id=$1;
-- name: WallpaperByImage :one
SELECT * FROM start_wallpapers WHERE image_key=$1;
-- name: LockWallpaper :one
SELECT * FROM start_wallpapers WHERE id=$1 FOR UPDATE;
-- name: WallpaperList :many
SELECT * FROM start_wallpapers WHERE (sqlc.arg(status_filter)::text='' OR status=sqlc.arg(status_filter)) AND strpos(lower(title),sqlc.arg(keyword)::text)>0
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
-- name: WallpaperCount :one
SELECT count(*) FROM start_wallpapers WHERE (sqlc.arg(status_filter)::text='' OR status=sqlc.arg(status_filter)) AND strpos(lower(title),sqlc.arg(keyword)::text)>0;
-- name: PublishedWallpapers :many
SELECT * FROM start_wallpapers WHERE status='published' ORDER BY sort_order,published_at DESC,id LIMIT 1001;
-- name: CreateWallpaper :one
INSERT INTO start_wallpapers(id,title,status,image_key,image_sha256,image_type,image_bytes,width,height,preview_key,preview_sha256,preview_bytes,preview_width,preview_height,source_url,credit,sort_order,published_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,CASE WHEN $3::text='published' THEN transaction_timestamp() ELSE NULL END) RETURNING *;
-- name: EditWallpaper :one
UPDATE start_wallpapers SET title=$3,source_url=$4,credit=$5,sort_order=$6,revision=revision+1,updated_at=transaction_timestamp() WHERE id=$1 AND revision=$2 RETURNING *;
-- name: TransitionWallpaper :one
UPDATE start_wallpapers SET status=sqlc.arg(next_status),revision=revision+1,updated_at=transaction_timestamp(),published_at=CASE WHEN sqlc.arg(next_status)::text='published' THEN transaction_timestamp() ELSE published_at END
WHERE id=sqlc.arg(id) AND revision=sqlc.arg(revision) AND status=sqlc.arg(previous_status) RETURNING *;
-- name: DeleteDraft :one
DELETE FROM start_wallpapers WHERE id=$1 AND revision=$2 AND status='draft' RETURNING *;
-- name: AppendWallpaperAudit :exec
INSERT INTO wallpaper_audit_logs(id,actor_account_id,actor_name,actor_role,action,wallpaper_id,request_id,before_data,after_data) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);
-- name: CheckWallpaperSchema :exec
SELECT w.id,a.id FROM start_wallpapers w CROSS JOIN wallpaper_audit_logs a LIMIT 0;
