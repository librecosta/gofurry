//go:build integration

package collection

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	json "encoding/json/v2"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"golang.org/x/net/dns/dnsmessage"
)

const harvestRevision = "349730b5c3ff57be084dfac72dfb77f19dacb54c"
const fixtureStream = "gofurry_b1_test"
const discoveryStream = "gofurry_catalog_test"
const fixtureSteamKey = "a920b029bb19982830080c13d82478a1"
const postgresImage = "postgres:18.6@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722"

var harvestSource = flag.String("harvest-source", "", "Read-only Harvest V0.1-D checkout")

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func run(t *testing.T, dir string, env []string, command string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), command, args...)
	cmd.Dir, cmd.Env = dir, env
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("%s %s: %v %s %s", command, args[0], err, output, exit.Stderr)
		}
		t.Fatalf("%s %s: %v", command, args[0], err)
	}
	return strings.TrimSpace(string(output))
}
func expectEventually(t *testing.T, timeout time.Duration, reason string, check func() bool) {
	t.Helper()
	for until := time.Now().Add(timeout); time.Now().Before(until); {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(reason)
}
func fixtureDB(t *testing.T, kind string) (platform.DatabaseConfig, string) {
	t.Helper()
	id := "gofurry-b1-" + kind + "-" + uuid.NewV7().String()
	user := "fixture_" + kind
	db := "b1_" + kind + "_test"
	container := run(t, "", nil, "docker", "run", "--detach", "--name", id, "--label", "gofurry-test=d1-b1",
		"--env", "POSTGRES_HOST_AUTH_METHOD=trust", "--env", "POSTGRES_USER="+user, "--env", "POSTGRES_DB="+db,
		"--publish", "127.0.0.1::5432", postgresImage)
	t.Cleanup(func() {
		cmd := exec.Command("docker", "rm", "--force", "--volumes", container)
		if cmd.Run() != nil {
			t.Error("disposable PostgreSQL cleanup failed")
		}
	})
	endpoint := run(t, "", nil, "docker", "port", container, "5432/tcp")
	host, port, err := net.SplitHostPort(endpoint)
	must(t, err)
	if host != "127.0.0.1" {
		t.Fatal("fixture database not confined to loopback")
	}
	expectEventually(t, 25*time.Second, "PostgreSQL fixture not ready", func() bool {
		return exec.Command("docker", "exec", container, "pg_isready", "-h", "127.0.0.1", "-U", user, "-d", db).Run() == nil
	})
	var n uint16
	_, err = fmt.Sscan(port, &n)
	must(t, err)
	return platform.DatabaseConfig{Host: host, Port: n, Name: db, User: user, Password: rand.Text(), SSLMode: "disable", ConnectTimeout: time.Second,
		MaxConnections: 8, MaxConnectionLifetime: time.Hour, MaxConnectionIdleTime: time.Minute}, container
}

type process struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func launch(t *testing.T, dir string, env []string, bin string, args ...string) *process {
	t.Helper()
	log, err := os.CreateTemp(dir, "process-*.log")
	must(t, err)
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = dir, env, log, log
	must(t, cmd.Start())
	p := &process{cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); _ = log.Close(); close(p.done) }()
	t.Cleanup(p.stop)
	return p
}

func (p *process) stop() { _ = p.cmd.Process.Kill(); <-p.done }

func localAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	must(t, err)
	addr := l.Addr().String()
	must(t, l.Close())
	return addr
}

