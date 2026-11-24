package management

import (
	"bytes"
	"encoding/base64"
	json "encoding/json/v2"
	"flag"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"uuid"
)

var assetLibraryConfig = flag.String("asset-library-config", "", "Loopback MEDIA-U1 PostgreSQL fixture with real Consumer source rows")

func TestGameAssetQueryBoundaries(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	cfg := platform.Config{Management: &platform.ManagementConfig{Token: token}}
	cfg.Deployment.Region = "CN"
	app := New(platform.NewLogger(io.Discard), cfg, wallpaper.New(nil, nil, true), &Operations{Catalog: catalog.New(nil), Region: "CN"})
	base := "games/" + mediaFixtureGame
	for _, path := range []string{
		"/assets?after=", "/assets?after=" + uuid.NewV4().String(), "/assets?unknown=x",
		"/assets?locale=en&locale=zh-cn", "/assets?source_role=../../bad", "/assets?kind=movie",
		"/asset-presentation?locale=en&surface=published&slot=artwork",
		"/asset-presentation?locale=en&surface=published&slot=gallery&after=-1",
	} {
		mediaCursorRequest(t, app, token, base+path, http.StatusBadRequest, nil)
	}
	mediaCursorRequest(t, app, "", base+"/assets", http.StatusUnauthorized, nil)
}

