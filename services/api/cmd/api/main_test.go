package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIRequiresConfig(t *testing.T) {
	for _, args := range [][]string{nil, {"healthcheck"}, {"--config", "missing.yaml"}, {"--config", ""}} {
		cmd := newCommand()
		cmd.SetArgs(args)
		cmd.SetOut(io.Discard)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestCLIYAMLStartAndProbe(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "server:\n  address: " + address + "\n  environment: production\ndeployment:\n  region: INTL\nlogging:\n  slow_request: 1s\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := newCommand()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--config", path})
	stopped := make(chan error, 1)
	go func() { stopped <- cmd.ExecuteContext(ctx) }()
	client := http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get("http://" + address + "/readyz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("configured listener did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	probe := newCommand()
	response, err := client.Get("http://" + address + "/api/v1/status?region=CN&locale=zh")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.Header.Get("X-GoFurry-Region") != "INTL" || response.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
		t.Fatal("request changed deployment policy")
	}
	probe.SetArgs([]string{"healthcheck", "--config", path})
	if err := probe.Execute(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}
