package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCitiesConfigSingleYAMLPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(validYAML+"cities:\n  index_path: data/cities.json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Cities.IndexPath != filepath.Join(dir, "data", "cities.json") {
		t.Fatal("city path is not relative to YAML")
	}
	if err := os.WriteFile(path, []byte(validYAML+"cities:\n  implicit_url: https://example.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("unknown city source accepted")
	}
}