func probe(t *testing.T, base string) {
	t.Helper()
	client := http.Client{Timeout: time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	expectEventually(t, 20*time.Second, "fixture listener did not become ready", func() bool {
		res, err := client.Get(base + "/readyz")
		if err != nil {
			return false
		}
		res.Body.Close()
		return res.StatusCode == 200
	})
}

func checkConsumerLifecycle(t *testing.T, l *lab, cut *networkCut, db platform.DatabaseConfig, pool *pgxpool.Pool, consumer uuid.UUID, publicID, cursor string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gofurry-api")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	run(t, "../..", nil, "go", "build", "-o", bin, "./cmd/api")
	address := localAddress(t)
	cfg := fmt.Sprintf(`server:
  address: %q
  environment: development
deployment:
  region: CN
logging:
  slow_request: 1s
database:
  host: 127.0.0.1
  port: %d
  name: %q
  user: %q
  password: %q
  ssl_mode: disable
  connect_timeout: 1s
  max_connections: 5
  min_connections: 0
  max_connection_lifetime: 30m
  max_connection_idle_time: 5m
harvest:
  url: %q
  namespace: %q
  consume: true
  reader_token: %q
  consumer_id: %q
  poll_interval: 1s
  page_size: 2
`, address, db.Port, db.Name, db.User, db.Password, cut.server.URL, fixtureStream, l.reader, consumer)
	config := filepath.Join(dir, "api.yaml")
	must(t, os.WriteFile(config, []byte(cfg), 0600))
	_, err := platform.LoadConfig(config)
	must(t, err)
	p := launch(t, dir, nil, bin, "--config", config)
	base := "http://" + address
	probe(t, base)
	expectEventually(t, 10*time.Second, "the Fiber-owned Consumer did not persist and ACK an event", func() bool {
		return scalar[bool](t, pool, "SELECT cursor<>$2 AND NOT ack_pending FROM collection_consumer WHERE namespace=$1", fixtureStream, cursor)
	})
	l.plane.stop()
	probe(t, base)
	response, err := l.http.Get(base + "/api/v1/sites/" + publicID)
	must(t, err)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("catalog HTTP returned %d during Harvest outage", response.StatusCode)
	}
	if runtime.GOOS == "windows" {
		p.stop()
	} else {
		must(t, p.cmd.Process.Signal(syscall.SIGTERM))
		select {
		case <-p.done:
			if !p.cmd.ProcessState.Success() {
				t.Fatal("GoFurry failed to exit cleanly on SIGTERM")
			}
		case <-time.After(12 * time.Second):
			t.Fatal("GoFurry did not drain on SIGTERM")
		}
	}
	expectEventually(t, 4*time.Second, "Fiber-owned collection session leaked after shutdown", func() bool {
		return scalar[int64](t, pool, "SELECT count(*) FROM pg_stat_activity WHERE application_name='gofurry-collection'") == 0
	})
	l.worker.stop()
	l.start()
}

type lab struct {
	t                                       *testing.T
	dir, bin, address, config, workerConfig string
	writer, reader, operator                string
	discoveryWriter, discoveryReader        string
	env                                     []string
	net                                     *httptest.Server
	http                                    *http.Client
	plane, worker                           *process
	mode, dns, news                         atomic.Int32
	intakeVersion                           atomic.Int32
	discoveryMode                           atomic.Int32
	created                                 int64
	calls                                   atomic.Int64
	certificate                             atomic.Pointer[tls.Certificate]
	sign                                    func() *tls.Certificate
}

