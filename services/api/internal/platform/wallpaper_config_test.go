package platform

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWallpaperRuntimeConfig(t *testing.T) {
	b := make([]byte, 32)
	rand.Read(b)
	valid := func() Config {
		var c Config
		c.Server.Address = "127.0.0.1:17321"
		c.Database = &DatabaseConfig{Host: "127.0.0.1", Port: 5432, Name: "gofurry_test", User: "gofurry", SSLMode: "disable", ConnectTimeout: time.Second, MaxConnections: 5, MaxConnectionLifetime: time.Hour, MaxConnectionIdleTime: time.Minute}
		c.Storage = &StorageConfig{}
		c.Management = &ManagementConfig{Address: "127.0.0.1:17324", Token: base64.RawURLEncoding.EncodeToString(b)}
		return c
	}
	if e := valid().ValidateWallpaperRuntime(); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Config){func(c *Config) { c.Management.Address = "0.0.0.0:17324" }, func(c *Config) { c.Management.Address = "localhost:17324" }, func(c *Config) { c.Management.Token = "weak" }, func(c *Config) { c.Database = nil }, func(c *Config) { c.Storage = nil }, func(c *Config) { c.Database.MaxConnections = 1000 }, func(c *Config) { c.Database.ConnectTimeout = time.Hour }} {
		c := valid()
		change(&c)
		if c.ValidateWallpaperRuntime() == nil {
			t.Fatal("accepted unsafe management configuration")
		}
	}
	var off Config
	if off.ValidateWallpaperRuntime() != nil {
		t.Fatal("optional dependency required")
	}
}
func TestWallpaperYAMLNumericBounds(t *testing.T) {
	const database = "database:\n  host: 127.0.0.1\n  port: 5432\n  name: gofurry_test\n  user: test\n  password: ''\n  ssl_mode: disable\n  connect_timeout: 2s\n  max_connections: 5\n  min_connections: 0\n  max_connection_lifetime: 30m\n  max_connection_idle_time: 5m\n"
	file := filepath.Join(t.TempDir(), "config.yaml")
	for _, tc := range []struct {
		data  string
		valid bool
	}{{database, true}, {strings.Replace(database, "port: 5432", "port: 65537", 1), false}, {strings.Replace(database, "port: 5432", "port: '5432'", 1), false}, {strings.Replace(database, "max_connections: 5", "max_connections: 4294967301", 1), false}, {strings.Replace(database, "connect_timeout: 2s", "connect_timeout: 2", 1), false}} {
		if e := os.WriteFile(file, []byte(validYAML+tc.data), 0600); e != nil {
			t.Fatal(e)
		}
		_, e := LoadConfig(file)
		if (e == nil) != tc.valid {
			t.Fatal("invalid YAML type or overflow handling", e)
		}
	}
}
