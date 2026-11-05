//go:build integration

package database_test

import (
	"context"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"
)

func TestFormatUpgradePreservesLegacy(t *testing.T) {
	if *emptyConfig == "" {
		t.Skip("explicit empty test database required")
	}
	cfg, e := platform.LoadConfig(*emptyConfig)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Database == nil || !strings.HasPrefix(cfg.Database.Name, "gofurry_c2_") || !strings.HasSuffix(cfg.Database.Name, "_empty_test") || cfg.Database.User != "gofurry_c2_test" {
		t.Fatal("unsafe migration test database")
	}
	ctx := context.Background()
	p, e := database.Open(ctx, *cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	var owner string
	var count int
	p.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()").Scan(&owner)
	p.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&count)
	if owner != cfg.Database.User || count != 0 {
		t.Fatal("test requires owned empty schema")
	}
	dir := t.TempDir()
	name := "202611050001_wallpapers.sql"
	b, e := os.ReadFile("../../migrations/" + name)
	if e != nil {
		t.Fatal(e)
	}
	if os.WriteFile(filepath.Join(dir, name), b, 0600) != nil {
		t.Fatal("cannot prepare C2 migration")
	}
	if e = database.Migrate(ctx, *cfg.Database, dir, "up", io.Discard); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := p.Exec(context.Background(), "DROP TABLE wallpaper_audit_logs,start_wallpapers,goose_db_version"); e != nil {
			t.Error("isolated empty test cleanup failed")
		}
	}()
	id := uuid.NewV7()
	hash := strings.Repeat("a", 64)
	const insert = `INSERT INTO start_wallpapers(id,title,status,image_key,image_sha256,image_type,image_bytes,width,height,preview_key,preview_sha256,preview_bytes,preview_width,preview_height) VALUES($1,'Legacy test draft','draft','start/backgrounds/'||$2||'.webp',$2,'image/webp',370,128,72,'start/backgrounds/'||$2||'.webp',$2,204,64,36)`
	if _, e = p.Exec(ctx, insert, id, hash); e != nil {
		t.Fatal(e)
	}
	if e = database.Migrate(ctx, *cfg.Database, "../../migrations", "up", io.Discard); e != nil {
		t.Fatal(e)
	}
	var typ, status string
	var revision int64
	if e = p.QueryRow(ctx, "SELECT image_type,status,revision FROM start_wallpapers WHERE id=$1", id).Scan(&typ, &status, &revision); e != nil || typ != "image/webp" || status != "draft" || revision != 1 {
		t.Fatal("migration changed legacy record")
	}
	var validated bool
	p.QueryRow(ctx, "SELECT convalidated FROM pg_constraint WHERE conname='official_wallpaper_avif'").Scan(&validated)
	if validated {
		t.Fatal("invalid legacy constraint falsely marked validated")
	}
	if _, e = p.Exec(ctx, insert, uuid.NewV7(), strings.Repeat("b", 64)); e == nil {
		t.Fatal("NOT VALID allowed new WebP business row")
	}
	s := wallpaper.New(p, nil, true)
	if _, e = s.Transition(ctx, wallpaper.Actor{}, id, 1, "published", true); e != wallpaper.ErrInvalid {
		t.Fatal("legacy WebP can publish")
	}
	if detail, e := s.Detail(ctx, id); e != nil || detail.Image.ContentType != "image/webp" {
		t.Fatal("legacy draft no longer readable")
	}
}