func TestGameAssetLibraryPostgres(t *testing.T) {
	if *assetLibraryConfig == "" {
		t.Skip("pass -asset-library-config for the isolated MEDIA-U1 integration")
	}
	ctx := t.Context()
	cfg, err := platform.LoadConfig(*assetLibraryConfig)
	if err != nil || cfg.Management == nil || cfg.Database == nil || cfg.Database.Host != "127.0.0.1" ||
		!strings.HasPrefix(cfg.Database.Name, "gofurry_media_u1_") || !strings.HasSuffix(cfg.Database.Name, "_test") {
		t.Fatal("expected designated MEDIA-U1 loopback test database")
	}
	pool, err := database.Open(ctx, *cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := catalog.New(pool)
	app := New(platform.NewLogger(io.Discard), cfg, wallpaper.New(pool, nil, true), &Operations{Pool: pool, Catalog: s, Region: "CN"})
	var gameID uuid.UUID
	var publicID string
	err = pool.QueryRow(ctx, `
SELECT g.id,g.public_id FROM games g JOIN game_sources s ON s.game_id=g.id
WHERE s.platform_code='steam' AND s.external_key='434000' AND EXISTS (SELECT 1 FROM game_asset_versions v WHERE v.game_id=g.id)`).Scan(&gameID, &publicID)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.NewV7()
	base := "games/" + publicID
	operation := func(path, method, role string, body any, want int, out any) uuid.UUID {
		t.Helper()
		requestID := uuid.NewV7()
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, mediaOps+path, bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.Management.Token)
		req.Header.Set("X-LC-Actor-ID", actor.String())
		req.Header.Set("X-LC-Actor-Name", "Media fixture operator")
		req.Header.Set("X-LC-Actor-Role", role)
		req.Header.Set("X-LC-Region", cfg.Deployment.Region)
		req.Header.Set("X-Request-ID", requestID.String())
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s %s gave %d instead of %d: %s", method, path, res.StatusCode, want, b)
		}
		if out != nil {
			if err := json.UnmarshalRead(res.Body, out); err != nil {
				t.Fatal(err)
			}
		}
		return requestID
	}
	// Create an additional platform identity through the existing GameSource
	// table. The fixture assigns no real itch collection binding or media.
	if _, err = pool.Exec(ctx, `
INSERT INTO game_sources(id,game_id,platform_code,external_key,source_url)
VALUES(uuidv7(),$1,'itch_io','fixture-identity','https://example.itch.io/fixture')`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
INSERT INTO game_assets(game_id,game_source_id,platform_code,external_key,kind,category,source_family,source_role,source_identity)
SELECT game_id,id,platform_code,external_key,'image','artwork','fixture','header','slot'
FROM game_sources WHERE game_id=$1 AND platform_code IN ('steam','itch_io')`, gameID); err != nil {
		t.Fatal(err)
	}
	var providers int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM game_assets WHERE game_id=$1 AND source_family='fixture' AND source_identity='slot'", gameID).Scan(&providers); err != nil || providers != 2 {
		t.Fatalf("provider identities collided: %d %v", providers, err)
	}
	// The fixture copies provenance from a real persisted source but supplies
	// synthetic positions and identities to exercise business capacity.
	if _, err = pool.Exec(ctx, `
WITH base AS (
 SELECT a.game_id,a.game_source_id,a.external_key,a.platform_code,v.binding_id,v.observation_id,v.scope_key,v.egress_region,v.locale,v.origin
 FROM game_assets a JOIN game_asset_versions v ON a.id=v.asset_id
 WHERE a.game_id=$1 ORDER BY v.id LIMIT 1
), new_assets AS (
 INSERT INTO game_assets(game_id,game_source_id,platform_code,external_key,kind,category,source_family,source_role,source_identity)
 SELECT game_id,game_source_id,platform_code,external_key,
 CASE WHEN n>70 THEN 'video' ELSE 'image' END,
 CASE WHEN n<=35 THEN 'artwork' WHEN n<=70 THEN 'screenshot' ELSE 'trailer' END,
 'scale_fixture','fixture_role',n::text FROM base CROSS JOIN generate_series(1,94) n
 RETURNING id,game_id,kind,source_identity
), versions AS (
 INSERT INTO game_asset_versions(game_id,asset_id,binding_id,observation_id,scope_key,egress_region,locale,origin,name,original_order,highlight,truncated)
 SELECT b.game_id,a.id,b.binding_id,b.observation_id,b.scope_key,b.egress_region,b.locale,b.origin,'Scale fixture',a.source_identity::int,NULL,false
 FROM new_assets a CROSS JOIN base b ORDER BY a.source_identity::int RETURNING id,original_order
)
INSERT INTO game_asset_variants(version_id,variant_key,url,format,source_role,source_asset_id,source_slot,source_digest,source_family)
SELECT v.id,CASE WHEN v.original_order>70 THEN 'mp4_max' ELSE 'standard' END,
 CASE WHEN v.original_order>70 THEN 'https://shared.steamstatic.com/synthetic.mp4' ELSE 'https://shared.steamstatic.com/synthetic.jpg' END,
 CASE WHEN v.original_order>70 THEN 'mp4' ELSE 'jpg' END,'fixture_role',v.original_order::text,'fixture','',''
FROM versions v`, gameID); err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, 0, 94)
	seen := make(map[uuid.UUID]bool)
	after := ""
	for i, want := range []int{32, 32, 30} {
		var page catalog.GameAssetPage
		mediaCursorRequest(t, app, cfg.Management.Token, base+"/assets?source_role=fixture_role"+after, http.StatusOK, &page)
		if len(page.Items) != want || (page.Next == nil) != (i == 2) {
			t.Fatalf("bad page %d: %d", i, len(page.Items))
		}
		for _, a := range page.Items {
			v := a.VersionID
			if seen[v] {
				t.Fatal("repeated version")
			}
			seen[v] = true
			ids = append(ids, v)
		}
		if i == 0 {
			first := page.Items[0]
			var detail catalog.GameAssetDetail
			mediaCursorRequest(t, app, cfg.Management.Token, base+"/assets/"+first.GameAsset.ID.String()+"/versions/"+first.VersionID.String(), http.StatusOK, &detail)
			if len(detail.Variants) != 1 || detail.Variants[0].SourceAssetID != "1" || detail.VersionID != first.VersionID {
				t.Fatal("asset detail returned wrong provenance")
			}
		}
		if page.Next != nil {
			after = "&after=" + page.Next.String()
		}
	}
	initial, err := s.ManagementGame(ctx, publicID)
	if err != nil {
		t.Fatal(err)
	}
	var result catalog.ManagementResult
	library := base + "/asset-library"
	presentation := base + "/asset-presentation"
	edit := catalog.GameAssetLibraryEdit{Revision: initial.Revision, Add: ids[:64]}
	operation(library, http.MethodPatch, "developer", edit, http.StatusForbidden, nil)
	request := operation(library, http.MethodPatch, "operator", edit, http.StatusOK, &result)
	prior := initial.Revision
	edit = catalog.GameAssetLibraryEdit{Revision: prior, Add: ids[64:]}
	operation(library, http.MethodPatch, "operator", edit, http.StatusConflict, nil)
	edit.Revision = result.Revision
	operation(library, http.MethodPatch, "operator", edit, http.StatusOK, &result)
	operation("receipts/"+request.String(), http.MethodGet, "operator", nil, http.StatusOK, nil)
	rev := result.Revision
	operation(library, http.MethodPatch, "operator", catalog.GameAssetLibraryEdit{Revision: rev, Add: ids[:1]}, http.StatusOK, &result)
	if result.Revision != rev || *result.Affected != 0 {
		t.Fatal("duplicate membership changed revision")
	}
	var publishedBefore, publishedAfter string
	if err := pool.QueryRow(ctx, "SELECT md5(jsonb_agg(to_jsonb(p) ORDER BY locale)::text) FROM game_published_localizations p WHERE game_id=$1", gameID).Scan(&publishedBefore); err != nil {
		t.Fatal(err)
	}
	part := catalog.GameAssetPresentationEdit{Revision: result.Revision, Locale: "en"}
	for i, v := range ids {
		if len(part.Set) == 64 {
			operation(presentation, http.MethodPatch, "operator", part, http.StatusOK, &result)
			part.Revision, part.Set = result.Revision, nil
		}
		part.Set = append(part.Set, catalog.GameAssetPlacement{GameAssetPosition: catalog.GameAssetPosition{Slot: "gallery", Position: int64(i) * 10}, VersionID: v})
	}
	operation(presentation, http.MethodPatch, "operator", part, http.StatusOK, &result)
	operation(library, http.MethodPatch, "operator", catalog.GameAssetLibraryEdit{Revision: result.Revision, Remove: ids[:1]}, http.StatusOK, &result)
	orderEdit := catalog.GameAssetPresentationEdit{Locale: "en", Revision: result.Revision,
		Remove: []catalog.GameAssetPosition{{Slot: "gallery", Position: 0}},
		Set:    []catalog.GameAssetPlacement{{GameAssetPosition: catalog.GameAssetPosition{Slot: "gallery", Position: 5}, VersionID: ids[1]}}}
	operation(presentation, http.MethodPatch, "operator", orderEdit, http.StatusOK, &result)
	query := presentation + "?locale=en&surface=draft&slot=gallery"
	after = ""
	total := 0
	for i, want := range []int{32, 32, 30} {
		var page catalog.GameAssetPresentationPage
		mediaCursorRequest(t, app, cfg.Management.Token, query+after, http.StatusOK, &page)
		if len(page.Items) != want || (page.Next == nil) != (i == 2) {
			t.Fatalf("bad gallery page: %d", len(page.Items))
		}
		if i == 0 && (page.Items[0].Position != 5 || page.Items[0].VersionID != ids[1]) {
			t.Fatal("manual reorder lost")
		}
		if page.Next != nil {
			after = "&after=" + strconv.FormatInt(*page.Next, 10)
		}
		total += len(page.Items)
	}
	if total != 94 {
		t.Fatal("gallery was truncated")
	}
	if err := pool.QueryRow(ctx, "SELECT md5(jsonb_agg(to_jsonb(p) ORDER BY locale)::text) FROM game_published_localizations p WHERE game_id=$1", gameID).Scan(&publishedAfter); err != nil || publishedBefore != publishedAfter {
		t.Fatalf("library or presentation edit changed legacy publication: %v", err)
	}
	view, err := s.ManagementGame(ctx, publicID)
	if err != nil || !view.HasUnpublishedChanges {
		t.Fatalf("new presentation draft not detected: %v", err)
	}
	operation(base+"/publish-changes", http.MethodPost, "operator", catalog.GamePublicationReview{Revision: view.Revision,
		PreviewToken: view.PreviewToken, ConfirmRelevance: true, ContentRating: "sfw"}, http.StatusOK, &result)
	var next catalog.GameAssetPresentationPage
	mediaCursorRequest(t, app, cfg.Management.Token, presentation+"?locale=en&surface=published&slot=gallery", http.StatusOK, &next)
	if len(next.Items) != 32 || *next.Revision != result.Revision || next.Items[0].Position != 5 {
		t.Fatal("explicit publication did not snapshot the presentation")
	}
}
