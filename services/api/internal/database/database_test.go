package database

import (
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestCNDatabaseTransport(t *testing.T) {
	cfg, err := platform.LoadConfig("../../../../deploy/cn/gofurry.example.yaml")
	if err != nil || cfg.Database == nil {
		t.Fatal("production configuration failed to load", err)
	}
	for _, mode := range []string{"disable", "verify-full"} {
		t.Run(mode, func(t *testing.T) {
			db := *cfg.Database
			db.SSLMode = mode
			pool, err := PoolConfig(db)
			if err != nil {
				t.Fatal(err)
			}
			conn := pool.ConnConfig
			if conn.Host != "librecosta-postgres" || conn.Port != 5432 ||
				conn.User != "gofurry_cn_runtime" || conn.Password != db.Password ||
				conn.ConnectTimeout != db.ConnectTimeout || pool.MaxConns != db.MaxConnections || len(conn.Fallbacks) != 0 {
				t.Fatal("connection endpoint, credentials, bounds or fallback changed")
			}
			if mode == "disable" && conn.TLSConfig != nil ||
				mode == "verify-full" && (conn.TLSConfig == nil || conn.TLSConfig.ServerName != conn.Host || conn.TLSConfig.InsecureSkipVerify) {
				t.Fatal("configured PostgreSQL transport changed")
			}
		})
	}
}