func newLab(t *testing.T, db platform.DatabaseConfig) *lab {
	t.Helper()
	dir := t.TempDir()
	l := &lab{t: t, dir: dir, address: localAddress(t), writer: rand.Text() + rand.Text(), reader: rand.Text() + rand.Text(),
		operator: rand.Text() + rand.Text(), http: &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{}}}
	l.discoveryWriter, l.discoveryReader = rand.Text()+rand.Text(), rand.Text()+rand.Text()
	l.created = time.Now().UTC().Unix()
	t.Cleanup(l.http.CloseIdleConnections)
	source, err := filepath.Abs(*harvestSource)
	must(t, err)
	if run(t, source, nil, "git", "rev-parse", "HEAD") != harvestRevision ||
		run(t, source, nil, "git", "status", "--porcelain", "--untracked-files=no") != "" {
		t.Fatal("expected clean pinned Harvest Steam catalog sources")
	}
	l.bin = filepath.Join(dir, "harvest-fixture")
	if runtime.GOOS == "windows" {
		l.bin += ".exe"
	}
	run(t, source, nil, "go", "test", "-c", "-tags=integration", "-o", l.bin, "./integration")
	ca := l.network()
	fixture := filepath.Join(dir, "network.json")
	raw, err := json.Marshal(map[string]any{"CA": ca, "Targets": map[string]string{
		"93.184.216.34:443": l.net.Listener.Addr().String(), "8.8.8.8:443": l.net.Listener.Addr().String(),
	}})
	must(t, err)
	must(t, os.WriteFile(fixture, raw, 0600))
	l.env = append(os.Environ(), "HARVEST_TEST_WEB="+fixture)
	l.config = filepath.Join(dir, "server.yaml")
	l.workerConfig = filepath.Join(dir, "worker.yaml")
	worker := rand.Text() + rand.Text()
	capabilities := []string{"web.http", "web.dns", "web.tls", "steam.details", "steam.news", "steam.players"}
	var yaml strings.Builder
	fmt.Fprintf(&yaml, `http:
  listen: %q
database:
  host: 127.0.0.1
  port: %d
  name: %q
  user: %q
  password: %q
  tls_mode: disable
principals:
`, l.address, db.Port, db.Name, db.User, db.Password)
	for _, grant := range []struct{ name, role, token string }{
		{"b1_writer", "writer", l.writer}, {"b1_reader", "reader", l.reader}, {"b1_operator", "operator", l.operator}, {"b1_worker", "worker", worker},
	} {
		grantNS, grantCaps := fixtureStream, strings.Join(capabilities, ", ")
		if grant.role == "worker" || grant.role == "operator" {
			grantNS += ", " + discoveryStream
			grantCaps += ", steam.catalog"
		}
		fmt.Fprintf(&yaml, `  - name: %s
    role: %s
    token: %q
    namespaces: [%s]
    capabilities: [%s]
`,
			grant.name, grant.role, grant.token, grantNS, grantCaps)
		if grant.role == "worker" {
			fmt.Fprintln(&yaml, `    max_slots: 1
    egress_profiles: [local]`)
		}
	}
	fmt.Fprintf(&yaml, `  - {name: catalog_writer, role: writer, token: %q, namespaces: [%s], capabilities: [steam.catalog]}
  - {name: catalog_reader, role: reader, token: %q, namespaces: [%s], capabilities: [steam.catalog]}
`, l.discoveryWriter, discoveryStream, l.discoveryReader, discoveryStream)
	fmt.Fprintln(&yaml, `network:
  global_bucket: global
  routes:
    - profile: local
      mode: direct
      paths:
        - {id: direct, kind: direct, region: cn, bucket: direct}
  sources:`)
	for _, cap := range capabilities {
		provider, account := "web", "web_account"
		if strings.HasPrefix(cap, "steam.") {
			provider, account = "steam_api", "steam_account"
		}
		if cap == "steam.details" {
			provider, account = "steam_store", "store_account"
		}
		fmt.Fprintf(&yaml, `    - {capability: %s, namespace: %s, profile: local, provider_bucket: %s, account_bucket: %s}
`, cap, fixtureStream, provider, account)
	}
	fmt.Fprintf(&yaml, "    - {capability: steam.catalog, namespace: %s, profile: local, provider_bucket: steam_catalog, account_bucket: steam_account}\n", discoveryStream)
	fmt.Fprintln(&yaml, "  quotas:")
	for _, bucket := range []string{"global", "direct", "web", "web_account", "steam_api", "steam_account", "store_account"} {
		fmt.Fprintf(&yaml, "    - {id: %s, burst: 32, per_minute: 600, concurrency: 4", bucket)
		if bucket == "steam_account" {
			fmt.Fprint(&yaml, ", daily_limit: 200")
		}
		fmt.Fprintln(&yaml, "}")
	}
	fmt.Fprintln(&yaml, "    - {id: steam_store, burst: 1, per_minute: 30, concurrency: 1}")
	fmt.Fprintln(&yaml, "    - {id: steam_catalog, burst: 8, per_minute: 60, concurrency: 1}")
	must(t, os.WriteFile(l.config, []byte(yaml.String()), 0600))
	yaml.Reset()
	fmt.Fprintf(&yaml, `steam_catalog_api_key: %q
control_url: %q
token: %q
concurrency: 1
offers:
`, fixtureSteamKey, "http://"+l.address, worker)
	for _, cap := range capabilities {
		impl := strings.Split(cap, ".")[0] + "_v1"
		fmt.Fprintf(&yaml, `  - {capability: %s, request_schema: 1, output_schema: 1, implementation: %s}
`, cap, impl)
	}
	fmt.Fprintln(&yaml, "  - {capability: steam.catalog, request_schema: 1, output_schema: 1, implementation: steam_v1}")
	fmt.Fprintln(&yaml, `egress:
  profiles:
    - {name: local, mode: direct, direct_region: cn}`)
	must(t, os.WriteFile(l.workerConfig, []byte(yaml.String()), 0600))
	run(t, dir, l.env, l.bin, "fixture", "migrate", "up", "--config", l.config)
	run(t, dir, l.env, l.bin, "fixture", "policy", "init", "--config", l.config)
	l.start()
	return l
}

