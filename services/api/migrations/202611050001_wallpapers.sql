-- +goose Up
CREATE TABLE start_wallpapers (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 160),
 status text NOT NULL CHECK (status IN ('draft','published','archived')),
 image_key text NOT NULL UNIQUE,
 image_sha256 text NOT NULL CHECK (image_sha256 ~ '^[a-f0-9]{64}$'),
 image_type text NOT NULL CHECK (image_type IN ('image/avif','image/webp')),
 image_bytes bigint NOT NULL CHECK (image_bytes BETWEEN 1 AND 52428800),
 width integer NOT NULL CHECK (width BETWEEN 1 AND 16384),
 height integer NOT NULL CHECK (height BETWEEN 1 AND 16384 AND width::bigint*height<=36000000),
 preview_key text NOT NULL,
 preview_sha256 text NOT NULL CHECK (preview_sha256 ~ '^[a-f0-9]{64}$'),
 preview_bytes bigint NOT NULL CHECK (preview_bytes BETWEEN 1 AND 5242880),
 preview_width integer NOT NULL CHECK (preview_width BETWEEN 1 AND 4096),
 preview_height integer NOT NULL CHECK (preview_height BETWEEN 1 AND 4096 AND preview_width::bigint*preview_height<=4000000),
 source_url text CHECK (source_url IS NULL OR length(source_url)<=2048),
 credit text CHECK (credit IS NULL OR length(credit)<=240),
 sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN -1000000 AND 1000000),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 published_at timestamptz,
 CHECK (image_key='start/backgrounds/'||image_sha256||CASE image_type WHEN 'image/avif' THEN '.avif' ELSE '.webp' END),
 CHECK (preview_key='start/backgrounds/'||preview_sha256||'.webp'),
 CHECK (status<>'published' OR published_at IS NOT NULL)
);
CREATE INDEX start_wallpapers_management ON start_wallpapers (created_at DESC,id DESC);
CREATE INDEX start_wallpapers_public ON start_wallpapers (sort_order,published_at DESC,id) WHERE status='published';
CREATE TABLE wallpaper_audit_logs (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 actor_account_id uuid,
 actor_name text NOT NULL CHECK (length(actor_name) BETWEEN 1 AND 80),
 actor_role text NOT NULL CHECK (actor_role IN ('owner','developer','operator','system')),
 action text NOT NULL CHECK (action IN ('wallpaper.created','wallpaper.edited','wallpaper.published','wallpaper.archived','wallpaper.restored','wallpaper.deleted','wallpaper.imported')),
 wallpaper_id uuid,
 request_id text NOT NULL CHECK (length(request_id) BETWEEN 1 AND 64),
 before_data jsonb NOT NULL CHECK (jsonb_typeof(before_data)='object' AND octet_length(before_data::text)<=16384),
 after_data jsonb NOT NULL CHECK (jsonb_typeof(after_data)='object' AND octet_length(after_data::text)<=16384),
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp()
);
CREATE INDEX wallpaper_audit_order ON wallpaper_audit_logs (created_at DESC,id DESC);
-- No foreign key to Admin, and no wallpaper FK: deletion retains the audit.
-- +goose Down
DROP TABLE wallpaper_audit_logs;
DROP TABLE start_wallpapers;
