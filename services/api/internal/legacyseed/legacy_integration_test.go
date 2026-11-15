//go:build integration

package legacyseed

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

const testImage = "postgres:18.6@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722"
const fixturePassword = "d5-only-synthetic=&%# ?"

func TestLegacySeedPostgreSQL18(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	base := t.TempDir()
	run := func(input string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdin = strings.NewReader(input)
		body, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(body)), err
	}
	docker := func(input string, args ...string) string {
		t.Helper()
		out, err := run(input, args...)
		if err != nil {
			t.Fatalf("docker fixture command failed: %v\n%s", err, out)
		}
		return out
	}
	// Docker writes image-pull progress to stderr when the image is cold.
	// Use a CID file so validation and cleanup only consume the container ID.
	cidPath := filepath.Join(base, "postgres.cid")
	docker("", "run", "-d", "--rm", "--cidfile", cidPath, "-p", "127.0.0.1::5432",
		"-e", "POSTGRES_PASSWORD=fixture-superuser-only", "-e", "POSTGRES_INITDB_ARGS=--encoding=UTF8", testImage)
	container := strings.TrimSpace(string(readTestFile(t, cidPath)))
	if !regexp.MustCompile("^[a-f0-9]{64}$").MatchString(container) {
		t.Fatal("unexpected fixture container identity")
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", container).Run() })
	portLine := docker("", "port", container, "5432/tcp")
	port, err := strconv.ParseUint(strings.TrimPrefix(portLine, "127.0.0.1:"), 10, 16)
	if err != nil {
		t.Fatal("invalid isolated PostgreSQL port")
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		if _, err := run("", "exec", container, "pg_isready", "-h", "127.0.0.1", "-U", "postgres"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the disposable fixture")
		}
		time.Sleep(300 * time.Millisecond)
	}
	psql := func(db, sql string) string {
		t.Helper()
		return docker("SET default_transaction_read_only=off;\n"+sql, "exec", "-i", container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-qAt", "-U", "postgres", "-d", db)
	}
	psql("postgres", `CREATE ROLE legacy_d5_restore NOLOGIN;
		CREATE ROLE legacy_d5_reader LOGIN PASSWORD 'd5-only-synthetic=&%# ?';
		CREATE DATABASE legacy_d5_gfn OWNER legacy_d5_restore;
		CREATE DATABASE legacy_d5_gfg OWNER legacy_d5_restore;
		CREATE DATABASE legacy_d5_gfa OWNER legacy_d5_restore;`)
	for _, kind := range []string{"gfn", "gfg", "gfa"} {
		db := "legacy_d5_" + kind
		fixture, err := os.ReadFile(filepath.Join("testdata", kind+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		input := "fixture_source_" + kind
		psql("postgres", "CREATE DATABASE "+input)
		psql(input, string(fixture))
		dump := "/tmp/" + kind + "-fixture.dump"
		docker("", "exec", container, "pg_dump", "-U", "postgres", "-Fc", "-f", dump, input)
		docker("", "exec", container, "pg_restore", "-U", "postgres", "--role=legacy_d5_restore", "--exit-on-error", "--no-owner", "--no-acl", "-d", db, dump)
		psql(db, "ALTER DATABASE "+db+" SET default_transaction_read_only=on")
		if kind == "gfa" {
			psql(db, "GRANT SELECT(kind) ON public.gfa_content_idea TO legacy_d5_reader")
		} else {
			psql(db, "GRANT SELECT ON ALL TABLES IN SCHEMA public TO legacy_d5_reader")
		}
	}
	c := Config{Snapshot: "fixture-2026", BackupSHA: strings.Repeat("a", 64), TargetDB: "gofurry_unused", Output: filepath.Join(base, "candidate"), Logos: filepath.Join(base, "logo-mirror")}
	database := func(kind string) Database {
		return Database{"127.0.0.1", uint16(port), "legacy_d5_" + kind, "legacy_d5_reader", fixturePassword}
	}
	c.GFN, c.GFG, c.GFA = database("gfn"), database("gfg"), database("gfa")
	pngData := samplePNG(t)
	key := func(id int, data []byte) string { return fmt.Sprintf("nav/sites/%d/icon/%s.png", id, sha(data)[:32]) }
	makeObject(t, c.Logos, key(1, pngData), pngData)
	makeObject(t, c.Logos, key(4, pngData), pngData)
	broken := append(bytes.Clone(pngData), 1)
	makeObject(t, c.Logos, key(3, broken), broken)
	psql(c.GFN.Name, fmt.Sprintf("UPDATE gfn_site SET icon=CASE id WHEN 1 THEN '%s' WHEN 2 THEN '%s' WHEN 3 THEN '%s' WHEN 4 THEN '%s' ELSE icon END",
		key(1, pngData), key(2, pngData), key(3, broken), key(4, pngData)))

	binary := filepath.Join(base, "legacy-seed")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/legacy-seed")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	configFile := filepath.Join(base, "fixture.yaml")
	writeConfig := func(x Config) {
		t.Helper()
		raw, err := yaml.Marshal(x)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configFile, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	unsafe := c
	unsafe.GFG.Name = c.GFN.Name
	writeConfig(unsafe)
	if _, err := LoadConfig(configFile, ""); err == nil {
		t.Fatal("duplicate db accepted")
	}
	unsafe.GFG.Name = "gfg"
	writeConfig(unsafe)
	if _, err := LoadConfig(configFile, ""); err == nil {
		t.Fatal("production db name accepted")
	}
	unsafe.GFG.Name = c.TargetDB
	writeConfig(unsafe)
	if _, err := LoadConfig(configFile, ""); err == nil {
		t.Fatal("application db name accepted")
	}
	unsafe = c
	unsafe.Logos = filepath.Join(c.Output, "mirror")
	writeConfig(unsafe)
	if _, err := LoadConfig(configFile, ""); err == nil {
		t.Fatal("output inside source mirror was accepted")
	}
	writeConfig(c)
	call := func(mode string, flags ...string) (Result, string, error) {
		args := append([]string{mode, "--config", configFile}, flags...)
		cmd := exec.CommandContext(ctx, binary, args...)
		out, err := cmd.Output()
		var result Result
		if err == nil && json.Unmarshal(out, &result) != nil {
			t.Fatal("CLI returned malformed result")
		}
		if e, ok := err.(*exec.ExitError); ok {
			out = append(out, e.Stderr...)
		}
		return result, string(out), err
	}
	success := func(mode string, flags ...string) Result {
		t.Helper()
		r, log, err := call(mode, flags...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, log)
		}
		return r
	}
	success("inspect")
	report := readTestFile(t, filepath.Join(c.Output, c.Snapshot+".inspect", "reports", "summary.json"))
	if bytes.Contains(report, []byte("fixture-private")) || !bytes.Contains(report, []byte("asset_not_inspected")) {
		t.Fatal("inspect did not redact or distinguish assets")
	}
	if _, err := os.Stat(filepath.Join(c.Output, c.Snapshot+".inspect", "content", "sites.ndjson")); !os.IsNotExist(err) {
		t.Fatal("inspect wrote private seed rows")
	}
	first := success("extract")
	if first.Status != "ready_for_D5_B" || first.AssetComplete {
		t.Fatalf("unexpected extraction state %v", first)
	}
	second := success("extract", "--output", filepath.Join(base, "repeat"))
	if first != second {
		t.Fatal("identical source content produced different hash")
	}
	root := filepath.Join(c.Output, c.Snapshot)
	var m manifest
	if err := json.Unmarshal(readTestFile(t, filepath.Join(root, "manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	wantIssues := []string{"archived_reference_site_id", "orphan_site_id", "address_ambiguous", "address_missing", "address_invalid", "duplicate_appid",
		"duplicate_code", "collection_manual_exclusion_conflict", "archived_reference_tag_id", "unsafe_key", "missing_object", "invalid_image",
		"logo_duplicate_content", "groups_invalid_entry", "developers_invalid_json", "info_oversize", "adult_flag_unknown"}
	for _, issue := range wantIssues {
		if m.Summary.Issues[issue] == 0 {
			t.Errorf("missing issue %s", issue)
		}
	}
	if m.Summary.UniqueLogos != 1 || m.Summary.Logos["ready"] != 2 || m.Summary.Ideas["total"] != 2 || m.Summary.Ideas["unknown"] != 1 ||
		m.Summary.Tables["gfg_game_first_available"].Excluded != 1 || m.Summary.Tables["gfg_game_release_state"].Excluded != 1 ||
		m.Summary.Tables["gfg_game"].Source != 4 || m.Summary.Tables["gfn_site"].Source != 7 {
		t.Fatalf("unexpected source reconciliation %#v", m.Summary)
	}
	fileContains := func(path string, required ...string) {
		t.Helper()
		raw := readTestFile(t, filepath.Join(root, path))
		for _, want := range required {
			if !bytes.Contains(raw, []byte(want)) {
				t.Errorf("%s missing %s", path, want)
			}
		}
	}
	fileContains("content/sites.ndjson", "示例网站", `"proposed_url":"https://www.example.com"`, `"create_time_utc":"2020-05-03T08:00:00Z"`, `"first_published_at":null`, `"address_basis":"explicit"`)
	fileContains("content/games.ndjson", `"adult_evidence":"legacy_adult"`, `"legacy_public_filter":"not_flagged_adult"`, `"steam_appid":null`, `"steam_appid":100`)
	fileContains("content/collection_manual_members.ndjson", `"export_order":2`)
	fileContains("content/collection_tag_rules.ndjson", `"tag_id":2`)
	fileContains("content/collection_exclusions.ndjson", `"game_id":3`)
	fileContains("content/collection_home_slots.ndjson", `"slot":5`)
	fileContains("reports/excluded-release-rows.ndjson", "gfg_game_first_available", "gfg_game_release_state", "non_editorial_release_evidence")
	fileContains("content/game_release_candidates.ndjson", `"manual_release_evidence":true`)
	for _, f := range []string{"summary.json", "summary.md"} {
		raw := readTestFile(t, filepath.Join(root, "reports", f))
		for _, secret := range []string{"https://", "nav/sites/", "private", "synthetic-password", fixturePassword, "unexpected-private-kind", "sensitive-fake-title", "示例网站"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatal("aggregate report contains private fixture data")
			}
		}
	}

	for _, n := range []int{100, 1000, 10000} {
		psql(c.GFN.Name, fmt.Sprintf("INSERT INTO gfn_site (id,name,name_en,info,info_en,create_time,update_time,nsfw,deleted) SELECT i,'批量','Bulk','说明','Summary','2020-05-03','2020-05-03','0',false FROM generate_series(8,%d) i ON CONFLICT DO NOTHING", n))
		dest := filepath.Join(base, fmt.Sprintf("volume-%d", n))
		scaled := c
		scaled.Output = dest
		start := time.Now()
		scaleResult, sampledHeap, err := observeExtraction(ctx, scaled)
		if err != nil {
			t.Fatal(err)
		}
		observed := time.Since(start)
		p := filepath.Join(dest, c.Snapshot, "content", "sites.ndjson")
		body := readTestFile(t, p)
		if bytes.Count(body, []byte{'\n'}) != n {
			t.Fatalf("expected %d streamed rows", n)
		}
		t.Logf("fixture_site_rows=%d sites_ndjson_bytes=%d extract_and_self_verify=%s sampled_go_heap_bytes=%d content_sha256=%s", n, len(body), observed, sampledHeap, scaleResult.ContentSHA)
	}
	limited := c
	limited.Limits = Limits{SourceRows: 9}
	writeConfig(limited)
	if _, out, err := call("extract", "--output", filepath.Join(base, "low-limit")); err == nil || !strings.Contains(out, "exceed configured 9 rows") {
		t.Fatal("configured source budget was not enforced")
	}
	writeConfig(c)
	psql(c.GFA.Name, "ALTER TABLE gfa_content_idea RENAME TO wrong_content_idea")
	fallback := filepath.Join(base, "gfa-missing")
	success("inspect", "--output", fallback)
	if raw := readTestFile(t, filepath.Join(fallback, c.Snapshot+".inspect", "reports", "summary.json")); !bytes.Contains(raw, []byte(`"gfa_status":"unsupported"`)) {
		t.Fatal("missing optional GFA table was not reported")
	}
	bad := c
	bad.GFN.Password = "synthetic-wrong-credential"
	writeConfig(bad)
	if _, out, err := call("inspect", "--output", filepath.Join(base, "bad-auth")); err == nil || strings.Contains(out, bad.GFN.Password) || !strings.Contains(out, "cannot connect") {
		t.Fatal("bad credentials were accepted or shown")
	}
	writeConfig(c)
	t.Setenv("PGHOST", "192.0.2.20")
	if _, out, err := call("inspect", "--output", filepath.Join(base, "ambient-pg")); err == nil || !strings.Contains(out, "remove PG*") || strings.Contains(out, "192.0.2.20") {
		t.Fatal("inherited PG settings were consumed or shown")
	}
	if err := os.Unsetenv("PGHOST"); err != nil {
		t.Fatal(err)
	}

	psql(c.GFN.Name, "GRANT UPDATE ON gfn_site TO legacy_d5_reader")
	if _, out, err := call("inspect", "--output", filepath.Join(base, "writer")); err == nil || !strings.Contains(out, "no write") {
		t.Fatal("writable reader was accepted")
	}
	psql(c.GFN.Name, "REVOKE UPDATE ON gfn_site FROM legacy_d5_reader; ALTER TABLE gfn_site ALTER COLUMN nsfw TYPE integer USING NULL")
	if _, out, err := call("extract", "--output", filepath.Join(base, "wrong-schema")); err == nil || !strings.Contains(out, "gfn_site.nsfw") {
		t.Fatal("unsupported schema was accepted")
	}
	if _, err := os.Stat(filepath.Join(base, "wrong-schema", c.Snapshot+".failed", "reports", "summary.json")); err != nil {
		t.Fatal("redacted failure report missing")
	}

	// Verify while all databases are unavailable.
	docker("", "stop", container)
	if verified := success("verify"); verified != first {
		t.Fatal("offline verification result disagrees")
	}
	extra := filepath.Join(root, "unlisted")
	if err := os.WriteFile(extra, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, out, err := call("verify"); err == nil || !strings.Contains(out, "unexpected private package file") {
		t.Fatal("unlisted file was accepted")
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	repeatRoot := filepath.Join(base, "repeat", c.Snapshot)
	tamperRelationAndChecksums(t, repeatRoot)
	if _, out, err := call("verify", "--output", filepath.Join(base, "repeat")); err == nil || !strings.Contains(out, "references or reconciliation") {
		t.Fatal("semantic relationship corruption with recomputed checksums was accepted")
	}
	p := filepath.Join(root, "content", "sites.ndjson")
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("tampered\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, out, err := call("verify"); err == nil || !strings.Contains(out, "checksum") {
		t.Fatal("tampered file was accepted")
	}
}

func tamperRelationAndChecksums(t *testing.T, root string) {
	t.Helper()
	path := "content/site_group_members.ndjson"
	changed := bytes.Replace(readTestFile(t, filepath.Join(root, path)), []byte(`"group_id":1`), []byte(`"group_id":999`), 1)
	makeObject(t, root, path, changed)
	var m manifest
	if err := json.Unmarshal(readTestFile(t, filepath.Join(root, "manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	for i := range m.Files {
		if f := &m.Files[i]; f.Path == path {
			f.SHA = sha(changed)
			f.Bytes = int64(len(changed))
		}
	}
	m.ContentSHA = sha(checksumLines(m.Files))
	body, err := marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	makeObject(t, root, "manifest.json", body)
	all := append(m.Files, entry{Path: "manifest.json", SHA: sha(body)})
	slices.SortFunc(all, func(a, b entry) int { return strings.Compare(a.Path, b.Path) })
	makeObject(t, root, "SHA256SUMS", checksumLines(all))
}

func observeExtraction(ctx context.Context, c Config) (Result, uint64, error) {
	runtime.GC()
	var high atomic.Uint64
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > high.Load() {
				high.Store(stats.HeapAlloc)
			}
			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	})
	r, err := Run(ctx, "extract", c)
	close(done)
	wg.Wait()
	return r, high.Load(), err
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
