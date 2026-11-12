package storage

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestSiteAssetImport(t *testing.T) {
	ctx := t.Context()
	logo, err := os.ReadFile("testdata/site-logo.webp")
	if err != nil {
		t.Fatal(err)
	}
	cover, err := os.ReadFile("testdata/site-cover.avif")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	type stored struct {
		data       []byte
		mime, hash string
	}
	objects := map[string]stored{}
	uploads := 0
	mode := ""
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/objects/"), "/cdn/")
		if !SiteLogo.ValidKey(key) && !SiteCover.ValidKey(key) {
			t.Error("object request escaped Site key namespace")
			w.WriteHeader(400)
			return
		}
		object, ok := objects[key]
		if r.Method == "PUT" {
			if r.Header.Get("If-None-Match") != "*" || !strings.Contains(r.Header.Get("Cache-Control"), "immutable") {
				t.Error("missing immutable upload headers")
			}
			if ok {
				w.WriteHeader(412)
				_, _ = io.WriteString(w, "<Error><Code>PreconditionFailed</Code></Error>")
				return
			}
			raw, _ := io.ReadAll(r.Body)
			objects[key] = stored{raw, r.Header.Get("Content-Type"), r.Header.Get("X-Amz-Meta-Sha256")}
			uploads++
			return
		}
		if !ok {
			w.WriteHeader(404)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/cdn/") {
			if r.Header.Get("Authorization") != "" {
				t.Error("public delivery received private authorization")
			}
			if mode == "redirect" {
				http.Redirect(w, r, "https://untrusted.invalid/site", 302)
				return
			}
			if mode == "corrupt" {
				object.data = append([]byte(nil), object.data...)
				object.data[len(object.data)-1] ^= 1
			}
		}
		w.Header().Set("Content-Type", object.mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(object.data)))
		w.Header().Set("X-Amz-Meta-Sha256", object.hash)
		if r.Method == "GET" {
			_, _ = w.Write(object.data)
		}
	}))
	defer backend.Close()
	store := New(platform.StorageConfig{Endpoint: backend.URL, Region: "auto", Bucket: "objects", AccessKeyID: "fixture", SecretAccessKey: "fixture", PublicBaseURL: backend.URL + "/cdn"})
	store.transport.TLSClientConfig = backend.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	defer store.Close()
	for _, fixture := range []struct {
		kind          SiteAssetKind
		raw           []byte
		width, height int
	}{
		{SiteLogo, logo, 160, 160}, {SiteCover, cover, 960, 540},
	} {
		var preflight *Store
		expected, err := preflight.ImportSiteAsset(ctx, fixture.kind, bytes.NewReader(fixture.raw), true)
		if err != nil || expected.Width != fixture.width || expected.Height != fixture.height || expected.URL != "" {
			t.Fatalf("preflight: %+v %v", expected, err)
		}
		for range 2 {
			got, err := store.ImportSiteAsset(ctx, fixture.kind, bytes.NewReader(fixture.raw), false)
			if err != nil || got.Key != expected.Key || got.Bytes != expected.Bytes || got.URL != backend.URL+"/cdn/"+got.Key {
				t.Fatalf("import: %+v %v", got, err)
			}
		}
		if store.URL(expected.Key) != "" {
			t.Fatal("background URL accepted a Site asset")
		}
		if err := store.PutNew(ctx, expected.Key, expected.ContentType, expected.SHA256, bytes.NewReader(fixture.raw)); err == nil {
			t.Fatal("background writer accepted a Site asset")
		}
	}
	mu.Lock()
	if uploads != 2 {
		t.Error("immutable replay wrote a duplicate object")
	}
	mode = "redirect"
	mu.Unlock()
	expected, _ := inspectSiteAsset(SiteLogo, logo)
	if _, err = store.VerifySiteAsset(ctx, SiteLogo, expected.Key); !errors.Is(err, ErrSiteAsset) {
		t.Fatalf("public redirect accepted: %v", err)
	}
	mu.Lock()
	mode = "corrupt"
	mu.Unlock()
	if _, err = store.VerifySiteAsset(ctx, SiteLogo, expected.Key); !errors.Is(err, ErrSiteAsset) {
		t.Fatalf("public mismatch accepted: %v", err)
	}
	for _, base := range []string{"http://cdn.example", "https://u:p@cdn.example", "https://cdn.example/?x", "https://cdn.example/#x", "https://cdn.example/%2e%2e/cdn"} {
		s := Store{publicBase: base}
		if s.SiteAssetURL(SiteLogo, expected.Key) != "" {
			t.Fatalf("accepted bad asset origin %s", base)
		}
	}
	if store.SiteAssetURL(SiteCover, expected.Key) != "" || store.SiteAssetURL(SiteLogo, "../"+expected.Key) != "" {
		t.Fatal("accepted wrong kind or path")
	}
}

func TestSiteAssetRejectsUnapprovedRaster(t *testing.T) {
	logo, _ := os.ReadFile("testdata/site-logo.webp")
	cover, _ := os.ReadFile("testdata/site-cover.avif")
	wideWebP, _ := os.ReadFile("../wallpaper/testdata/preview.webp")
	nonWideAVIF, _ := os.ReadFile("../wallpaper/testdata/nonwide.avif")
	for _, fixture := range []struct {
		kind SiteAssetKind
		raw  []byte
	}{
		{SiteLogo, []byte("<svg><script>alert(1)</script></svg>")},
		{SiteCover, []byte("<html>image</html>")},
		{SiteLogo, cover}, {SiteCover, logo}, {SiteLogo, wideWebP}, {SiteCover, nonWideAVIF},
		{SiteLogo, logo[:len(logo)/2]}, {SiteCover, cover[:len(cover)/2]},
		{SiteLogo, append(append([]byte(nil), logo...), []byte("<svg/>")...)},
		{SiteLogo, make([]byte, 1<<20+1)}, {SiteCover, make([]byte, 4<<20+1)},
	} {
		if _, err := inspectSiteAsset(fixture.kind, fixture.raw); !errors.Is(err, ErrSiteAsset) {
			t.Fatalf("accepted invalid %s asset: %v", fixture.kind, err)
		}
	}
}
