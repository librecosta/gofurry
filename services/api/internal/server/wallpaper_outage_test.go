package server

import (
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/rs/zerolog"
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOptionalDatabaseOutagePreservesPublicAPIs(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.Database = &platform.DatabaseConfig{Host: "127.0.0.1", Port: uint16(port), Name: "unavailable_test", User: "unused", SSLMode: "disable", ConnectTimeout: time.Second, MaxConnections: 1, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute}
	s := New(zerolog.Nop(), cfg)
	defer s.close()
	if s.initErr != nil {
		t.Fatal("optional database outage prevents startup")
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/api/v1/start/backgrounds", 503}, {"/api/v1/status", 200}, {"/api/v1/start/weather?lang=en", 200}, {"/api/v1/start/location", 200}} {
		res, e := s.App.Test(httptest.NewRequest("GET", tc.path, nil))
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("%s: %d", tc.path, res.StatusCode)
		}
	}
}
