//go:build integration

package wallpaper

import (
	"context"
	json "encoding/json/v2"
	"flag"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"os"
	"strings"
	"testing"
)

var observationFile = flag.String("c3-observation", "", "Explicit non-secret browser request correlation JSON; read-only check")

func TestC3CrossServiceAudit(t *testing.T) {
	if *observationFile == "" || *testConfig == "" {
		t.Skip("explicit C3 browser observation/config required")
	}
	cfg, e := platform.LoadConfig(*testConfig)
	if e != nil || cfg.Database == nil || !strings.HasPrefix(cfg.Database.Name, "gofurry_c2_c3_") || !strings.HasSuffix(cfg.Database.Name, "_test") {
		t.Fatal("invalid C3 read-only test configuration")
	}
	var observation struct {
		RequestID string `json:"request_id"`
		ID        string `json:"id"`
		Name      string `json:"actor_name"`
		Role      string `json:"actor_role"`
	}
	b, e := os.ReadFile(*observationFile)
	if e != nil || json.Unmarshal(b, &observation) != nil {
		t.Fatal("observation unavailable")
	}
	p, e := database.Open(context.Background(), *cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	var count int
	e = p.QueryRow(context.Background(), "SELECT count(*) FROM wallpaper_audit_logs WHERE request_id=$1 AND wallpaper_id=$2::uuid AND actor_name=$3 AND actor_role=$4 AND action='wallpaper.created'", observation.RequestID, observation.ID, observation.Name, observation.Role).Scan(&count)
	if e != nil || count != 1 {
		t.Fatal("Admin/GoFurry authority audit request correlation failed")
	}
}
