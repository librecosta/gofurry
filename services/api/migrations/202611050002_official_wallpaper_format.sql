-- +goose Up
-- NOT VALID preserves C2 legacy records without silently deleting or rewriting
-- assets. PostgreSQL nevertheless enforces these constraints on EVERY new write.
-- An old incompatible Draft cannot be published; replace via a new AVIF upload,
-- then explicitly delete the old Draft with its revision. Never rewrite history.
ALTER TABLE start_wallpapers ADD CONSTRAINT official_wallpaper_avif CHECK (image_type='image/avif') NOT VALID;
ALTER TABLE start_wallpapers ADD CONSTRAINT official_wallpaper_ratio CHECK (width::bigint*9=height::bigint*16 AND preview_width::bigint*9=preview_height::bigint*16 AND preview_width<=480 AND preview_height<=270) NOT VALID;
-- Validate immediately for deployments with no legacy incompatible records.
-- +goose StatementBegin
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM start_wallpapers WHERE image_type<>'image/avif') THEN
  ALTER TABLE start_wallpapers VALIDATE CONSTRAINT official_wallpaper_avif;
 END IF;
 IF NOT EXISTS (SELECT 1 FROM start_wallpapers WHERE width::bigint*9<>height::bigint*16 OR preview_width::bigint*9<>preview_height::bigint*16 OR preview_width>480 OR preview_height>270) THEN
  ALTER TABLE start_wallpapers VALIDATE CONSTRAINT official_wallpaper_ratio;
 END IF;
END $$;
-- +goose StatementEnd
-- +goose Down
ALTER TABLE start_wallpapers DROP CONSTRAINT official_wallpaper_ratio;
ALTER TABLE start_wallpapers DROP CONSTRAINT official_wallpaper_avif;
