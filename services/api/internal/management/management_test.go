package management

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"
)

func TestBearerAndActorBoundary(t *testing.T) {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	var cfg platform.Config
	cfg.Management = &platform.ManagementConfig{Token: token}
	cfg.Deployment.Region = "CN"
	cfg.Server.Environment = "development"
	var logs bytes.Buffer
	app := New(platform.NewLogger(&logs, "development"), cfg, wallpaper.New(nil, nil, true))
	cases := []struct {
		name, auth, field, value string
		status                   int
	}{
		{"missing", "", "", "", 401}, {"wrong", "Bearer incorrect", "", "", 401}, {"query only", "", "", "", 401},
		{"valid", "Bearer " + token, "", "", 503},
		{"region", "Bearer " + token, "X-LC-Region", "INTL", 403}, {"environment", "Bearer " + token, "X-LC-Environment", "production", 403},
		{"actor", "Bearer " + token, "X-LC-Actor-ID", "invalid", 400}, {"role", "Bearer " + token, "X-LC-Actor-Role", "root", 400}, {"id", "Bearer " + token, "X-Request-ID", "invalid", 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/internal/v1/readyz?token="+token, nil)
			req.Header.Set("Authorization", tc.auth)
			req.Header.Set("Cookie", "admin_session=secret-cookie")
			req.Header.Set("X-LC-Actor-ID", uuid.NewV7().String())
			req.Header.Set("X-LC-Actor-Name", "Local Operator")
			req.Header.Set("X-LC-Actor-Role", "operator")
			req.Header.Set("X-Request-ID", uuid.NewV7().String())
			req.Header.Set("X-LC-Region", "CN")
			req.Header.Set("X-LC-Environment", "development")
			if tc.field != "" {
				req.Header.Set(tc.field, tc.value)
			}
			res, e := app.Test(req)
			if e != nil {
				t.Fatal(e)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("%d: %s", res.StatusCode, body)
			}
			if res.Header.Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("management enables CORS")
			}
		})
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), "secret-cookie") {
		t.Fatal("credential leaked")
	}
}
