package management

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
)

var tagManagementConfig = flag.String("tag-management-config", "", "absolute path to the native development YAML for PostgreSQL temporary tables")

const (
	tagGame       = "0000000000000001"
	tagCollection = "0000000000000002"
	tagAPI        = "/internal/v1/ops/games/"
)

func tagManagementTestPool(t *testing.T, ctx context.Context) (platform.Config, *pgxpool.Pool) {
	t.Helper()
	cfg, err := platform.LoadConfig(*tagManagementConfig)
	if err != nil || !filepath.IsAbs(*tagManagementConfig) || cfg.Database == nil || cfg.Database.Name != "gofurry_cn_dev" || cfg.Management == nil {
		t.Fatal("expected an explicit gofurry_cn_dev configuration")
	}
	opts, err := database.PoolConfig(*cfg.Database)
	tagMust(t, err)
	opts.MinConns, opts.MaxConns = 0, 1
	opts.AfterConnect = prepareTagManagementSchema
	pool, err := pgxpool.NewWithConfig(ctx, opts)
	tagMust(t, err)
	t.Cleanup(pool.Close)
	tagMust(t, pool.Ping(ctx))
	return cfg, pool
}

func prepareTagManagementSchema(ctx context.Context, conn *pgx.Conn) error {
	var name string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "gofurry_cn_dev" {
		return fmt.Errorf("expected the development database for temporary tables")
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		for _, table := range []string{"games", "game_collections", "game_collection_members", "game_editorial_tag_categories",
			"game_editorial_tags", "game_editorial_tag_members", "game_collection_tag_rules", "game_collection_exclusions",
			"management_operations", "catalog_publication_decisions"} {
			if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
				return err
			}
		}
		// The application and copied trigger can only reach this connection's temporary tables.
		_, err := tx.Exec(ctx, `SET search_path TO pg_temp, pg_catalog;
CREATE VIEW game_channel_locales AS SELECT id AS game_id,'zh'::text AS locale,slug AS name FROM games;
INSERT INTO games(id,public_id,slug) VALUES(uuidv7(),'0000000000000001','tag-game');
INSERT INTO game_collections(id,public_id,slug,localizations)
VALUES(uuidv7(),'0000000000000002','tag-collection','[]');`)
		if err != nil {
			return err
		}
		var definition string
		if err = tx.QueryRow(ctx, "SELECT pg_get_viewdef('public.game_collection_effective_members'::regclass)").Scan(&definition); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "CREATE VIEW game_collection_effective_members AS "+strings.ReplaceAll(definition, "public.", "pg_temp.")); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT pg_get_functiondef('public.bump_editorial_collection_revisions()'::regprocedure)").Scan(&definition); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, strings.ReplaceAll(definition, "public.", "pg_temp.")); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT pg_get_triggerdef(oid) FROM pg_trigger
