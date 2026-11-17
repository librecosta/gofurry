//go:build integration

package management

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
)

var assetConfig = flag.String("management-asset-config", "", "Disposable local gofurry_d4_assets_test database")

func TestManagedSiteAsset(t *testing.T) {
	if *assetConfig == "" {
		t.Skip("explicit disposable database required")
	}
	cfg, err := platform.LoadConfig(*assetConfig)
	if err != nil {
		t.Fatal(err)
	}
	db := cfg.Database
	if db == nil || db.Host != "127.0.0.1" || db.Name != "gofurry_d4_assets_test" || cfg.Server.Environment != "development" || cfg.Deployment.Region != "CN" {
		t.Fatal("expected the dedicated loopback D4 asset fixture")
	}
	if err = database.Migrate(t.Context(), *db, "../../migrations", "up", io.Discard); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(t.Context(), *db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const testToken = "local test bearer with no external authority"
	cfg.Management = &platform.ManagementConfig{Token: testToken}
	type blob struct {
		data       []byte
		mime, hash string
	}
	var mu sync.Mutex
	written := map[string]blob{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticated := strings.HasPrefix(r.URL.Path, "/fixture/")
		public := strings.HasPrefix(r.URL.Path, "/public/")
		if !authenticated && !public {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/fixture/"), "/public/")
		if public && r.Header.Get("Authorization") != "" {
			t.Error("signed credentials sent to public delivery")
		}
		mu.Lock()
		defer mu.Unlock()
		object, exists := written[key]
		if r.Method == http.MethodPut {
			if !authenticated || r.Header.Get("If-None-Match") != "*" || !strings.Contains(r.Header.Get("Cache-Control"), "immutable") {
				t.Error("immutable PUT requirements missing")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if exists {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			b, e := io.ReadAll(io.LimitReader(r.Body, (4<<20)+1))
			if e != nil {
				t.Error(e)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			written[key] = blob{b, r.Header.Get("Content-Type"), r.Header.Get("X-Amz-Meta-Sha256")}
			return
		}
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", object.mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(object.data)))
		w.Header().Set("X-Amz-Meta-Sha256", object.hash)
		if r.Method == http.MethodGet {
			_, _ = w.Write(object.data)
		}
	}))
	defer server.Close()
	defaultTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	store := storage.New(platform.StorageConfig{Endpoint: server.URL, Region: "auto", Bucket: "fixture", AccessKeyID: "fixture", SecretAccessKey: "fixture", PublicBaseURL: server.URL + "/public"})
	http.DefaultTransport = defaultTransport
	defer store.Close()
	sites := catalog.New(pool).WithSiteAssets(store)
	site, err := sites.CreateSite(t.Context(), catalog.SiteInput{Editorial: catalog.Editorial{Slug: "d4-assets"}})
	if err != nil {
		t.Fatal(err)
	}
	app := New(platform.NewLogger(io.Discard, "development"), cfg, wallpaper.New(pool, store, true), &Operations{Pool: pool, Catalog: sites, Assets: store, Region: "CN"})
	actor := uuid.NewV7().String()
	path := "/internal/v1/ops/sites/" + site.PublicID
	call := func(method, path, mime string, input []byte, expected int) string {
		t.Helper()
		rid := uuid.NewV7().String()
		req := httptest.NewRequest(method, path, bytes.NewReader(input))
		for k, v := range map[string]string{"Authorization": "Bearer " + testToken, "Content-Type": mime, "X-LC-Actor-ID": actor, "X-LC-Actor-Name": "Fixture Owner", "X-LC-Actor-Role": "owner", "X-Request-ID": rid, "X-LC-Region": "CN", "X-LC-Environment": "development"} {
			req.Header.Set(k, v)
		}
		res, e := app.Test(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != expected {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s %s: got %d, want %d, %s", method, path, res.StatusCode, expected, b)
		}
		return rid
	}
	current := site.Revision
	for _, entry := range []struct{ kind, mime, file string }{
		{"logo", "image/webp", "site-logo.webp"},
		{"cover", "image/avif", "site-cover.avif"},
	} {
		raw, e := os.ReadFile("../storage/testdata/" + entry.file)
		if e != nil {
			t.Fatal(e)
		}
		upload := path + "/assets/" + entry.kind + "?revision=" + strconv.FormatInt(current, 10)
		call("POST", upload, entry.mime, []byte("<svg/>"), 400)
		rid := call("POST", upload, entry.mime, raw, 200)
		call("GET", "/internal/v1/ops/receipts/"+rid, "", nil, 200)
		call("POST", upload, entry.mime, raw, 409)
		current++
	}
	after, err := sites.ManagementSite(t.Context(), site.PublicID)
	if err != nil || after.Revision != current || !strings.HasPrefix(after.LogoURL, server.URL+"/public/sites/logos/") ||
		!strings.HasPrefix(after.CoverURL, server.URL+"/public/sites/covers/") {
		t.Fatalf("verified images were not bound: %+v %v", after, err)
	}
	if !after.HasLogo {
		t.Fatal("bound Logo is not visible in the Management DTO")
	}
	clearBody := func(revision int64) []byte {
		return []byte(`{"revision":` + strconv.FormatInt(revision, 10) + `}`)
	}
	call("DELETE", path+"/logo", "application/json", clearBody(current-1), 409)
	rid := call("DELETE", path+"/logo", "application/json", clearBody(current), 200)
	call("GET", "/internal/v1/ops/receipts/"+rid, "", nil, 200)
	cleared, err := sites.ManagementSite(t.Context(), site.PublicID)
	if err != nil || cleared.HasLogo || cleared.LogoURL != "" || cleared.CoverURL != after.CoverURL || cleared.Revision != current+1 {
		t.Fatal("Logo clear failed to preserve other assets and revision")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(written) != 2 {
		t.Fatal("expected two immutable verified objects")
	}
}
