//go:build integration

package wallpaper

import (
	json "encoding/json/v2"
	"os"
	"strings"
	"testing"
)

func TestRealDevelopmentPublication(t *testing.T) {
	ctx, s, _ := integration(t, true)
	actor := testActor()
	image, e := os.Open("testdata/image.avif")
	if e != nil {
		t.Fatal(e)
	}
	defer image.Close()
	preview, e := os.Open("testdata/preview.webp")
	if e != nil {
		t.Fatal(e)
	}
	defer preview.Close()
	row, e := s.Upload(ctx, actor, Metadata{Title: "Reviewed development fixture"}, image, preview, "image/avif", "image/webp")
	if e != nil {
		t.Fatal(e)
	}
	if catalog, e := s.Catalog(ctx); e != nil || len(catalog) != 0 {
		t.Fatal("draft leaked", e)
	}
	if _, e = s.Transition(ctx, actor, row.ID, 1, "published", false); e != ErrInvalid {
		t.Fatal("published without human confirmation")
	}
	result, e := s.Transition(ctx, actor, row.ID, 1, "published", true)
	if e != nil || !result.Changed || result.Wallpaper.Revision != 2 || result.Wallpaper.PublishedAt == nil {
		t.Fatal("publish failed", e)
	}
	repeat, e := s.Transition(ctx, actor, row.ID, 2, "published", true)
	if e != nil || repeat.Changed {
		t.Fatal("duplicate publication changed state", e)
	}
	catalog, e := s.Catalog(ctx)
	if e != nil || len(catalog) != 1 || !catalog[0].SFW {
		t.Fatal("published catalog failed", e)
	}
	encoded, _ := json.Marshal(catalog)
	for _, word := range []string{"actor", "audit", "revision", "password", "token"} {
		if strings.Contains(string(encoded), word) {
			t.Fatal("private catalog field")
		}
	}
	if _, e = s.Transition(ctx, actor, row.ID, 2, "archived", false); e != nil {
		t.Fatal(e)
	}
	if catalog, e := s.Catalog(ctx); e != nil || len(catalog) != 0 {
		t.Fatal("archive cache was not invalidated", e)
	}
	if _, e = s.Transition(ctx, actor, row.ID, 3, "published", true); e != ErrConflict {
		t.Fatal("archived directly published")
	}
	if _, e = s.Transition(ctx, actor, row.ID, 3, "draft", false); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Edit(ctx, actor, row.ID, 3, Metadata{Title: "Stale edit"}); e != ErrConflict {
		t.Fatal("stale revision accepted")
	}
	if e = s.Delete(ctx, actor, row.ID, 4); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Detail(ctx, row.ID); e != ErrNotFound {
		t.Fatal("draft not deleted")
	}
	var count int
	if e = s.pool.QueryRow(ctx, "SELECT count(*) FROM wallpaper_audit_logs WHERE wallpaper_id=$1", row.ID).Scan(&count); e != nil || count != 5 {
		t.Fatal("audit missing or duplicated", count, e)
	}
	t.Log("REAL DEVELOPMENT R2 + POSTGRESQL: Draft→Published→Archived→Draft, repeat/conflict rules, immediate catalog invalidation, draft deletion and retained audit passed")
}
