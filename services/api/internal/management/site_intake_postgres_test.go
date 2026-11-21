package management

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/handlers"
	"github.com/librecosta/gofurry/services/api/internal/navigation"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
)

var siteIntakeConfig = flag.String("site-intake-config", "", "absolute path to the native development YAML")

func TestSiteIntakePostgresHTTP(t *testing.T) {
	if *siteIntakeConfig == "" {
		t.Skip("pass -site-intake-config to check the Site contract in PostgreSQL temporary tables")
	}
	cfg, err := platform.LoadConfig(*siteIntakeConfig)
	if err != nil || !filepath.IsAbs(*siteIntakeConfig) || cfg.Database == nil || cfg.Database.Name != "gofurry_cn_dev" || cfg.Management == nil {
		t.Fatal("expected an explicit gofurry_cn_dev configuration")
	}
	opts, err := database.PoolConfig(*cfg.Database)
	siteMust(t, err)
	opts.MinConns, opts.MaxConns = 0, 1
	migration, err := os.ReadFile("../../migrations/202611210001_site_intake.sql")
	siteMust(t, err)
	opts.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return prepareSiteIntakeSchema(ctx, conn, string(migration))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, opts)
	siteMust(t, err)
	defer pool.Close()
	siteMust(t, pool.Ping(ctx))

	token, actor := uuid.NewV7().String()+uuid.NewV7().String(), uuid.NewV7().String()
	cfg.Management.Token = token
	sites := catalog.New(pool)
	admin := New(platform.NewLogger(io.Discard), cfg, wallpaper.New(pool, nil, true), &Operations{Pool: pool, Catalog: sites, Region: "CN"})
	api := fiber.New(fiber.Config{JSONEncoder: platform.MarshalJSON})
	nav := navigation.New(pool)
	api.Get("/api/v1/sites/directory", handlers.NavigationDirectory(nav, "CN"))
	api.Get("/api/v1/sites/:publicID/related", handlers.NavigationRelated(nav))
	adminURL, apiURL := siteListen(t, admin), siteListen(t, api)
	request := func(method, origin, path, role string, payload any, want int, into any) {
		t.Helper()
		body, err := json.Marshal(payload)
		siteMust(t, err)
		req, err := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(body))
		siteMust(t, err)
		req.Header.Set("Content-Type", "application/json")
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-LC-Actor-ID", actor)
			req.Header.Set("X-LC-Actor-Name", "Test administrator")
			req.Header.Set("X-LC-Actor-Role", role)
			req.Header.Set("X-LC-Region", "CN")
			req.Header.Set("X-Request-ID", uuid.NewV7().String())
		}
		response, err := http.DefaultClient.Do(req)
		siteMust(t, err)
		defer response.Body.Close()
		if response.StatusCode != want {
			data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, response.StatusCode, want, data)
		}
		if into != nil {
			siteMust(t, json.UnmarshalRead(response.Body, into))
		}
	}
	const mgmt = "/internal/v1/ops/sites"
	intake := map[string]any{
		"addresses":        []catalog.Address{{Kind: "primary", URL: "www.例子.example"}},
		"primary_category": "gallery",
		"localizations":    []catalog.Localization{{Locale: "zh", Name: "网站画册"}},
	}
	var created catalog.ManagementResult
	request("POST", adminURL, mgmt, "developer", intake, 403, nil)
	request("POST", adminURL, mgmt, "owner", intake, 200, &created)
	var detail catalog.SiteManagement
	request("GET", adminURL, mgmt+"/"+created.Target, "owner", nil, 200, &detail)
	if detail.Status != catalog.Draft || detail.Slug != "xn-fsqu00a-example" || detail.Primary != "gallery" ||
		len(detail.Localizations) != 1 || detail.Localizations[0].Name != "网站画册" || !catalog.ValidPublicID(created.Target) ||
		len(detail.Addresses) != 1 || detail.Addresses[0].Url != "https://www.xn--fsqu00a.example" {
		t.Fatalf("unexpected new draft: %+v", detail)
	}
	intake["addresses"] = []catalog.Address{{Kind: "primary", URL: "http://www.xn--fsqu00a.example/"}}
	var duplicate struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	request("POST", adminURL, mgmt, "owner", intake, 409, &duplicate)
	if !strings.HasSuffix(duplicate.Error.Message, created.Target) {
		t.Fatal("duplicate error did not identify the existing site")
	}

	var changed catalog.ManagementResult
	edit := catalog.SiteManagementEdit{Revision: created.Revision, Primary: "gallery", Secondary: []string{"resources"}, Tags: []string{"portfolio", "tutorial"},
		Addresses: []catalog.Address{{Kind: "primary", URL: "https://another.example/path"}},
		Localizations: []catalog.SiteLocalization{
			{Localization: catalog.Localization{Locale: "zh", Name: "画册新名称", Summary: "中文简介"}},
			{Localization: catalog.Localization{Locale: "en", Name: "Gallery", Summary: "English summary"}},
		},
	}
	request("PATCH", adminURL, mgmt+"/"+created.Target, "owner", edit, 200, &changed)
	request("GET", adminURL, mgmt+"/"+created.Target, "owner", nil, 200, &detail)
	if detail.Slug != "xn-fsqu00a-example" || detail.PublicID != created.Target ||
		len(detail.Localizations) != 2 || detail.Localizations[0].Summary != "English summary" || detail.Localizations[1].Summary != "中文简介" ||
		!slices.Equal(detail.Tags, edit.Tags) || !slices.Equal(detail.Secondary, edit.Secondary) {
		t.Fatalf("edit lost stable identity or bilingual data: %+v", detail)
	}

	operation := func(id, action string, rev int64, sfw bool, status int) catalog.ManagementResult {
		t.Helper()
		var result catalog.ManagementResult
		request("POST", adminURL, mgmt+"/"+id+"/"+action, "owner", publication{Revision: rev, ConfirmSFW: sfw, Reason: "Review the site"},
			status, nilIfError(status, &result))
		return result
	}
	edit.Revision, edit.Primary, edit.Secondary = changed.Revision, "", nil
	request("PATCH", adminURL, mgmt+"/"+created.Target, "owner", edit, 200, &changed)
	operation(created.Target, "publish", changed.Revision, true, 422)
	request("GET", adminURL, mgmt+"/"+created.Target, "owner", nil, 200, &detail)
	if detail.Status != catalog.Draft || detail.Rating != "unreviewed" || detail.Revision != changed.Revision {
		t.Fatal("failed publication changed the site or its review")
	}
	edit.Revision, edit.Primary = detail.Revision, "gallery"
	request("PATCH", adminURL, mgmt+"/"+created.Target, "owner", edit, 200, &changed)
	operation(created.Target, "publish", changed.Revision, false, 422)
	published := operation(created.Target, "publish", changed.Revision, true, 200)

	intake["addresses"] = []catalog.Address{{Kind: "primary", URL: "https://www.xn--fsqu00a.example/other"}}
	intake["localizations"] = []catalog.Localization{{Locale: "zh", Name: "相关站点"}, {Locale: "en", Summary: "Only an English summary"}}
	intake["primary_category"], intake["tags"] = "community", edit.Tags
	var related catalog.ManagementResult
	request("POST", adminURL, mgmt, "owner", intake, 200, &related)
	request("GET", adminURL, mgmt+"/"+related.Target, "owner", nil, 200, &detail)
	if related.Target == created.Target || detail.Slug != "xn-fsqu00a-example" || len(detail.Localizations) != 2 ||
		detail.Localizations[0].Name != "相关站点" || detail.Localizations[0].Summary != "Only an English summary" {
		t.Fatal("same-host paths with different IDs or short summary fallback were lost")
	}
	operation(related.Target, "publish", related.Revision, true, 200)
	var listings navigation.Directory
	request("GET", apiURL, "/api/v1/sites/directory?locale=en&tags=portfolio", "", nil, 200, &listings)
	if listings.Total != 2 {
		t.Fatal("published sites lost tag filtering")
	}
	request("GET", apiURL, "/api/v1/sites/directory?language=en", "", nil, 400, nil)
	var recommendations struct {
		Items []navigation.Card `json:"items"`
	}
	request("GET", apiURL, "/api/v1/sites/"+created.Target+"/related", "", nil, 200, &recommendations)
	if len(recommendations.Items) != 1 || recommendations.Items[0].PublicID != related.Target {
		t.Fatal("shared tags did not produce a related site")
	}
	withdrawn := operation(created.Target, "unpublish", published.Revision, false, 200)
	request("GET", apiURL, "/api/v1/sites/directory?tags=portfolio", "", nil, 200, &listings)
	if listings.Total != 1 {
		t.Fatal("withdrawn site remained in navigation")
	}
	operation(created.Target, "restore", withdrawn.Revision, false, 200)
	request("GET", apiURL, "/api/v1/sites/directory?tags=portfolio", "", nil, 200, &listings)
	if listings.Total != 2 {
		t.Fatal("restored site did not return to navigation")
	}
	legacy, err := dbgen.New(pool).PublishedSite(ctx, "0000000000000001")
	siteMust(t, err)
	if legacy.Slug != "old-link" {
		t.Fatal("the migration changed an existing site's public link")
	}
}

