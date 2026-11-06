//go:build integration

package wallpaper

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"strings"
	"testing"
	"time"
	"uuid"
)

// Read-only against the real catalog; synthetic pagination rows live only in a
// connection-local temporary table and are rolled back. No reset or object I/O.
func TestC4PublicCatalog(t *testing.T) {
	if *testConfig == "" {
		t.Skip("explicit development YAML required")
	}
	cfg, err := platform.LoadConfig(*testConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Environment != "development" || cfg.Database == nil || !strings.HasPrefix(cfg.Database.Name, "gofurry_") || !strings.HasSuffix(cfg.Database.Name, "_test") {
		t.Fatal("development database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, *cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	objects := storage.New(*cfg.Storage)
	defer objects.Close()
	service := New(pool, objects, true)
	page, err := service.PublicPage(ctx, 1, 9, "")
	if err != nil || len(page.Backgrounds) > 9 {
		t.Fatal(page, err)
	}
	t.Logf("real Published catalog: total=%d first_page=%d", page.Total, len(page.Backgrounds))
	if len(page.Backgrounds) == 0 {
		t.Fatal("real development Published resource required")
	}
	first := page.Backgrounds[0]
	if one, e := service.PublicOne(ctx, first.ID, false); e != nil || one.ID != first.ID {
		t.Fatal("detail", e)
	}
	if one, e := service.PublicOne(ctx, first.ID, true); e != nil || (page.Total > 1 && one.ID == first.ID) {
		t.Fatal("random exclusion", e)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "CREATE TEMP TABLE start_wallpapers (LIKE public.start_wallpapers INCLUDING DEFAULTS) ON COMMIT DROP"); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(tx)
	var draft uuid.UUID
	for i := 0; i < 22; i++ {
		status := "published"
		if i == 21 {
			status = "draft"
		}
		id := uuid.NewV7()
		if status == "draft" {
			draft = id
		}
		_, err = q.CreateWallpaper(ctx, dbgen.CreateWallpaperParams{ID: id, Title: fmt.Sprintf("Title %02d", i), Credit: pgtype.Text{String: "Author Alpha", Valid: true}, Status: status, ImageKey: fmt.Sprint(i), ImageSha256: strings.Repeat("a", 64), ImageType: "image/avif", ImageBytes: 1, Width: 1920, Height: 1080, PreviewKey: "preview", PreviewSha256: strings.Repeat("b", 64), PreviewBytes: 1, PreviewWidth: 480, PreviewHeight: 270, SortOrder: int32(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	count, e := q.PublishedWallpaperCount(ctx, "alpha")
	if e != nil || count != 21 {
		t.Fatal("credit search", count, e)
	}
	seen := map[uuid.UUID]bool{}
	for p := 0; p < 3; p++ {
		rows, e := q.PublishedWallpapers(ctx, dbgen.PublishedWallpapersParams{Keyword: "alpha", PageLimit: 9, PageOffset: int32(p * 9)})
		if e != nil || len(rows) != []int{9, 9, 3}[p] {
			t.Fatal("pagination", len(rows), e)
		}
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatal("duplicate page")
			}
			seen[r.ID] = true
		}
	}
	if _, e = q.PublicWallpaper(ctx, draft); databaseError(e) != ErrNotFound {
		t.Fatal("draft leak", e)
	}
	rows, e := q.PublishedWallpapers(ctx, dbgen.PublishedWallpapersParams{Keyword: "%", PageLimit: 9})
	if e != nil || len(rows) != 0 {
		t.Fatal("wildcard interpreted", e)
	}
	t.Log("PostgreSQL LIMIT/OFFSET 9/9/3, literal title/credit search, stable distinct pages and Published-only detail passed; temporary rows rolled back")
}