func (l *lab) start() {
	l.plane = launch(l.t, l.dir, l.env, l.bin, "fixture", "serve", "--local", "--config", l.config)
	probe(l.t, "http://"+l.address)
	l.worker = launch(l.t, l.dir, l.env, l.bin, "fixture", "worker", "--config", l.workerConfig)
}

func (l *lab) job(req request) {
	t := l.t
	t.Helper()
	data, err := json.Marshal(struct {
		Request request `json:"request"`
	}{req})
	must(t, err)
	path := "http://" + l.address + "/api/v1/jobs"
	r, err := http.NewRequestWithContext(t.Context(), "POST", path, bytes.NewReader(data))
	must(t, err)
	r.Header.Set("Authorization", "Bearer "+l.writer)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", uuid.NewV7().String())
	resp, err := l.http.Do(r)
	must(t, err)
	var j struct {
		ID uuid.UUID `json:"id"`
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	must(t, err)
	if resp.StatusCode != 202 {
		t.Fatalf("Harvest Job submit returned %d for %s", resp.StatusCode, req.Capability)
	}
	must(t, json.Unmarshal(b, &j))
	cli := &client{base: "http://" + l.address, http: l.http}
	expectEventually(t, 30*time.Second, "Harvest Job did not complete: "+req.Capability, func() bool {
		var status struct {
			Job struct {
				State string `json:"state"`
			} `json:"job"`
		}
		if err := cli.call(t.Context(), l.writer, "GET", "jobs/"+j.ID.String(), nil, nil, &status, 200); err != nil {
			return false
		}
		if status.Job.State == "failed" || status.Job.State == "cancelled" {
			t.Fatalf("Harvest Job %s: %s", req.Capability, status.Job.State)
		}
		return status.Job.State == "succeeded" || status.Job.State == "partial"
	})
}

func (l *lab) network() string {
	t := l.t
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	ca.Subject.CommonName = "Disposable B1 CA"
	root, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	must(t, err)
	var serial int64 = 1
	l.sign = func() *tls.Certificate {
		serial++
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		must(t, err)
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(8 * time.Hour),
			DNSNames: []string{"*.example.com", "dns.google", "store.steampowered.com", "api.steampowered.com", "partner.steam-api.com"},
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		leaf, err := x509.CreateCertificate(rand.Reader, template, ca, pub, key)
		must(t, err)
		return &tls.Certificate{Certificate: [][]byte{leaf}, PrivateKey: priv}
	}
	l.certificate.Store(l.sign())
	l.net = httptest.NewUnstartedServer(http.HandlerFunc(l.feed))
	l.net.TLS = &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return l.certificate.Load(), nil }}
	l.net.StartTLS()
	t.Cleanup(l.net.Close)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root}))
}

