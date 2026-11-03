package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestBackgroundObjectCreateOnce(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "untrusted-environment")
	hash := strings.Repeat("a", 64)
	key := "start/backgrounds/" + hash + ".webp"
	var object []byte
	puts := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=yaml-key/") {
			t.Error("SDK did not use explicit YAML credentials")
		}
		if r.URL.Path != "/development/"+key {
			t.Error("unexpected object path")
		}
		if r.Method != "PUT" && object == nil {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("x-amz-meta-sha256", hash)
		w.Header().Set("ETag", `"test-etag"`)
		switch r.Method {
		case "PUT":
			puts++
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("missing atomic create-only condition")
			}
			object, _ = io.ReadAll(r.Body)
		case "HEAD":
			w.Header().Set("Content-Length", "4")
		case "GET":
			_, _ = w.Write(object)
		default:
			t.Error("unexpected operation")
		}
	}))
	defer backend.Close()
	store := New(platform.StorageConfig{Endpoint: backend.URL, Region: "auto", Bucket: "development", AccessKeyID: "yaml-key", SecretAccessKey: "yaml-secret", PublicBaseURL: "https://public.example"})
	ctx := context.Background()
	if err := store.PutNew(ctx, key, "image/webp", hash, bytes.NewReader([]byte("data"))); err != nil {
		t.Fatal(err)
	}
	if err := store.PutNew(ctx, key, "image/webp", hash, bytes.NewReader([]byte("other"))); !errors.Is(err, ErrExists) {
		t.Fatalf("overwrite accepted: %v", err)
	}
	if puts != 1 {
		t.Fatal("existing object received another PUT")
	}
	body, metadata, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "data" || metadata.SHA256 != hash || metadata.URL != "https://public.example/"+key {
		t.Fatal("GET metadata/body changed")
	}
	if err := store.PutNew(ctx, "nav/hero/original.webp", "image/webp", hash, bytes.NewReader(nil)); err == nil {
		t.Fatal("write outside new prefix accepted")
	}
	if err := store.PutNew(ctx, key, "image/avif", hash, bytes.NewReader(nil)); err == nil {
		t.Fatal("key/type mismatch accepted")
	}
}

func TestBackgroundConditionalRaceAndSafeErrors(t *testing.T) {
	for _, denied := range []bool{false, true} {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if denied {
				w.WriteHeader(403)
				_, _ = io.WriteString(w, "<Error><Code>AccessDenied</Code><Message>private-signed-request</Message></Error>")
				return
			}
			if r.Method == "HEAD" {
				w.WriteHeader(404)
				return
			}
			w.WriteHeader(412)
			_, _ = io.WriteString(w, "<Error><Code>PreconditionFailed</Code></Error>")
		}))
		store := New(platform.StorageConfig{Endpoint: backend.URL, Region: "auto", Bucket: "development", AccessKeyID: "key", SecretAccessKey: "secret"})
		hash := strings.Repeat("b", 64)
		err := store.PutNew(context.Background(), "start/backgrounds/"+hash+".avif", "image/avif", hash, bytes.NewReader([]byte("data")))
		backend.Close()
		if err == nil || strings.Contains(err.Error(), "private-signed-request") {
			t.Fatalf("unsafe error: %v", err)
		}
		if !denied && !errors.Is(err, ErrExists) {
			t.Fatalf("conditional race not rejected: %v", err)
		}
	}
}
