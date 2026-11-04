package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQWeatherConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	const good = "qweather:\n  enabled: true\n  api_host: test.xy.qweatherapi.com\n  developer_id: testDeveloper\n  project_id: testProject\n  credential_id: testCredential\n  private_key_path: data/private/test.pem\n  proxy_url: ''\n"
	for _, tc := range []struct {
		name, data string
		valid      bool
	}{
		{"disabled", "qweather:\n  enabled: false\n", true},
		{"missing degrades", "qweather:\n  enabled: true\n", true},
		{"valid", good, true},
		{"host URL", strings.Replace(good, "test.xy.qweatherapi.com", "https://test.xy.qweatherapi.com", 1), false},
		{"foreign host", strings.Replace(good, "test.xy.qweatherapi.com", "test.example", 1), false},
		{"host injection", strings.Replace(good, "test.xy.qweatherapi.com", "test.qweatherapi.com/path", 1), false},
		{"credential newline", strings.Replace(good, "testDeveloper", `"test\nDeveloper"`, 1), false},
		{"proxy path", strings.Replace(good, "proxy_url: ''", "proxy_url: http://127.0.0.1:9/path", 1), false},
		{"unknown", good + "  unknown: true\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(validYAML+tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.name == "valid" {
				if cfg.QWeather.PrivateKeyPath != filepath.Join(filepath.Dir(path), "data", "private", "test.pem") {
					t.Fatal("key path not relative to YAML")
				}
				data, _ := MarshalJSON(cfg.QWeather)
				for _, value := range []string{"testDeveloper", "testProject", "testCredential", "test.xy.qweatherapi.com", "test.pem"} {
					if strings.Contains(string(data), value) {
						t.Fatal("private configuration serialized")
					}
				}
			}
			if tc.name == "missing degrades" && len(cfg.QWeather.MissingFields()) != 5 {
				t.Fatal("missing field diagnostic incomplete")
			}
		})
	}
}