func (l *lab) feed(w http.ResponseWriter, r *http.Request) {
	l.calls.Add(1)
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.Query().Has("key") {
		l.t.Error("credential reached a simulated source")
	}
	w.Header().Set("Connection", "close")
	q := r.URL.Query()
	switch r.Host {
	case "dns.google":
		raw, err := base64.RawURLEncoding.DecodeString(q.Get("dns"))
		var req dnsmessage.Message
		if err != nil || req.Unpack(raw) != nil || len(req.Questions) != 1 {
			w.WriteHeader(400)
			return
		}
		rr := req.Questions[0]
		mode := l.dns.Load()
		address, ttl := byte(8), uint32(300)
		if mode == 1 {
			ttl = 42
		}
		if mode == 2 {
			address = 9
		}
		reply := dnsmessage.Message{Header: dnsmessage.Header{ID: req.ID, Response: true, RecursionAvailable: true}, Questions: req.Questions,
			Answers: []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: rr.Name, Class: rr.Class, TTL: ttl}, Body: &dnsmessage.AResource{A: [4]byte{8, 8, 8, address}}}}}
		raw, _ = reply.Pack()
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(raw)
	case "partner.steam-api.com":
		l.catalogFeed(w, r)
	case "store.steampowered.com":
		w.Header().Set("Content-Type", "application/json")
		extra := `"is_free":true`
		switch q.Get("cc") {
		case "US":
			extra = `"is_free":false,"price_overview":{"currency":"USD","initial":0,"final":0,"discount_percent":0}`
		case "HK":
			extra = `"is_free":false`
		}
		app := q.Get("appids")
		name, kind, description := "Observed "+q.Get("l"), "game", ""
		if app == "42" || app == "3000000001" {
			extra += `,"supported_languages":"English<br>Simplified Chinese<script>private-script</script>","developers":["Fox Studio"],"publishers":["Pine Press"],"platforms":{"windows":true,"mac":false},"categories":[{"id":2,"description":"Single-player"}],"genres":[{"id":"23","description":"Indie"}],"release_date":{"coming_soon":true,"date":"Q3 2027"},"header_image":"https://cdn.akamai.steamstatic.com/fixture/header.jpg?secret=fixture","capsule_image":"https://cdn.akamai.steamstatic.com/fixture/capsule.jpg","screenshots":[{"id":0,"path_full":"https://cdn.akamai.steamstatic.com/fixture/shot.jpg"},{"id":1,"path_full":"https://not-steam.example/fixture/unsafe.jpg"}],"movies":[{"id":0,"mp4":{"max":"https://video.steamcontent.com/fixture/movie.mp4"}}]`
		}
		switch app {
		case "3100000001", "3100000002", "3100000003", "3100000004", "3100000005", "3100000006", "3100000007", "3100000008", "3100000009":
			name, kind, description, extra = l.eligibilityFixture(app, name, kind, description, extra)
		case "42":
			description = "An independent English description."
			if q.Get("l") == "schinese" && q.Get("cc") != "HK" {
				name, description = "松林故事", "森林里的狐狸重新打开了图书馆。"
			}
		case "3000000001", "3000000005":
			name, description = "Fable of the Foxes", "<p>An anthropomorphic fox cast restores a library.</p><script>fixture-private-marker</script>"
			if app == "3000000001" && q.Get("l") == "schinese" && q.Get("cc") == "CN" {
				name, description = "狐狸寓言", "一群狐狸重建了他们的图书馆。"
			} else if l.intakeVersion.Load() == 1 && app == "3000000001" {
				name, description = "Fable of the Foxes Revised", "The fox librarians reopen their town library."
			}
		case "3000000002":
			name, description = "Unknown Orchard", "An orchard with only a Storefront synopsis."
		case "3000000003":
			name, kind = "Orchard Demo", "demo"
		case "3000000004":
			name, kind = "Mysterious App", ""
		case "3000000006":
			name, description = "Before Intake", "A detail record with no change on the next job."
		case "3000000007":
			name, description = "After Outage", "A game collected after Harvest recovers."
		}
		_, _ = fmt.Fprintf(w, `{%q:{"success":true,"data":{"steam_appid":%s,"name":%q,"type":%q,"short_description":%q,%s}}}`,
			app, app, name, kind, description, extra)
	case "api.steampowered.com":
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "GetNumberOfCurrentPlayers") {
			_, _ = io.WriteString(w, `{"response":{"result":1,"player_count":0}}`)
			return
		}
		items := "[]"
		if l.news.Load() == 0 {
			items = fmt.Sprintf(`[{"gid":"123456","title":"Fixture update","date":%d,"url":"https://steamcommunity.com/games/42/announcements/detail/123456?token=discard","contents":"discard-raw-article"}]`, time.Now().Unix())
		}
		_, _ = fmt.Fprintf(w, `{"appnews":{"appid":%s,"newsitems":%s,"count":1}}`, q.Get("appid"), items)
	default:
		if l.mode.Load() == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.Method == "HEAD" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Set-Cookie", "private=discard-cookie")
		generator, script := "WordPress 6", "/wp-content/script.js"
		if l.mode.Load() == 1 {
			generator, script = "Ghost 5", "/assets/built/ghost-sdk.js"
		}
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><title>Fixture title</title><meta name="generator" content="%s"><link rel="canonical" href="/home?token=discard-query"></head><body><script src="%s"></script></body></html>`, generator, script)
	}
}

type networkCut struct {
	server         *httptest.Server
	post, ack      atomic.Bool
	schedulePosts  atomic.Int32
	jobPosts       atomic.Int32
	jobDrop        atomic.Bool
	offline        atomic.Bool
	pauseSnapshot  atomic.Bool
	snapshotRead   chan struct{}
	resumeSnapshot chan struct{}
}

func proxyLab(t *testing.T, l *lab) *networkCut {
	t.Helper()
	cut := &networkCut{snapshotRead: make(chan struct{}), resumeSnapshot: make(chan struct{})}
	target, _ := url.Parse("http://" + l.address)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{}
	proxy.ModifyResponse = func(r *http.Response) error {
		path := r.Request.URL.Path
		if r.Request.Method == "POST" && path == "/api/v1/jobs" {
			cut.jobPosts.Add(1)
			if r.StatusCode == 202 && cut.jobDrop.CompareAndSwap(true, false) {
				return errNetwork
			}
		}
		if r.Request.Method == "POST" && path == "/api/v1/schedules" {
			cut.schedulePosts.Add(1)
			if r.StatusCode == 201 && cut.post.CompareAndSwap(true, false) {
				return errNetwork
			}
		}
		if r.Request.Method == "POST" && strings.HasSuffix(path, "/ack") && r.StatusCode == 200 && cut.ack.CompareAndSwap(true, false) {
			return errNetwork
		}
		if path == "/api/v1/snapshots/current" && r.StatusCode == 200 && cut.pauseSnapshot.CompareAndSwap(true, false) {
			close(cut.snapshotRead)
			select {
			case <-cut.resumeSnapshot:
			case <-r.Request.Context().Done():
			}
		}
		return nil
	}
	proxy.ErrorHandler = func(http.ResponseWriter, *http.Request, error) { panic(http.ErrAbortHandler) }
	cut.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cut.offline.Load() {
			panic(http.ErrAbortHandler)
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(cut.server.Close)
	return cut
}

func runWithTimeout(ctx context.Context, timeout time.Duration, run func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return run(ctx)
}
