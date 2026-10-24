package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/rs/zerolog"
)

func TestHTTPRuntime(t *testing.T) {
	var logs bytes.Buffer
	s := New(platform.NewLogger(&logs, "production"))
	s.App.Post("/test/json", func(c fiber.Ctx) error {
		var body map[string]any
		if err := c.Bind().JSON(&body); err != nil {
			return fiber.ErrBadRequest
		}
		return c.JSON(body)
	})
	s.App.Get("/test/panic", func(fiber.Ctx) error { panic("secret-panic") })
	s.App.Get("/test/error", func(fiber.Ctx) error { return errors.New("secret-error") })
	// Verify the configured functions, then exercise both adapters through HTTP.
	encoder := s.App.Config().JSONEncoder
	decoder := s.App.Config().JSONDecoder
	if encoder == nil || decoder == nil {
		t.Fatal("JSON adapters missing")
	}
	if reflect.ValueOf(encoder).Pointer() != reflect.ValueOf(sonic.Marshal).Pointer() || reflect.ValueOf(decoder).Pointer() != reflect.ValueOf(sonic.Unmarshal).Pointer() {
		t.Fatal("expected Sonic adapters")
	}
	encoded, err := encoder(map[string]string{"message": "你好"})
	if err != nil || !sonic.Valid(encoded) {
		t.Fatalf("encoder: %s %v", encoded, err)
	}
	var decoded map[string]string
	if err := decoder(encoded, &decoded); err != nil || decoded["message"] != "你好" {
		t.Fatalf("decoder: %v", err)
	}

	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/livez", "", 200},
		{"HEAD", "/livez", "", 200},
		{"GET", "/readyz", "", 503},
		{"GET", "/api/v1/status", "", 200},
		{"POST", "/test/json", `{"message":"你好","number":42}`, 200},
		{"POST", "/test/json", `{invalid`, 400},
		{"GET", "/missing?token=secret-query", "", 404},
		{"GET", "/test/panic", "", 500},
		{"GET", "/test/error", "", 500},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "runtime-test")
			req.Header.Set("Authorization", "Bearer secret-token")
			req.Header.Set("Cookie", "session=secret-cookie")
			res, err := s.App.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != tc.status {
				t.Fatalf("status %d: %s", res.StatusCode, body)
			}
			if res.Header.Get("X-Request-ID") != "runtime-test" {
				t.Fatal("missing request id")
			}
			if res.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing security headers")
			}
			if tc.method != "HEAD" && !json.Valid(body) {
				t.Fatalf("not JSON: %s", body)
			}
			if tc.status >= 400 && tc.path != "/readyz" {
				var envelope struct {
					RequestID string `json:"request_id"`
					Error     struct {
						Code    int
						Message string
					}
				}
				if err := json.Unmarshal(body, &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.RequestID != "runtime-test" || envelope.Error.Code != tc.status {
					t.Fatalf("bad error: %s", body)
				}
				if bytes.Contains(body, []byte("secret")) {
					t.Fatalf("error leaked detail: %s", body)
				}
			}
			if tc.path == "/test/json" && tc.status == 200 && !bytes.Contains(body, []byte("你好")) {
				t.Fatalf("round trip failed: %s", body)
			}
		})
	}
	if strings.Contains(logs.String(), "secret") {
		t.Fatalf("credential leaked: %s", logs.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["request_id"] != "runtime-test" || entry["service"] != "gofurry-api" {
			t.Fatalf("uncorrelated log: %s", line)
		}
	}
}

func TestGracefulDrain(t *testing.T) {
	s := New(zerolog.Nop())
	address := make(chan string, 1)
	s.App.Hooks().OnListen(func(data fiber.ListenData) error { address <- "http://127.0.0.1:" + data.Port; return nil })
	entered, release := make(chan struct{}), make(chan struct{})
	s.App.Get("/test/slow", func(c fiber.Ctx) error { close(entered); <-release; return c.SendString("completed") })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- s.Listen(ctx, "127.0.0.1:0") }()
	var base string
	select {
	case base = <-address:
	case err := <-stopped:
		t.Fatalf("listen failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("listen timeout")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("not ready after listen")
	}
	finished := make(chan error, 1)
	go func() {
		res, err := client.Get(base + "/test/slow")
		if err != nil {
			finished <- err
			return
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err == nil && string(body) != "completed" {
			err = fmt.Errorf("truncated body: %s", body)
		}
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request timeout")
	}
	cancel()
	deadline := time.After(3 * time.Second)
	for s.ready.Load() {
		select {
		case <-deadline:
			t.Fatal("readiness did not drop")
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case err := <-stopped:
		t.Fatalf("exited before drain: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("drain timeout")
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}
