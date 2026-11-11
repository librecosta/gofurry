//go:build integration

package database_test

import (
	"context"
	"flag"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/server"
	"github.com/rs/zerolog"
	"io"
	"strings"
	"testing"
	"time"
)

var emptyConfig = flag.String("empty-wallpaper-config", "", "Explicit isolated gofurry_c2_*_empty_test YAML")

func TestExplicitMigrationOnly(t *testing.T) {
	if *emptyConfig == "" {
		t.Skip("requires explicit empty test database")
	}
	cfg, e := platform.LoadConfig(*emptyConfig)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Database == nil || !strings.HasPrefix(cfg.Database.Name, "gofurry_c2_") || !strings.HasSuffix(cfg.Database.Name, "_empty_test") || cfg.Database.User != "gofurry_c2_test" || cfg.Management != nil {
		t.Fatal("not an isolated empty test configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, e := database.Open(ctx, *cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	var owner string
	pool.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()").Scan(&owner)
	if owner != cfg.Database.User {
		t.Fatal("database ownership mismatch")
	}
	count := func() int {
		var n int
		if e := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if count() != 0 {
		t.Fatal("expected an empty isolated database")
	}
	if e = database.Migrate(ctx, *cfg.Database, "../../migrations", "status", io.Discard); e != nil {
		t.Fatal(e)
	}
	if count() != 0 {
		t.Fatal("status wrote schema")
	}
	app := server.New(zerolog.Nop(), cfg)
	if e = app.Wallpapers.Ready(ctx); e == nil {
		t.Fatal("unmigrated schema reported ready")
	}
	if e = app.App.Shutdown(); e != nil {
		t.Fatal(e)
	}
	if count() != 0 {
		t.Fatal("server automatically migrated")
	}
	if e = database.Migrate(ctx, *cfg.Database, "../../migrations", "up", io.Discard); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := pool.Exec(context.Background(), "DROP VIEW site_public_navigation,site_public_history; DROP TABLE site_navigation_reviews,site_navigation_features,site_interaction_days,site_navigation_heat_state,site_navigation_heat,site_yearbooks,site_yearbook_revisions,site_monthly_coverage,collection_sources,collection_bindings,collection_plans,collection_plan_bindings,collection_consumer,collection_gaps,collection_current,site_evidence_changes,site_daily_coverage,game_evidence_news,game_evidence_players,game_sources,game_localizations,games,site_addresses,site_localizations,sites,wallpaper_audit_logs,start_wallpapers,goose_db_version; DROP FUNCTION preserve_catalog_identity(),navigation_distinct_codes(text[])"); e != nil {
			t.Error("isolated test cleanup failed")
		}
	}()
	if count() != 30 {
		t.Fatal("wrong migrated schema count")
	}
}
