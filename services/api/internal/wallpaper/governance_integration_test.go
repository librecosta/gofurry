//go:build integration

package wallpaper

import (
	"bytes"
	"context"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type mockEntry struct {
	a Asset
	b []byte
}
type mockObjects struct {
	mu      sync.Mutex
	entries map[string]mockEntry
	base    string
	fail    bool
	missing bool
	puts    int
}

func mockStorage(t *testing.T) *mockObjects {
	m := &mockObjects{entries: map[string]mockEntry{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		entry, ok := m.entries[strings.TrimPrefix(r.URL.Path, "/")]
		missing := m.missing
		m.mu.Unlock()
		if !ok || missing {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", entry.a.ContentType)
		w.Write(entry.b)
	}))
	t.Cleanup(srv.Close)
	m.base = srv.URL
	return m
}
func (m *mockObjects) URL(k string) string { return m.base + "/" + k }
func (m *mockObjects) Head(_ context.Context, k string) (storage.Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return storage.Object{}, ErrUnavailable
	}
	e, ok := m.entries[k]
	if !ok || m.missing {
		return storage.Object{}, storage.ErrNotFound
	}
	return storage.Object{Key: k, SHA256: e.a.SHA256, Bytes: e.a.Bytes, ContentType: e.a.ContentType}, nil
}
func (m *mockObjects) Get(ctx context.Context, k string) (io.ReadCloser, storage.Object, error) {
	o, e := m.Head(ctx, k)
	if e != nil {
		return nil, o, e
	}
	m.mu.Lock()
	b := append([]byte{}, m.entries[k].b...)
	m.mu.Unlock()
	return io.NopCloser(bytes.NewReader(b)), o, nil
}
func (m *mockObjects) PutNew(_ context.Context, k, typ, hash string, r io.ReadSeeker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return ErrUnavailable
	}
	if _, ok := m.entries[k]; ok {
		return storage.ErrExists
	}
	b, e := io.ReadAll(r)
	if e != nil {
		return e
	}
	m.puts++
	m.entries[k] = mockEntry{a: Asset{Key: k, SHA256: hash, ContentType: typ, Bytes: int64(len(b))}, b: b}
	return nil
}
func uploadFixture(t *testing.T, ctx context.Context, s *Service, actor Actor) (Detail, error) {
	t.Helper()
	image, e := os.Open("testdata/image.webp")
	if e != nil {
		t.Fatal(e)
	}
	defer image.Close()
	preview, e := os.Open("testdata/preview.webp")
	if e != nil {
		t.Fatal(e)
	}
	defer preview.Close()
	row, e := s.Upload(ctx, actor, Metadata{Title: "Isolated test wallpaper"}, image, preview, "image/webp", "image/webp")
	return s.detail(row), e
}
func TestPostgresConstraintsRevisionAndAudit(t *testing.T) {
	ctx, s, cfg := integration(t, false)
	m := mockStorage(t)
	s.objects = m
	actor := testActor()
	row, e := uploadFixture(t, ctx, s, actor)
	if e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{
		"UPDATE start_wallpapers SET id='00000000-0000-4000-8000-000000000001'", "UPDATE start_wallpapers SET status='deleted'", "UPDATE start_wallpapers SET title=' '", "UPDATE start_wallpapers SET image_key='../secret'", "UPDATE start_wallpapers SET image_sha256=upper(image_sha256)", "UPDATE start_wallpapers SET image_type='image/png'", "UPDATE start_wallpapers SET image_bytes=0", "UPDATE start_wallpapers SET preview_key='start/backgrounds/invalid.webp'", "UPDATE start_wallpapers SET width=20000", "UPDATE start_wallpapers SET preview_height=0", "UPDATE start_wallpapers SET revision=0", "UPDATE start_wallpapers SET status='published',published_at=NULL",
	} {
		if _, e = s.pool.Exec(ctx, query); e == nil {
			t.Fatal("constraint accepted invalid row")
		}
	}
	secondPool, e := database.Open(ctx, *cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer secondPool.Close()
	second := New(secondPool, m, true)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, service := range []*Service{s, second} {
		go func(service *Service) {
			<-start
			_, e := service.Edit(ctx, actor, row.ID, 1, Metadata{Title: "Concurrent title"})
			results <- e
		}(service)
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		e := <-results
		if e == nil {
			success++
		} else if e == ErrConflict {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("revision race overwrote content")
	}
	if _, e = s.pool.Exec(ctx, "ALTER TABLE wallpaper_audit_logs ADD CONSTRAINT test_audit_failure CHECK(action<>'wallpaper.edited') NOT VALID"); e != nil {
		t.Fatal(e)
	}
	_, editErr := s.Edit(ctx, actor, row.ID, 2, Metadata{Title: "Must roll back"})
	_, e = s.pool.Exec(ctx, "ALTER TABLE wallpaper_audit_logs DROP CONSTRAINT test_audit_failure")
	if e != nil || editErr == nil {
		t.Fatal("audit failure accepted")
	}
	current, e := s.Detail(ctx, row.ID)
	if e != nil || current.Revision != 2 || current.Title != "Concurrent title" {
		t.Fatal("business committed without audit")
	}
	m.missing = true
	if _, e = s.Transition(ctx, actor, row.ID, 2, "published", true); e != ErrUnavailable {
		t.Fatal("published missing object")
	}
	m.missing = false
	if _, e = s.Transition(ctx, actor, row.ID, 2, "archived", false); e != ErrConflict {
		t.Fatal("draft archived")
	}
	if _, e = s.Transition(ctx, actor, row.ID, 2, "published", true); e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, actor, row.ID, 3); e != ErrConflict {
		t.Fatal("published deleted")
	}
	if catalog, e := s.Catalog(ctx); e != nil || len(catalog) != 1 {
		t.Fatal(e)
	}
	var snapshot, id string
	if e = s.pool.QueryRow(ctx, "SELECT actor_name,request_id FROM wallpaper_audit_logs WHERE action='wallpaper.created'").Scan(&snapshot, &id); e != nil || snapshot != actor.Name || id != actor.RequestID {
		t.Fatal("audit snapshot/request correlation failed")
	}
	var text string
	if e = s.pool.QueryRow(ctx, "SELECT string_agg(before_data::text||after_data::text,'') FROM wallpaper_audit_logs").Scan(&text); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(text, cfg.Management.Token) || strings.Contains(text, "Authorization") {
		t.Fatal("secret in audit")
	}
	s.pool.Close()
	if _, e = s.Catalog(ctx); e != ErrUnavailable {
		t.Fatal("cached catalog masked database outage")
	}
	t.Log("POSTGRESQL INTEGRATION + MOCK STORAGE: constraints, two-connection revision conflict, audit rollback, missing-object publication, snapshot, outage passed")
}
func TestStorageAndDatabaseFailures(t *testing.T) {
	ctx, s, _ := integration(t, false)
	m := mockStorage(t)
	s.objects = m
	actor := testActor()
	m.fail = true
	if _, e := uploadFixture(t, ctx, s, actor); e != ErrUnavailable {
		t.Fatal("storage failure accepted")
	}
	m.fail = false
	if _, e := s.pool.Exec(ctx, "ALTER TABLE wallpaper_audit_logs ADD CONSTRAINT test_create_failure CHECK(action<>'wallpaper.created') NOT VALID"); e != nil {
		t.Fatal(e)
	}
	_, createErr := uploadFixture(t, ctx, s, actor)
	_, e := s.pool.Exec(ctx, "ALTER TABLE wallpaper_audit_logs DROP CONSTRAINT test_create_failure")
	if e != nil || createErr == nil {
		t.Fatal("database failure accepted")
	}
	var count int
	s.pool.QueryRow(ctx, "SELECT count(*) FROM start_wallpapers").Scan(&count)
	if count != 0 || len(m.entries) != 2 {
		t.Fatal("failed DB write created a fake record or removed reusable objects")
	}
	puts := m.puts
	if _, e = uploadFixture(t, ctx, s, actor); e != nil || m.puts != puts {
		t.Fatal("retry did not reuse immutable objects", e)
	}
}
