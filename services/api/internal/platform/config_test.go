package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = "server:\n  address: 127.0.0.1:17321\n  environment: development\ndeployment:\n  region: CN\nlogging:\n  slow_request: 1s\n"

func TestConfigSingleSource(t *testing.T) {
	t.Setenv("API_ADDR", "bad-env-address")
	t.Setenv("APP_ENV", "bad-env-mode")
	path := filepath.Join(t.TempDir(), "explicit.yaml")
	for _, tc := range []struct {
		name, data string
		valid      bool
	}{
		{"CN", validYAML, true},
		{"INTL", strings.ReplaceAll(validYAML, "region: CN", "region: INTL"), true},
		{"invalid region", strings.ReplaceAll(validYAML, "region: CN", "region: other"), false},
		{"invalid environment", strings.ReplaceAll(validYAML, "environment: development", "environment: other"), false},
		{"missing region", strings.ReplaceAll(validYAML, "  region: CN", ""), false},
		{"unknown option", validYAML + "nsfw: true\n", false},
		{"unknown nested option", strings.ReplaceAll(validYAML, "region: CN", "region: CN\n  nsfw: true"), false},
		{"invalid port", strings.ReplaceAll(validYAML, "17321", "70000"), false},
		{"invalid duration", strings.ReplaceAll(validYAML, "1s", "0s"), false},
		{"duplicate", validYAML + "server: {}\n", false},
		{"invalid YAML", "server: [", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			if tc.valid && cfg.ProbeURL() != "http://127.0.0.1:17321/readyz" {
				t.Fatal(cfg.ProbeURL())
			}
		})
	}
	if _, err := LoadConfig(""); err == nil {
		t.Fatal("missing explicit path accepted")
	}
	if _, err := LoadConfig(path + "-absent"); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestProbeUsesConfiguredPort(t *testing.T) {
	for _, address := range []string{"0.0.0.0:17321", "[::]:17321"} {
		var cfg Config
		cfg.Server.Address = address
		if !strings.HasSuffix(cfg.ProbeURL(), ":17321/readyz") || strings.Contains(cfg.ProbeURL(), "0.0.0.0") {
			t.Fatal(cfg.ProbeURL())
		}
	}
}
