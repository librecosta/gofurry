package platform

import (
	"crypto/rand"
	"encoding/base64"
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