func nilIfError(status int, output any) any {
	if status >= 400 {
		return nil
	}
	return output
}

func prepareSiteIntakeSchema(ctx context.Context, conn *pgx.Conn, migration string) error {
	var name string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "gofurry_cn_dev" {
		return fmt.Errorf("unexpected development database")
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		for _, table := range []string{"sites", "site_localizations", "site_addresses", "site_navigation_reviews",
			"catalog_publication_decisions", "management_operations", "site_yearbooks", "site_yearbook_revisions",
			"site_navigation_heat", "site_navigation_heat_state", "collection_bindings", "site_monthly_coverage"} {
			if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
				return err
			}
		}
		// Confine all subsequent DDL and app statements to this test connection's temporary schema.
		_, err := tx.Exec(ctx, `SET search_path TO pg_temp, pg_catalog;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS supported_languages text[] NOT NULL DEFAULT '{}';
CREATE VIEW site_public_navigation AS SELECT * FROM sites WHERE status='published' AND content_rating='sfw';
CREATE VIEW site_public_history AS SELECT * FROM sites WHERE status IN ('published','archived');
CREATE INDEX sites_navigation_languages ON sites USING gin(supported_languages);
INSERT INTO sites(id,public_id,slug,status,published_at,content_rating,supported_languages)
VALUES(uuidv7(),'0000000000000001','old-link','published',now(),'sfw',ARRAY['zh']);
INSERT INTO site_localizations(site_id,locale,name,summary) SELECT id,'zh','存量网站','' FROM sites;`)
		if err != nil {
			return err
		}
		attempt, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		_, guarded := attempt.Exec(ctx, migration)
		if err = attempt.Rollback(ctx); err != nil {
			return err
		}
		if guarded == nil || !strings.Contains(guarded.Error(), "Export and review existing site supported_languages") {
			return fmt.Errorf("migration did not stop on existing languages: %v", guarded)
		}
		// Fixture-only data with a nonempty language array has served its guard test.
		if _, err = tx.Exec(ctx, "UPDATE sites SET supported_languages='{}'"); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, migration)
		return err
	})
}

func siteListen(t *testing.T, app *fiber.App) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	siteMust(t, err)
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() {
		_ = app.ShutdownWithTimeout(time.Second)
		_ = listener.Close()
		if err := <-done; err != nil {
			t.Errorf("stop isolated HTTP service: %v", err)
		}
	})
	return "http://" + listener.Addr().String()
}

func siteMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
