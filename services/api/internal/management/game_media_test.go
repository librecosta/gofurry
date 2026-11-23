package management

import (
	"encoding/base64"
	json "encoding/json/v2"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
)

const mediaFixtureGame = "cajktrpb3s5gmkr1"
const mediaOps = "/internal/v1/ops/"

var mediaCursorConfig = flag.String("media-cursor-config", "", "GoFurry R1-C-F1 loopback fixture YAML with 25 candidate rows")

func mediaCursorRequest(t *testing.T, app *fiber.App, token, path string, status int, out any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, mediaOps+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-LC-Actor-ID", uuid.NewV7().String())
		req.Header.Set("X-LC-Actor-Name", "Native media tester")
		req.Header.Set("X-LC-Actor-Role", "operator")
		req.Header.Set("X-LC-Region", "CN")
		req.Header.Set("X-Request-ID", uuid.NewV7().String())
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		t.Fatalf("GET %s returned %d, want %d", path, res.StatusCode, status)
	}
	if out != nil {
		if err := json.UnmarshalRead(res.Body, out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMediaCandidateQueryBoundaries(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	cfg := platform.Config{Management: &platform.ManagementConfig{Token: token}}
	cfg.Deployment.Region = "CN"
	app := New(platform.NewLogger(io.Discard), cfg, wallpaper.New(nil, nil, true),
		&Operations{Catalog: catalog.New(nil), Region: "CN"})
	path := "games/" + mediaFixtureGame + "/media-candidates"
	const cursor = "019abcde-f0ab-7000-8000-0123456789ab"
	for _, query := range []string{
		"?after=" + mediaFixtureGame, "?after=" + uuid.NewV4().String(),
		"?after=" + strings.ToUpper(cursor), "?after=" + strings.ReplaceAll(cursor, "-", ""),
		"?after=invalid", "?after=", "?after=%ZZ", "?after=" + strings.Repeat("0", 1024),
		"?after=" + cursor + "&after=" + cursor, "?cursor=" + cursor, "?after=" + cursor + "&limit=12",
	} {
		mediaCursorRequest(t, app, token, path+query, http.StatusBadRequest, nil)
	}
	mediaCursorRequest(t, app, token, "games/invalid/media-candidates", http.StatusBadRequest, nil)
	mediaCursorRequest(t, app, "", path+"?after="+cursor, http.StatusUnauthorized, nil)
	for _, directory := range []string{"sites", "games", "games/collections"} {
		mediaCursorRequest(t, app, token, directory+"?after="+cursor, http.StatusBadRequest, nil)
	}
}

func TestMediaCandidatePaginationPostgres(t *testing.T) {
	if *mediaCursorConfig == "" {
		t.Skip("pass -media-cursor-config for the isolated media fixture")
	}
	cfg, err := platform.LoadConfig(*mediaCursorConfig)
	if err != nil || cfg.Database == nil || cfg.Database.Host != "127.0.0.1" || cfg.Database.Name != "gofurry_r1c_f1_test" || cfg.Management == nil {
		t.Fatal("expected the isolated loopback gofurry_r1c_f1_test configuration")
	}
	ctx := t.Context()
	pool, err := database.Open(ctx, *cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	app := New(platform.NewLogger(io.Discard), cfg, wallpaper.New(pool, nil, true),
		&Operations{Pool: pool, Catalog: catalog.New(pool), Region: "CN"})

	var expected []uuid.UUID
	var anchors []uuid.UUID
	err = pool.QueryRow(ctx, `SELECT array_agg(m.observation_id ORDER BY m.id), array_agg(m.id ORDER BY m.id)
FROM game_source_media m JOIN game_sources s ON s.id=m.game_source_id JOIN games g ON g.id=s.game_id
WHERE g.public_id=$1 AND m.state IN ('present','confirmed_empty')`, mediaFixtureGame).Scan(&expected, &anchors)
	if err != nil || len(expected) != 25 {
		t.Fatalf("expected a 25-row fixture, got %d: %v", len(expected), err)
	}

	path := "games/" + mediaFixtureGame + "/media-candidates"
	var got []uuid.UUID
	after := ""
	for i, size := range []int{12, 12, 1} {
		var page catalog.GameMediaCandidatePage
		mediaCursorRequest(t, app, cfg.Management.Token, path+after, http.StatusOK, &page)
		if len(page.Items) != size {
			t.Fatalf("page %d has %d rows, want %d", i+1, len(page.Items), size)
		}
		for _, candidate := range page.Items {
			if candidate.Media != nil && candidate.Media.AppID == 1332010 ||
				candidate.Assets != nil && candidate.Assets.AppID == 1332010 {
				got = append(got, candidate.ObservationID)
			} else {
				t.Fatal("candidate belongs to another game or lost its payload")
			}
		}
		if i == 2 {
			if page.Next != nil {
				t.Fatal("last page has a continuation")
			}
		} else {
			if page.Next == nil || *page.Next != anchors[len(got)-1] {
				t.Fatalf("page %d has a different keyset boundary", i+1)
			}
			after = "?after=" + page.Next.String()
		}
	}
	if !slices.Equal(got, expected) {
		t.Fatal("pagination changed candidate order or returned duplicates or gaps")
	}
	for _, directory := range []struct {
		path, cursor, nextID string
	}{
		{"sites", "0000000000000001", "0000000000000002"},
		{"games", mediaFixtureGame, "de7mqra5npxtx39h"},
		{"games/collections", "0000000000000001", "0000000000000002"},
	} {
		var page struct {
			Items []struct {
				PublicID string `json:"public_id"`
			} `json:"items"`
		}
		mediaCursorRequest(t, app, cfg.Management.Token, directory.path+"?after="+directory.cursor, http.StatusOK, &page)
		if len(page.Items) != 1 || page.Items[0].PublicID != directory.nextID {
			t.Fatalf("%s did not continue after its Public ID", directory.path)
		}
	}
	var other catalog.GameMediaCandidatePage
	mediaCursorRequest(t, app, cfg.Management.Token, "games/de7mqra5npxtx39h/media-candidates?after="+anchors[11].String(), http.StatusOK, &other)
	for _, candidate := range other.Items {
		if slices.Contains(got, candidate.ObservationID) {
			t.Fatal("another game returned a candidate from the first game")
		}
	}
}