WHERE tgfoid='public.bump_editorial_collection_revisions()'::regprocedure AND NOT tgisinternal ORDER BY tgname`)
		if err != nil {
			return err
		}
		definitions, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil || len(definitions) == 0 {
			return fmt.Errorf("failed to read the collection revision triggers: %w", err)
		}
		for _, definition := range definitions {
			if _, err := tx.Exec(ctx, strings.ReplaceAll(definition, "public.", "pg_temp.")); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestEditorialTagManagementPostgresHTTP(t *testing.T) {
	if *tagManagementConfig == "" {
		t.Skip("pass -tag-management-config to run native PostgreSQL and HTTP checks")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Second)
	defer cancel()
	cfg, pool := tagManagementTestPool(t, ctx)
	token, actor := uuid.NewV7().String()+uuid.NewV7().String(), uuid.NewV7().String()
	cfg.Management.Token = token
	app := New(platform.NewLogger(io.Discard), cfg, wallpaper.New(pool, nil, true),
		&Operations{Pool: pool, Catalog: catalog.New(pool), Region: "CN"})
	base := siteListen(t, app) + tagAPI
	request := func(method, path, role string, requestID string, data, out any, status int) {
		t.Helper()
		payload, err := json.Marshal(data)
		tagMust(t, err)
		req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(payload))
		tagMust(t, err)
		req.Header.Set("Content-Type", "application/json")
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-LC-Actor-ID", actor)
			req.Header.Set("X-LC-Actor-Name", "Native tag tester")
			req.Header.Set("X-LC-Actor-Role", role)
			req.Header.Set("X-LC-Region", "CN")
			req.Header.Set("X-Request-ID", requestID)
		}
		res, err := http.DefaultClient.Do(req)
		tagMust(t, err)
		defer res.Body.Close()
		body, err := io.ReadAll(io.LimitReader(res.Body, 30000))
		tagMust(t, err)
		if res.StatusCode != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, res.StatusCode, status, body)
		}
		if expected, ok := out.(string); ok {
			if !bytes.Contains(body, []byte(expected)) {
				t.Fatalf("%s %s missing %q: %s", method, path, expected, body)
			}
		} else if out != nil {
			tagMust(t, json.Unmarshal(body, out))
		}
	}
	send := func(method, path, role string, data, out any, status int) {
		t.Helper()
		request(method, path, role, uuid.NewV7().String(), data, out, status)
	}
	const categories, tags = "editorial-tag-categories", "editorial-tags"
	names := func(zh, en string) []catalog.Localization {
		return []catalog.Localization{{Locale: "zh", Name: zh, Summary: "中文简介"}, {Locale: "en", Name: en, Summary: "English summary"}}
	}
	category := func(zh, en string, order int32) catalog.EditorialCategoryDefinition {
		return catalog.EditorialCategoryDefinition{EditorialCategoryEdit: catalog.EditorialCategoryEdit{Localizations: names(zh, en), SortOrder: order}}
	}
	tag := func(over string) catalog.EditorialTagDefinition {
		return catalog.EditorialTagDefinition{EditorialTagEditDefinition: catalog.EditorialTagEditDefinition{
			Category: over, Localizations: names("剧情向", "Story Rich"),
		}}
	}
	code := func(value string) editorialCode { return editorialCode{Code: value} }
	send("GET", tags+"/manage", "", nil, nil, 401)
	send("GET", tags+"/manage", "developer", nil, nil, 200)
	send("POST", categories, "developer", category("玩法", "Gameplay", 10), nil, 403)
	send("POST", tags, "", tag("gameplay"), nil, 401)
	send("POST", tags, "developer", tag("gameplay"), nil, 403)

	rid := uuid.NewV7().String()
	request("POST", categories, "owner", rid, category("玩法", "Gameplay", 10), nil, 200)
	request("POST", categories, "owner", rid, category("重复请求", "Replay", 5), nil, 409)
	send("POST", categories, "operator", category("题材", "Theme", 20), nil, 200)
	send("PATCH", categories+"/theme", "operator", category("叙事主题", "Narrative", -5).EditorialCategoryEdit, nil, 200)
	var categoryPage struct {
		Items []editorialCategory `json:"items"`
	}
	send("GET", categories+"/manage", "owner", nil, &categoryPage, 200)
	if len(categoryPage.Items) != 2 || categoryPage.Items[0].Code != "theme" || categoryPage.Items[0].SortOrder != -5 {
		t.Fatalf("category order, edits or replay rollback failed: %+v", categoryPage.Items)
	}
	send("GET", categories+"/manage?q=Narrative&state=active", "owner", nil, &categoryPage, 200)
	if len(categoryPage.Items) != 1 || categoryPage.Items[0].Code != "theme" {
		t.Fatalf("category search or immutable code failed: %+v", categoryPage.Items)
	}
	send("POST", categories+"/archive", "operator", code("theme"), nil, 200)
	send("GET", categories+"/manage?state=archived", "owner", nil, &categoryPage, 200)
	if len(categoryPage.Items) != 1 || !categoryPage.Items[0].Archived {
		t.Fatal("archived category was missing")
	}
	send("POST", tags, "operator", tag("theme"), catalog.ErrEditorialCategoryArchived.Error(), 409)
	send("POST", tags, "operator", tag("missing"), nil, 404)
	send("POST", categories+"/restore", "operator", code("theme"), nil, 200)
	send("POST", tags, "operator", tag("gameplay"), nil, 200)
	send("POST", tags, "operator", tag("gameplay"), catalog.ErrEditorialExists.Error(), 409)
	fallback := catalog.EditorialTagDefinition{EditorialTagEditDefinition: catalog.EditorialTagEditDefinition{
		Category: "gameplay", Localizations: []catalog.Localization{{Locale: "zh", Name: "仅中文"}},
	}}
	send("POST", tags, "owner", fallback, nil, 200)
	send("POST", tags, "owner", fallback, catalog.ErrEditorialExists.Error(), 409)
	var secondary string
	tagMust(t, pool.QueryRow(ctx, "SELECT code FROM game_editorial_tags WHERE code='tag-'||id::text AND uuid_extract_version(id)=7").Scan(&secondary))
	send("POST", categories+"/archive", "owner", code("gameplay"), catalog.ErrEditorialCategoryInUse.Error(), 409)
	send("PATCH", tags+"/story-rich", "owner", map[string]any{"code": "changed"}, nil, 400)

	assign := func(rev int64, items ...catalog.EditorialTagAssignment) catalog.EditorialTagEdit {
		return catalog.EditorialTagEdit{Revision: rev, Tags: items}
	}
	mainTag := catalog.EditorialTagAssignment{Code: "story-rich", Role: "primary"}
	subTag := catalog.EditorialTagAssignment{Code: secondary, Role: "secondary"}
	var game catalog.ManagementResult
	send("PATCH", tagGame+"/tags", "operator", assign(1, mainTag, subTag), &game, 200)
	send("PATCH", tagGame+"/tags", "operator", assign(1, mainTag), nil, 409)
	send("PATCH", tagGame+"/tags", "operator", assign(game.Revision, mainTag, catalog.EditorialTagAssignment{Code: secondary, Role: "primary"}), nil, 400)
	var assigned catalog.EditorialGameTags
	send("GET", tagGame+"/tags", "owner", nil, &assigned, 200)
	if len(assigned.Tags) != 2 || assigned.Tags[0].Role != "primary" || assigned.Tags[1].Role != "secondary" {
		t.Fatalf("incorrect assignment roles: %+v", assigned.Tags)
	}
	send("POST", tags+"/archive", "owner", code("story-rich"), catalog.ErrEditorialTagInUse.Error(), 409)
	var collection catalog.ManagementResult
	rulePath := "collections/" + tagCollection + "/rules"
	rules := []string{"story-rich"}
	send("PATCH", rulePath, "operator", catalog.CollectionRulesEdit{Revision: 1, Tags: &rules}, &collection, 200)
	checkMembers := func(wantRevision int64, wantMembers int) {
		t.Helper()
		var revision int64
		var count int
		tagMust(t, pool.QueryRow(ctx, `SELECT revision,(SELECT count(*) FROM game_collection_effective_members) FROM game_collections`).Scan(&revision, &count))
		if revision != wantRevision || count != wantMembers {
			t.Fatalf("collection revision/members %d/%d, want %d/%d", revision, count, wantRevision, wantMembers)
		}
	}
	checkMembers(collection.Revision, 1)
	send("PATCH", tags+"/story-rich", "operator", catalog.EditorialTagEditDefinition{Category: "theme", Localizations: names("故事向", "Story Driven")}, nil, 200)
	checkMembers(collection.Revision+1, 1)
	send("PATCH", tags+"/story-rich", "operator", catalog.EditorialTagEditDefinition{Category: "theme", Localizations: names("叙事向", "Narrative Driven")}, nil, 200)
	checkMembers(collection.Revision+1, 1)
	var tagPage struct {
		Items []editorialTag `json:"items"`
	}
	send("GET", tags+"/manage?q=Narrative&category=theme", "owner", nil, &tagPage, 200)
	if len(tagPage.Items) != 1 || tagPage.Items[0].Code != "story-rich" || tagPage.Items[0].CategoryName != "叙事主题" ||
		!bytes.Contains(tagPage.Items[0].Localizations, []byte("English summary")) {
		t.Fatalf("tag names, immutable code or category missing: %+v", tagPage.Items)
	}
	send("GET", tags+"?q=%E5%8F%99%E4%BA%8B", "owner", nil, "story-rich", 200)
	send("PATCH", tagGame+"/tags", "owner", assign(game.Revision, subTag), &game, 200)
	checkMembers(collection.Revision+2, 0)
	send("POST", tags+"/archive", "owner", code("story-rich"), catalog.ErrEditorialTagInUse.Error(), 409)
	rules = []string{}
	send("PATCH", rulePath, "owner", catalog.CollectionRulesEdit{Revision: collection.Revision + 2, Tags: &rules}, &collection, 200)
	send("POST", tags+"/archive", "owner", code("story-rich"), nil, 200)
	send("GET", tags+"/manage?category=theme&state=archived", "owner", nil, &tagPage, 200)
	if len(tagPage.Items) != 1 || !tagPage.Items[0].Archived {
		t.Fatal("archived tag was missing")
	}
	var active []struct {
		Code string `json:"code"`
	}
	send("GET", tags+"?q=narrative", "owner", nil, &active, 200)
	if len(active) != 0 {
		t.Fatal("game picker included an archived tag")
	}
	send("PATCH", tagGame+"/tags", "owner", assign(game.Revision, mainTag), nil, 400)
	send("GET", tagGame+"/tags", "owner", nil, &assigned, 200)
	if len(assigned.Tags) != 1 || assigned.Tags[0].Code != secondary || assigned.Revision != game.Revision {
		t.Fatal("failed archived assignment did not roll back")
	}
	send("POST", categories+"/archive", "owner", code("theme"), nil, 200)
	send("POST", tags+"/restore", "owner", code("story-rich"), catalog.ErrEditorialCategoryArchived.Error(), 409)
	send("POST", categories+"/restore", "owner", code("theme"), nil, 200)
	send("POST", tags+"/restore", "operator", code("story-rich"), nil, 200)
	send("GET", tags+"?q=narrative", "owner", nil, "story-rich", 200)
	checkMembers(collection.Revision, 0)

	var audit []string
	var status string
	var publicationDecisions int
	tagMust(t, pool.QueryRow(ctx, `SELECT array_agg(DISTINCT actor_role), (SELECT status FROM games),
 (SELECT count(*) FROM catalog_publication_decisions) FROM management_operations WHERE actor_id=$1 AND action LIKE 'game.tag%'`, actor).
		Scan(&audit, &status, &publicationDecisions))
	if !slices.Equal(audit, []string{"operator", "owner"}) || status != catalog.Draft || publicationDecisions != 0 {
		t.Fatalf("incorrect tag audit or publication side effects: %v %s %d", audit, status, publicationDecisions)
	}
}

func tagMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
