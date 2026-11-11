package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHarvestExplicitYAML(t *testing.T) {
	const db = `database:
  host: 127.0.0.1
  port: 5432
  name: fixture
  user: fixture
  ssl_mode: disable
  connect_timeout: 1s
  max_connections: 2
  min_connections: 0
  max_connection_lifetime: 30m
  max_connection_idle_time: 5m
`
	const h = `harvest:
  url: https://harvest.example
  namespace: gofurry_cn
  consume: true
  consumer_id: 0193c812-c600-7000-8000-000000000001
  reader_token: fixture_reader_012345678901234567890
`
	tests := []struct {
		name, yaml string
		valid      bool
		enabled    bool
	}{
		{"default disabled", validYAML + db, true, false},
		{"explicit reader", validYAML + db + h, true, true},
		{"development loopback", validYAML + db + strings.Replace(h, "https://harvest.example", "http://127.0.0.1:3210", 1), true, true},
		{"remote HTTP", validYAML + db + strings.Replace(h, "https://", "http://", 1), false, false},
		{"URL credentials", validYAML + db + strings.Replace(h, "https://", "https://user:password@", 1), false, false},
		{"distinct roles", validYAML + db + h + "  writer_token: fixture_reader_012345678901234567890\n", false, false},
		{"unknown key", validYAML + db + h + "  operator_token: ignored\n", false, false},
		{"INTL isolation", strings.Replace(validYAML, "region: CN", "region: INTL", 1) + db + h, false, false},
		{"no regional DB", validYAML + h, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "api.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(path)
			if (err == nil) != tc.valid {
				t.Fatal("unexpected validity")
			}
			if !tc.valid {
				return
			}
			if (cfg.Harvest != nil) != tc.enabled {
				t.Fatal("unexpected enabled state")
			}
			if cfg.Harvest == nil {
				return
			}
			if cfg.Harvest.PollInterval != 5*time.Second || cfg.Harvest.PageSize != 32 {
				t.Fatal("unexpected poll bounds")
			}
			raw, err := MarshalJSON(cfg.Harvest)
			if err != nil || strings.Contains(string(raw), cfg.Harvest.ReaderToken) {
				t.Fatal("reader credential must stay private")
			}
		})
	}
}
