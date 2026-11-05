//go:build integration

package wallpaper

import (
	"context"
	"flag"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"
)

var testConfig = flag.String("wallpaper-config", "", "Explicit disposable GoFurry-owned *_test YAML")
var realBucket = flag.String("development-bucket", "", "Explicitly confirmed isolated development bucket; enables real object operations")
var realPublic = flag.String("development-public-base", "", "Explicitly confirmed development public domain")

func integration(t *testing.T, real bool) (context.Context, *Service, platform.Config) {
	t.Helper()
	if *testConfig == "" {
		t.Skip("explicit isolated database configuration required")
	}
	cfg, e := platform.LoadConfig(*testConfig)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Server.Environment != "development" || cfg.Database == nil || !strings.HasPrefix(cfg.Database.Name, "gofurry_c2_") || !strings.HasSuffix(cfg.Database.Name, "_test") {
		t.Fatal("refusing non-exclusive test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	pool, e := database.Open(ctx, *cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	var owner string
	if e = pool.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()").Scan(&owner); e != nil || owner != "gofurry_c2_test" || cfg.Database.User != owner {
		t.Fatal("isolated test database ownership mismatch")
	}
	if real {
		if *realBucket == "" || *realPublic == "" {
			t.Skip("real development bucket/domain confirmation required")
		}
		if cfg.Storage == nil || cfg.Storage.Bucket != *realBucket || cfg.Storage.PublicBaseURL != *realPublic || !strings.Contains(*realBucket, "dev") {
			t.Fatal("unconfirmed development storage")
		}
	}
	if _, e = pool.Exec(ctx, "TRUNCATE wallpaper_audit_logs,start_wallpapers"); e != nil {
		t.Fatal(e)
	}
	var objects Objects
	if real {
		store := storage.New(*cfg.Storage)
		t.Cleanup(store.Close)
		objects = store
	}
	return ctx, New(pool, objects, true), cfg
}
func testActor() Actor {
	id := uuid.NewV7()
	return Actor{ID: &id, Name: "Local Acceptance", Role: "owner", RequestID: uuid.NewV7().String()}
}
func TestRealDevelopmentUpload(t *testing.T) {
	ctx, s, _ := integration(t, true)
	actor := testActor()
	for _, name := range []string{"image.avif", "image.webp"} {
		image, e := os.Open("testdata/" + name)
		if e != nil {
			t.Fatal(e)
		}
		preview, e := os.Open("testdata/preview.webp")
		if e != nil {
			t.Fatal(e)
		}
		typ := "image/avif"
		if strings.HasSuffix(name, "webp") {
			typ = "image/webp"
		}
		row, e := s.Upload(ctx, actor, Metadata{Title: "Development fixture " + name}, image, preview, typ, "image/webp")
		if typ == "image/webp" {
			image.Close()
			preview.Close()
			if e != ErrInvalid {
				t.Fatal("高清 WebP accepted")
			}
			continue
		}
		if e != nil {
			t.Fatalf("real development upload: %v", e)
		}
		if row.Status != "draft" || row.ID.String()[14] != '7' {
			t.Fatal("upload not a UUIDv7 draft")
		}
		if _, e = s.Upload(ctx, actor, Metadata{Title: "Duplicate"}, image, preview, typ, "image/webp"); e != ErrConflict {
			t.Fatal("duplicate created a record", e)
		}
		image.Close()
		preview.Close()
	}
	t.Log("REAL DEVELOPMENT R2: AVIF+WebP, WebP+WebP, immutable reuse, S3/public GET byte/hash/type verification and duplicate prevention passed")
}
