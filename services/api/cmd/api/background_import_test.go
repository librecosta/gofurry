package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackgroundImportGuards(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "invalid.webp")
	if err := os.WriteFile(file, []byte("this-is-not-a-webp"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"development", "production"} {
		path := filepath.Join(dir, "config.yaml")
		yaml := "server:\n  address: 127.0.0.1:17321\n  environment: " + environment + "\ndeployment:\n  region: CN\nlogging:\n  slow_request: 1s\nstorage:\n  endpoint: https://storage.invalid\n  region: auto\n  bucket: development\n  access_key_id: test\n  secret_access_key: test\n  public_base_url: https://public.invalid\n"
		if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := newCommand()
		cmd.SetOut(io.Discard)
		cmd.SetArgs([]string{"--config", path, "background-import", file})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("invalid import accepted")
		}
		if environment == "production" && !strings.Contains(err.Error(), "requires development YAML") {
			t.Fatal(err)
		}
		if environment == "development" && !strings.Contains(err.Error(), "must contain AVIF or WebP") {
			t.Fatal(err)
		}
	}
}
