//go:build integration

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"
)

var wallpaperConfig = flag.String("wallpaper-runtime-config", "", "Explicit isolated GoFurry-owned test YAML")
var devBucket = flag.String("development-bucket", "", "Confirmed development bucket")
var devPublic = flag.String("development-public-base", "", "Confirmed development public base")

type lockedLogs struct {
	mu sync.Mutex
	b  bytes.Buffer
}
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func (l *lockedLogs) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(b)
}
func (l *lockedLogs) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }
func TestManagementRealDevelopmentHTTP(t *testing.T) {
	if *wallpaperConfig == "" {
		t.Skip("explicit isolated config required")
	}
	cfg, e := platform.LoadConfig(*wallpaperConfig)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Database == nil || cfg.Management == nil || cfg.Storage == nil || cfg.Server.Environment != "development" || !strings.HasPrefix(cfg.Database.Name, "gofurry_c2_") || !strings.HasSuffix(cfg.Database.Name, "_test") || cfg.Storage.Bucket != *devBucket || cfg.Storage.PublicBaseURL != *devPublic || *devBucket == "" {
		t.Fatal("unconfirmed isolated database/development storage")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool, e := database.Open(ctx, *cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	var owner string
	if e = pool.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()").Scan(&owner); e != nil || owner != "gofurry_c2_test" || owner != cfg.Database.User {
		t.Fatal("test database ownership mismatch")
	}
	if _, e = pool.Exec(ctx, "TRUNCATE wallpaper_audit_logs,start_wallpapers"); e != nil {
		t.Fatal(e)
	}
	temp := t.TempDir()
	t.Setenv("TMP", temp)
	t.Setenv("TMPDIR", temp)
	var logs lockedLogs
	s := New(platform.NewLogger(&logs, "production"), cfg)
	stopped := make(chan error, 1)
	go func() { stopped <- s.Listen(ctx, cfg.Server.Address) }()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-stopped:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(12 * time.Second):
			t.Error("dual listener shutdown timed out")
		}
	})
	client := &http.Client{Timeout: 100 * time.Second}
	public := "http://" + cfg.Server.Address
	internal := "http://" + cfg.Management.Address
	actor := uuid.NewV7().String()
	headers := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+cfg.Management.Token)
		req.Header.Set("X-LC-Actor-ID", actor)
		req.Header.Set("X-LC-Actor-Name", "HTTP Acceptance")
		req.Header.Set("X-LC-Actor-Role", "owner")
		req.Header.Set("X-LC-Region", cfg.Deployment.Region)
		req.Header.Set("X-LC-Environment", "development")
		req.Header.Set("X-Request-ID", uuid.NewV7().String())
	}
	request := func(method, url string, r io.Reader, change func(*http.Request)) (int, []byte) {
		t.Helper()
		req, e := http.NewRequest(method, url, r)
		if e != nil {
			t.Fatal(e)
		}
		headers(req)
		if r != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if change != nil {
			change(req)
		}
		res, e := client.Do(req)
		if e != nil {
			for errors.Unwrap(e) != nil {
				e = errors.Unwrap(e)
			}
			t.Fatal("HTTP request failed:", e)
		}
		defer res.Body.Close()
		b, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, b
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		req, _ := http.NewRequest("GET", internal+"/internal/v1/readyz", nil)
		headers(req)
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("internal listener not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	base := internal + "/internal/v1/start/wallpapers"
	for _, tc := range []struct {
		name   string
		modify func(*http.Request)
		status int
	}{
		{"missing", func(r *http.Request) { r.Header.Del("Authorization") }, 401}, {"wrong", func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }, 401}, {"query", func(r *http.Request) {
			r.Header.Del("Authorization")
			q := r.URL.Query()
			q.Set("token", cfg.Management.Token)
			r.URL.RawQuery = q.Encode()
		}, 401},
		{"region", func(r *http.Request) { r.Header.Set("X-LC-Region", "OTHER") }, 403}, {"environment", func(r *http.Request) { r.Header.Set("X-LC-Environment", "production") }, 403}, {"actor", func(r *http.Request) { r.Header.Set("X-LC-Actor-ID", "invalid") }, 400}, {"role", func(r *http.Request) { r.Header.Set("X-LC-Actor-Role", "root") }, 400}, {"id", func(r *http.Request) { r.Header.Set("X-Request-ID", "invalid") }, 400},
	} {
		status, _ := request("GET", base, nil, tc.modify)
		if status != tc.status {
			t.Fatalf("%s: %d", tc.name, status)
		}
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		status, _ := request(method, public+"/internal/v1/start/wallpapers", nil, nil)
		if status != 404 {
			t.Fatal("management exposed on public App")
		}
	}
	if status, _ := request("POST", public+"/api/v1/start/backgrounds", nil, nil); status != 405 && status != 404 {
		t.Fatal("public catalog accepts mutations")
	}
	makeUpload := func(imageName, previewName, imageType string, corrupt bool) (*bytes.Buffer, string) {
		t.Helper()
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		writer.WriteField("title", "HTTP fixture")
		for _, part := range []struct{ field, name, typ string }{{"image", imageName, imageType}, {"preview", previewName, "image/webp"}} {
			if part.name == "" {
				continue
			}
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="ignored.bin"`, part.field))
			h.Set("Content-Type", part.typ)
			w, _ := writer.CreatePart(h)
			data, e := os.ReadFile(filepath.Join("../wallpaper/testdata", part.name))
			if e != nil {
				t.Fatal(e)
			}
			if corrupt && part.field == "image" {
				data = data[:20]
			}
			w.Write(data)
		}
		writer.Close()
		return &b, writer.FormDataContentType()
	}
	upload := func(image, preview, typ string, corrupt bool) (int, []byte) {
		body, ct := makeUpload(image, preview, typ, corrupt)
		return request("POST", base, body, func(r *http.Request) { r.Header.Set("Content-Type", ct) })
	}
	for _, tc := range []struct {
		image, preview, typ string
		corrupt             bool
	}{{"image.webp", "preview.webp", "image/avif", false}, {"image.webp", "preview.webp", "image/webp", false}, {"nonwide.avif", "preview.webp", "image/avif", false}, {"image.avif", "nonwide.webp", "image/avif", false}, {"image.avif", "", "image/avif", false}, {"image.avif", "preview.webp", "image/avif", true}} {
		status, _ := upload(tc.image, tc.preview, tc.typ, tc.corrupt)
		if status != 400 {
			t.Fatalf("invalid upload: %d", status)
		}
	}
	oversizeConn, e := net.DialTimeout("tcp", cfg.Management.Address, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	oversizeConn.SetDeadline(time.Now().Add(3 * time.Second))
	oversizeRequest, _ := http.NewRequest("POST", base, nil)
	headers(oversizeRequest)
	oversizeRequest.Header.Set("Content-Type", "multipart/form-data; boundary=too-large")
	oversizeRequest.Header.Set("Content-Length", fmt.Sprint(65<<20))
	fmt.Fprintf(oversizeConn, "POST /internal/v1/start/wallpapers HTTP/1.1\r\nHost: localhost\r\n")
	oversizeRequest.Header.Write(oversizeConn)
	io.WriteString(oversizeConn, "\r\n"+strings.Repeat("x", 8192))
	oversizeResponse, e := http.ReadResponse(bufio.NewReader(oversizeConn), oversizeRequest)
	if e != nil {
		oversizeConn.Close()
		t.Fatal("oversized request response failed")
	}
	oversizeResponse.Body.Close()
	oversizeConn.Close()
	if oversizeResponse.StatusCode != 413 {
		t.Fatal("whole body bound bypassed", oversizeResponse.StatusCode)
	}
	var oversized bytes.Buffer
	writer := multipart.NewWriter(&oversized)
	writer.WriteField("title", "oversized preview")
	w, _ := writer.CreateFormFile("image", "image.avif")
	fixture, _ := os.ReadFile("../wallpaper/testdata/image.avif")
	w.Write(fixture)
	w, _ = writer.CreateFormFile("preview", "preview.webp")
	io.CopyN(w, zeroReader{}, (5<<20)+1)
	writer.Close()
	if status, _ := request("POST", base, &oversized, func(r *http.Request) { r.Header.Set("Content-Type", writer.FormDataContentType()) }); status != 413 {
		t.Fatal("preview bound bypassed", status)
	}
	release1, _ := s.Wallpapers.AcquireUpload()
	release2, _ := s.Wallpapers.AcquireUpload()
	status, _ := upload("image.webp", "preview.webp", "image/webp", false)
	release1()
	release2()
	if status != 429 {
		t.Fatal("concurrent upload budget bypassed")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE wallpaper_audit_logs ADD CONSTRAINT test_http_audit_failure CHECK(action<>'wallpaper.created') NOT VALID"); e != nil {
		t.Fatal(e)
	}
	defer pool.Exec(context.Background(), "ALTER TABLE wallpaper_audit_logs DROP CONSTRAINT IF EXISTS test_http_audit_failure")
	failedStatus, _ := upload("image.avif", "preview.webp", "image/avif", false)
	if _, e = pool.Exec(ctx, "ALTER TABLE wallpaper_audit_logs DROP CONSTRAINT test_http_audit_failure"); e != nil {
		t.Fatal(e)
	}
	var failedRows int
	pool.QueryRow(ctx, "SELECT count(*) FROM start_wallpapers").Scan(&failedRows)
	if failedStatus != 503 || failedRows != 0 {
		t.Fatal("real storage audit-failure status/rollback mismatch", failedStatus, failedRows)
	}
	status, b := upload("image.avif", "preview.webp", "image/avif", false)
	if status != 201 {
		t.Fatalf("real upload: %d %s", status, b)
	}
	var row struct {
		ID       uuid.UUID `json:"id"`
		Revision int64     `json:"revision"`
	}
	if json.Unmarshal(b, &row) != nil || row.ID.String()[14] != '7' {
		t.Fatal("invalid created draft")
	}
	if status, _ = upload("image.avif", "preview.webp", "image/avif", false); status != 409 {
		t.Fatal("duplicate image accepted")
	}
	var originalKey, originalHash string
	if e = pool.QueryRow(ctx, "SELECT preview_key,preview_sha256 FROM start_wallpapers WHERE id=$1", row.ID).Scan(&originalKey, &originalHash); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "UPDATE start_wallpapers SET preview_key=$2,preview_sha256=$3 WHERE id=$1", row.ID, "start/backgrounds/"+strings.Repeat("0", 64)+".webp", strings.Repeat("0", 64)); e != nil {
		t.Fatal(e)
	}
	missingStatus, _ := request("POST", base+"/"+row.ID.String()+"/publish", strings.NewReader(`{"revision":1,"confirm_public_use":true}`), nil)
	if _, e = pool.Exec(ctx, "UPDATE start_wallpapers SET preview_key=$2,preview_sha256=$3 WHERE id=$1", row.ID, originalKey, originalHash); e != nil {
		t.Fatal(e)
	}
	if missingStatus != 503 {
		t.Fatal("missing real development object allowed publication")
	}
	if status, _ = request("POST", base+"/"+row.ID.String()+"/publish", strings.NewReader(`{"revision":1,"confirm_public_use":true}`), nil); status != 200 {
		t.Fatal("real publication failed", status)
	}
	status, b = request("GET", public+"/api/v1/start/backgrounds", nil, nil)
	if status != 200 || !bytes.Contains(b, []byte(row.ID.String())) {
		t.Fatal("published catalog missing wallpaper")
	}
	if status, _ = request("PATCH", base+"/"+row.ID.String(), strings.NewReader(`{"revision":1,"title":"stale"}`), nil); status != 409 {
		t.Fatal("stale metadata overwrite")
	}
	if status, _ = request("POST", base+"/"+row.ID.String()+"/archive", strings.NewReader(`{"revision":2}`), nil); status != 200 {
		t.Fatal("archive failed", status)
	}
	_, b = request("GET", public+"/api/v1/start/backgrounds", nil, nil)
	if bytes.Contains(b, []byte(row.ID.String())) {
		t.Fatal("archived row leaked from cache")
	}
	if status, _ = request("POST", base+"/"+row.ID.String()+"/restore", strings.NewReader(`{"revision":3}`), nil); status != 200 {
		t.Fatal("restore failed", status)
	}
	if status, _ = request("DELETE", base+"/"+row.ID.String(), strings.NewReader(`{"revision":4}`), nil); status != 204 {
		t.Fatal("draft deletion failed", status)
	}
	status, b = upload("4k.avif", "preview.webp", "image/avif", false)
	if status != 201 || json.Unmarshal(b, &row) != nil {
		t.Fatal("real 4K AVIF rejected", status)
	}
	var width, height int
	if e = pool.QueryRow(ctx, "SELECT width,height FROM start_wallpapers WHERE id=$1", row.ID).Scan(&width, &height); e != nil || width != 3840 || height != 2160 {
		t.Fatal("4K dimensions altered")
	}
	if status, _ = upload("4k.avif", "preview.webp", "image/avif", false); status != 409 {
		t.Fatal("4K original duplicated")
	}
	if status, _ = request("DELETE", base+"/"+row.ID.String(), strings.NewReader(`{"revision":1}`), nil); status != 204 {
		t.Fatal("4K fixture cleanup failed")
	}
	// Disconnect while the handler is spooling a streamed body; temp files must disappear.
	conn, e := net.DialTimeout("tcp", cfg.Management.Address, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	boundary := "interrupted-upload"
	prefix := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"image\"; filename=\"image.avif\"\r\nContent-Type: image/avif\r\n\r\n"
	fmt.Fprintf(conn, "POST /internal/v1/start/wallpapers HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\nX-LC-Actor-ID: %s\r\nX-LC-Actor-Name: HTTP Acceptance\r\nX-LC-Actor-Role: owner\r\nX-LC-Region: %s\r\nX-LC-Environment: development\r\nX-Request-ID: %s\r\nContent-Type: multipart/form-data; boundary=%s\r\nContent-Length: 262144\r\n\r\n%s%s", cfg.Management.Token, actor, cfg.Deployment.Region, uuid.NewV7().String(), boundary, prefix, strings.Repeat("x", 8192))
	time.Sleep(100 * time.Millisecond)
	conn.Close()
	deadline = time.Now().Add(3 * time.Second)
	for {
		entries, _ := os.ReadDir(temp)
		if len(entries) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("aborted upload left temporary files")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(logs.String(), cfg.Management.Token) || strings.Contains(logs.String(), cfg.Storage.SecretAccessKey) {
		t.Fatal("credentials leaked to HTTP logs")
	}
	for _, path := range []string{"/api/v1/status", "/api/v1/start/weather?lang=en", "/api/v1/start/location", "/api/v1/start/cities?q=London"} {
		status, _ := request("GET", public+path, nil, nil)
		if status != 200 {
			t.Fatalf("unrelated API failed %s: %d", path, status)
		}
	}
	t.Log("REAL HTTP + POSTGRESQL + DEVELOPMENT R2: dual App isolation, Bearer/actor checks, malformed/concurrent uploads, draft/publication/archive/restore/delete, CDN verification, cancellation cleanup, log hygiene and unrelated APIs passed")
}
