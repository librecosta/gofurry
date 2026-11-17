//go:build integration

package legacyimport

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"go.yaml.in/yaml/v3"
)

const postgres18 = "postgres:18.6@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722"
const pgPassword = "fixture-superuser-only"

func localPostgres(t *testing.T, ctx context.Context, base string) (uint16, func(string, string)) {
	t.Helper()
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
			t.Fatalf("disposable PostgreSQL: %v\n%s", err, out)
		}
		return out
	}
	cidPath := filepath.Join(base, "postgres.cid")
	docker("", "run", "-d", "--rm", "--cidfile", cidPath, "-p", "127.0.0.1::5432",
		"-e", "POSTGRES_PASSWORD="+pgPassword, "-e", "POSTGRES_INITDB_ARGS=--encoding=UTF8", postgres18)
	cid := strings.TrimSpace(string(testRead(t, cidPath)))
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(cid) {
		t.Fatal("invalid disposable container identity")
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", cid).Run() })
	rawPort := docker("", "port", cid, "5432/tcp")
	port, err := strconv.ParseUint(strings.TrimPrefix(rawPort, "127.0.0.1:"), 10, 16)
	if err != nil || port == 0 {
		t.Fatal("invalid disposable port")
	}
	for deadline := time.Now().Add(50 * time.Second); ; {
		if _, err = run("", "exec", cid, "pg_isready", "-h", "127.0.0.1", "-U", "postgres"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture PostgreSQL server did not become ready")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return uint16(port), func(db, sql string) {
		docker(sql, "exec", "-i", cid, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-qAt", "-U", "postgres", "-d", db)
	}
}
func testRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func testWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, ctx context.Context, base string) (Config, legacyseed.Config, func(string, string), string) {
	t.Helper()
	port, psql := localPostgres(t, ctx, base)
	psql("postgres", `CREATE ROLE legacy_d5_restore NOLOGIN;
CREATE ROLE legacy_d5_reader LOGIN PASSWORD 'fixture-reader-only';
CREATE DATABASE legacy_d5_gfn OWNER legacy_d5_restore;
CREATE DATABASE legacy_d5_gfg OWNER legacy_d5_restore;
CREATE DATABASE legacy_d5_gfa OWNER legacy_d5_restore;
CREATE DATABASE gofurry_d5b1_fixture;`)

	load := func(kind string) legacyseed.Database {
		db := "legacy_d5_" + kind
		raw := testRead(t, filepath.Join("..", "legacyseed", "testdata", kind+".sql"))
		psql(db, "SET ROLE legacy_d5_restore;\n"+string(raw))
		if kind == "gfa" {
			psql(db, "GRANT SELECT(kind) ON public.gfa_content_idea TO legacy_d5_reader;")
		} else {
			psql(db, "GRANT SELECT ON ALL TABLES IN SCHEMA public TO legacy_d5_reader;")
		}
		return legacyseed.Database{Host: "127.0.0.1", Port: port, Name: db, User: "legacy_d5_reader", Password: "fixture-reader-only"}
	}

	seed := legacyseed.Config{Snapshot: "d5b1-fixture", BackupSHA: strings.Repeat("a", 64), TargetDB: "gofurry_d5b1_fixture",
		Output: filepath.Join(base, "sealed"), Logos: filepath.Join(base, "logos"), GFN: load("gfn"), GFG: load("gfg"), GFA: load("gfa")}
	logo := testRead(t, filepath.Join("..", "storage", "testdata", "site-logo.webp"))
	for _, id := range []int{1, 2, 4} {
		k := fmt.Sprintf("nav/sites/%d/icon/%s.webp", id, digest(logo)[:32])
		f := filepath.Join(seed.Logos, filepath.FromSlash(k))
		if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
			t.Fatal(err)
		}
		testWrite(t, f, logo)
		psql(seed.GFN.Name, fmt.Sprintf("UPDATE gfn_site SET icon='%s' WHERE id=%d;", k, id))
	}
	psql(seed.GFN.Name, "UPDATE gfn_site SET icon=NULL WHERE id=6;")
	psql(seed.GFG.Name, `INSERT INTO gfg_game (id,name,name_en,info,info_en,appid) VALUES
	(5,'自动成员','Dynamic','说明','Synthetic',0),(6,'暂时失败','Transient','说明','Synthetic',0);
	INSERT INTO gfg_game_tag (game_id,tag_id,role) VALUES (5,1,'normal'),(5,3,'primary'),(6,3,'normal');
	INSERT INTO gfg_game_collection_tag (collection_id,tag_id) VALUES (1,3);
	UPDATE gfg_game SET links='[{"key":"itch","value":"https://fixture.itch.io/dynamic"}]' WHERE id=5;`)
	res, err := legacyseed.Run(ctx, "extract", seed)
	if err != nil {
		t.Fatal(err)
	}
	c := Config{SeedDir: filepath.Join(seed.Output, seed.Snapshot), Snapshot: seed.Snapshot, Backup: seed.BackupSHA,
		Content: res.ContentSHA, Deployment: deployment, Region: "CN", Environment: "development", Isolation: uuid.NewV7().String()}
	c.Database.Host = "127.0.0.1"
	c.Database.Port = port
	c.Database.Name = seed.TargetDB
	c.Database.User = "postgres"
	c.Database.Password = pgPassword
	psql("postgres", "COMMENT ON DATABASE "+c.Database.Name+" IS 'gofurry-d5-b1:"+c.Isolation+"';")
	db := platform.DatabaseConfig{Host: c.Database.Host, Port: port, Name: c.Database.Name, User: c.Database.User, Password: c.Database.Password, SSLMode: "disable", ConnectTimeout: 5 * time.Second, MaxConnections: 3}
	if err = database.Migrate(ctx, db, "../../migrations", "up", io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := legacyseed.OpenBundle(ctx, c.SeedDir, c.Snapshot, c.Backup, c.Content)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	built, err := build(ctx, b, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var hash string
	for _, row := range built.Items {
		if row.Key.String() == "gfn_site/1" && row.LogoIssue != "logo_approval_pending" {
			t.Fatal("fixture logo was not decoded")
		}
	}
	// The extractor can re-encode the original bytes. Read its final digest.
	index := testRead(t, filepath.Join(c.SeedDir, "assets", "site-logo-index.ndjson"))
	var first legacyseed.Logo
	if err = decode(bytes.SplitN(index, []byte("\n"), 2)[0], &first); err != nil || first.Status != "ready" {
		t.Fatal("fixture logo index is not ready")
	}
	hash = first.SHA
	return c, seed, psql, hash
}

type fixtureStore struct {
	mu       sync.Mutex
	objects  map[string][]byte
	puts     int
	requests int
	broken   bool
}

func setupStore(t *testing.T, c *Config, base string) *fixtureStore {
	t.Helper()
	s := &fixtureStore{objects: map[string][]byte{}}
	const bucket = "gofurry-d5b1-fixture"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests++
		name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/"+bucket+"/"), "/cdn/")
		if !strings.HasPrefix(name, "sites/logos/") {
			t.Error("object operation escaped Site Logo namespace")
			w.WriteHeader(400)
			return
		}
		raw, found := s.objects[name]
		if r.Method == http.MethodPut {
			if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Content-Type") != "image/webp" {
				t.Error("immutable WebP upload headers missing")
			}
			if found {
				w.WriteHeader(412)
				return
			}
			raw, _ = io.ReadAll(r.Body)
			if digest(raw) != r.Header.Get("X-Amz-Meta-Sha256") {
				t.Error("upload has wrong hash")
			}
			s.objects[name] = raw
			s.puts++
			return
		}
		if !found {
			w.WriteHeader(404)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/cdn/") {
			if r.Header.Get("Authorization") != "" {
				t.Error("private credentials sent to delivery")
			}
			if s.broken {
				w.WriteHeader(503)
				return
			}
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		w.Header().Set("X-Amz-Meta-Sha256", digest(raw))
		if r.Method == http.MethodGet {
			_, _ = w.Write(raw)
		}
	}))
	t.Cleanup(server.Close)
	ca := filepath.Join(base, "fixture-ca.pem")
	testWrite(t, ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	raw := fmt.Sprintf("storage:\n  endpoint: %s\n  public_base_url: %s/cdn\n  region: auto\n  bucket: %s\n  access_key_id: fixture\n  secret_access_key: fixture\n  ca_file: '%s'\n",
		server.URL, server.URL, bucket, strings.ReplaceAll(ca, "'", "''"))
	if yaml.Unmarshal([]byte(raw), c) != nil {
		t.Fatal("fixture store config invalid")
	}
	return s
}

func decide(op string, k key, value any) decision {
	return decision{Operation: op, Subject: &k, Reviewer: "fixture-owner", Reason: "Synthetic reviewed data", Evidence: "fixture:" + k.String(), Value: encode(value)}
}
func testDecisions(t *testing.T, c Config, decisions []decision) {
	t.Helper()
	testWrite(t, c.DecisionsFile, encode(decisionFile{1, c.Content, decisions}))
}

func execSQL(t *testing.T, p *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if _, err := p.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}
func expectInt(t *testing.T, p *pgxpool.Pool, ctx context.Context, want int, sql string, args ...any) {
	t.Helper()
	var actual int
	if err := p.QueryRow(ctx, sql, args...).Scan(&actual); err != nil || actual != want {
		t.Fatalf("count = %d, want %d (%v)", actual, want, err)
	}
}

func TestLegacyImportPostgreSQL18(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Minute)
	defer cancel()
	base := t.TempDir()
	c, seed, psql, logoHash := fixture(t, ctx, base)
	objects := setupStore(t, &c, base)
	c.DecisionsFile = filepath.Join(base, "decisions.json")
	pool, err := c.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := dbgen.New(pool)
	s := catalog.New(pool)
	ed := catalog.Editorial{Slug: "carefully-edited", Localizations: []catalog.Localization{{Locale: "en", Name: "Curated existing"}}}
	site, err := s.CreateSite(ctx, catalog.SiteInput{Editorial: ed, Addresses: []catalog.Address{{Kind: "primary", URL: "https://editor.example/"}}})
	if err != nil {
		t.Fatal(err)
	}
	site, err = s.ChangeSitePublication(ctx, site.PublicID, site.Revision, "publish", true, catalog.PublicationDecision{Actor: "fixture", Reason: "Test existing publication"})
	if err != nil {
		t.Fatal(err)
	}
	game, err := s.CreateGame(ctx, catalog.GameInput{Editorial: ed, Sources: []catalog.Source{{PlatformCode: "steam", ExternalKey: "100"}}})
	if err != nil {
		t.Fatal(err)
	}
	game, err = s.SetGameStatus(ctx, game.ID, game.Revision, catalog.Published)
	if err != nil {
		t.Fatal(err)
	}

	ds := []decision{
		decide("supply_slug", makeKey(sites, 2), "archived-site"),
		decide("select_address", makeKey(sites, 1), addressValue{Primary: &key{"gfn_target_tracking_periods", []int64{1}}}),
		decide("map_group", makeKey(groups, 1), groupValue{Category: "community", Tags: []string{"forum"}}),
		decide("resolve_relation", makeKey(groupMembers, 3), relationValue{Ignore: true}),
		decide("allow_publish", makeKey(sites, 1), publishValue{SFW: true}),
		decide("link_existing", makeKey(sites, 3), linkValue{site.ID, "site", site.PublicID, site.Revision}),
		decide("allow_logo", makeKey(sites, 1), logoHash),
		decide("allow_logo", makeKey(sites, 2), logoHash),
		decide("allow_logo", makeKey(sites, 4), logoHash),
		decide("select_source", makeKey(games, 1), []catalog.Source{{PlatformCode: "steam", ExternalKey: "20"}}),
		decide("select_source", makeKey(games, 2), []catalog.Source{}),
		decide("supply_slug", makeKey(games, 2), "adult-fixture"),
		decide("link_existing", makeKey(games, 4), linkValue{game.ID, "game", game.PublicID, game.Revision}),
		decide("select_source", makeKey(games, 5), []catalog.Source{{PlatformCode: "itch_io", ExternalKey: "fixture/dynamic"}}),
		decide("resolve_tag_code", makeKey(tags, 1), "adventure"),
		decide("resolve_tag_code", makeKey(tags, 3), "alternate-adventure"),
		decide("resolve_relation", makeKey(tagMembers, 999, 2), relationValue{Ignore: true}),
		decide("allow_publish", makeKey(games, 1), publishValue{SFW: true}),
		decide("allow_publish", makeKey(games, 5), publishValue{SFW: true}),
		decide("allow_publish", makeKey(games, 6), publishValue{SFW: true}),
		decide("allow_publish", makeKey(collections, 1), publishValue{RelationsApproved: true}),
	}
	testDecisions(t, c, ds)
	plan, report, err := Run(ctx, c, Options{Mode: "plan"})
	if err != nil || report.Status != "planned" {
		t.Fatalf("plan: %v %+v", err, report)
	}
	again, _, err := Run(ctx, c, Options{Mode: "plan"})
	if err != nil || !bytes.Equal(plan.Bytes(), again.Bytes()) {
		t.Fatal("dry run must be byte-deterministic for identical preimages")
	}
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM legacy_import_batches")
	if objects.requests != 0 {
		t.Fatal("plan accessed object storage")
	}
	planPath := filepath.Join(base, "plan.json")
	testWrite(t, planPath, plan.Bytes())
	apply := Options{Mode: "apply", PlanFile: planPath, PlanSHA: report.PlanSHA}
	verify := Options{Mode: "verify", PlanFile: planPath, PlanSHA: report.PlanSHA}
	if _, _, err = Run(ctx, c, verify); !errors.Is(err, ErrIncomplete) {
		t.Fatal("unapplied plan verified")
	}
	crossRegion := c
	crossRegion.Region = "INTL"
	if _, _, err = Run(ctx, crossRegion, apply); err == nil || errors.Is(err, ErrIncomplete) {
		t.Fatal("cross-region target accepted")
	}
	wrongMarker := c
	wrongMarker.Isolation = uuid.NewV7().String()
	if _, _, err = Run(ctx, wrongMarker, Options{Mode: "plan"}); err == nil {
		t.Fatal("wrong isolation marker accepted")
	}

	mutated := filepath.Join(c.SeedDir, "content", "sites.ndjson")
	saved := testRead(t, mutated)
	testWrite(t, mutated, append(bytes.Clone(saved), []byte("{}\n")...))
	if _, _, err = Run(ctx, c, apply); err == nil || errors.Is(err, ErrIncomplete) {
		t.Fatal("mutated seed was not rejected before apply")
	}
	testWrite(t, mutated, saved)

	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM legacy_import_batches")

	locked, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = locked.Exec(ctx, "SELECT pg_advisory_lock(1280760113,1)"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Run(ctx, c, apply); err == nil || errors.Is(err, ErrIncomplete) {
		t.Fatal("competing import entered a locked deploy scope")
	}
	_, err = locked.Exec(ctx, "SELECT pg_advisory_unlock(1280760113,1)")
	locked.Release()
	if err != nil {
		t.Fatal(err)
	}
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM legacy_import_batches")

	execSQL(t, pool, ctx, `CREATE FUNCTION fixture_public_id_collision() RETURNS trigger LANGUAGE plpgsql AS $fixture$
	BEGIN
	 IF current_setting('fixture.collided',true) IS DISTINCT FROM 'yes' THEN
	  PERFORM set_config('fixture.collided','yes',true);
	  SELECT public_id INTO NEW.public_id FROM games WHERE slug='carefully-edited';
	 END IF;
	 RETURN NEW;
	END $fixture$;
	CREATE TRIGGER fixture_public_id_collision BEFORE INSERT ON games FOR EACH ROW
	 WHEN (NEW.slug='dynamic') EXECUTE FUNCTION fixture_public_id_collision();
	CREATE FUNCTION fixture_reject_import() RETURNS trigger LANGUAGE plpgsql AS $fixture$
BEGIN RAISE EXCEPTION 'fixture transient failure'; END $fixture$;
CREATE TRIGGER fixture_game_failure BEFORE INSERT ON games FOR EACH ROW WHEN (NEW.slug='transient') EXECUTE FUNCTION fixture_reject_import();
CREATE TRIGGER fixture_logo_failure BEFORE UPDATE ON sites FOR EACH ROW
 WHEN (NEW.slug IN ('example','archived-site') AND NEW.logo_asset_key IS DISTINCT FROM OLD.logo_asset_key)
 EXECUTE FUNCTION fixture_reject_import();`)
	_, failed, err := Run(ctx, c, apply)
	if !errors.Is(err, ErrIncomplete) || failed.Counts["retry_pending"] != 3 {
		for _, p := range failed.Phases {
			if p.Issue != "" {
				t.Logf("phase %+v", p)
			}
		}
		t.Fatalf("partial result %s %+v: %v", failed.Status, failed.Counts, err)
	}
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM legacy_game_ids WHERE legacy_id=6")
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM sites WHERE slug='example' AND logo_asset_key IS NOT NULL")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM sites WHERE slug='archived-site' AND status='archived' AND logo_asset_key IS NULL")
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM legacy_import_receipts WHERE source_key IN ('gfn_site/1','gfg_game/6') AND ((source_key='gfn_site/1' AND phase='logo') OR source_key='gfg_game/6')")
	if objects.puts != 1 || failed.Assets["uploaded_objects"] != 1 {
		t.Fatal("shared verified object should be uploaded once despite binding failure")
	}
	var beforeSite, beforeGame targetState
	siteMap, err := mapping(ctx, q, makeKey(sites, 1))
	if err != nil {
		t.Fatal(err)
	}
	beforeSite, err = readTarget(ctx, q, makeKey(sites, 1), siteMap.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	gameMap, err := mapping(ctx, q, makeKey(games, 1))
	if err != nil {
		t.Fatal(err)
	}
	beforeGame, err = readTarget(ctx, q, makeKey(games, 1), gameMap.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeSite.ID == beforeGame.ID || uuidVersion(beforeSite.ID) != 7 || uuidVersion(beforeGame.ID) != 7 ||
		!catalog.ValidPublicID(beforeSite.PublicID) || !catalog.ValidPublicID(beforeGame.PublicID) {
		t.Fatal("D1 independent typed UUIDv7/Public ID invariant broken")
	}

	execSQL(t, pool, ctx, "DROP TRIGGER fixture_game_failure ON games; DROP TRIGGER fixture_logo_failure ON sites;")
	_, success, err := Run(ctx, c, apply)
	if err != nil || success.Status != "complete" {
		for _, p := range success.Phases {
			if p.Issue != "" {
				t.Logf("phase %+v", p)
			}
		}
		t.Fatalf("recovery %s %+v: %v", success.Status, success.Counts, err)
	}
	if success.Rows == nil || success.SourceRows == 0 || objects.puts != 1 || success.Assets["verified_references"] != 3 {
		t.Fatalf("asset and source reconciliation %+v", success.Assets)
	}
	expectInt(t, pool, ctx, 3, "SELECT count(*) FROM sites WHERE logo_asset_key=$1", logoKey(logoHash))
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM sites WHERE slug='archived-site' AND status='archived' AND content_rating='adult' AND first_published_at IS NULL")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM games WHERE slug='adult-fixture' AND publication_blocked AND status='draft'")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM games WHERE slug='indie' AND NOT publication_blocked AND status='draft'")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM sites WHERE slug='example' AND primary_category='community' AND navigation_tags=ARRAY['forum'] AND status='published'")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM site_addresses WHERE site_id=$1 AND url='https://www.example.com' AND active", beforeSite.ID)
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM site_addresses WHERE url LIKE '%old.example.com%'")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM game_sources WHERE platform_code='steam' AND external_key='20' AND game_id=$1 AND active", beforeGame.ID)
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM game_sources WHERE platform_code='itch_io' AND external_key='fixture/dynamic' AND source_url='https://fixture.itch.io/dynamic' AND active")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM games WHERE id=$1 AND revision=$2 AND status='published'", game.ID, game.Revision)
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM sites WHERE id=$1 AND revision=$2 AND status='published'", site.ID, site.Revision)
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM game_localizations WHERE game_id=$1 AND name='Curated existing'", game.ID)
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM legacy_game_ids WHERE legacy_id=4 AND target_id=$1 AND origin='linked' AND verified_by='fixture-owner'", game.ID)
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM game_editorial_tag_members WHERE game_id=$1 AND role='primary'", beforeGame.ID)

	coll, err := mapping(ctx, q, makeKey(collections, 1))
	if err != nil {
		t.Fatal(err)
	}
	counts, err := q.LegacyCollectionCounts(ctx, coll.TargetID)
	if err != nil || counts.ManualCount != 2 || counts.RuleCount != 3 || counts.ExclusionCount != 2 ||
		counts.ManualExcludedCount != 1 || counts.EffectiveCount != 3 || counts.PublicCount != 2 {
		t.Fatalf("collection reconciliation %+v %v", counts, err)
	}
	page, err := s.PublicGameCollection(ctx, catalog.ChannelMarket{Region: "CN"}, coll.PublicIDAtLink, catalog.ChannelCriteria{Locale: "en", Limit: 1})
	if err != nil || len(page.Members.Items) != 1 || page.Members.After == "" {
		t.Fatalf("public hybrid collection: %+v %v", page, err)
	}
	next, err := s.PublicGameCollection(ctx, catalog.ChannelMarket{Region: "CN"}, coll.PublicIDAtLink,
		catalog.ChannelCriteria{Locale: "en", Limit: 1, After: page.Members.After})
	if err != nil || len(next.Members.Items) != 1 || next.Members.Items[0].PublicID == page.Members.Items[0].PublicID {
		t.Fatal("editorial cursor lost or repeated an automatic member")
	}

	beforeReplay, err := readTarget(ctx, q, makeKey(sites, 1), beforeSite.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, replay, err := Run(ctx, c, apply)
	if err != nil || replay.Counts["applied"] != 0 || objects.puts != 1 {
		t.Fatalf("same plan replay: %+v %v", replay.Counts, err)
	}
	afterReplay, err := readTarget(ctx, q, makeKey(sites, 1), beforeSite.ID)
	if err != nil || afterReplay != beforeReplay {
		t.Fatal("replay changed durable Site, revision or Public ID")
	}

	if _, r, err := Run(ctx, c, verify); err != nil || r.Status != "complete" {
		t.Fatalf("read-back: %s %+v %v", r.Status, r.Counts, err)
	}
	objects.mu.Lock()
	objects.broken = true
	objects.mu.Unlock()
	if _, r, err := Run(ctx, c, verify); !errors.Is(err, ErrIncomplete) || r.Assets["pending_references"] != 3 || objects.puts != 1 {
		t.Fatalf("CDN failure should fail verification without re-upload: %+v %v", r.Assets, err)
	}
	objects.mu.Lock()
	objects.broken = false
	objects.mu.Unlock()

	seed.Snapshot = "d5b1-second"
	seed.BackupSHA = strings.Repeat("b", 64)
	extracted, err := legacyseed.Run(ctx, "extract", seed)
	if err != nil {
		t.Fatal(err)
	}
	c2 := c
	c2.Snapshot, c2.Backup, c2.Content = seed.Snapshot, seed.BackupSHA, extracted.ContentSHA
	c2.SeedDir = filepath.Join(seed.Output, seed.Snapshot)
	c2.DecisionsFile = filepath.Join(base, "decisions-second.json")
	testDecisions(t, c2, ds)
	second, r, err := Run(ctx, c2, Options{Mode: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(base, "plan-second.json")
	testWrite(t, secondPath, second.Bytes())
	if _, r, err = Run(ctx, c2, Options{"apply", secondPath, r.PlanSHA}); err != nil || r.Counts["applied"] != 0 || objects.puts != 1 {
		t.Fatalf("different snapshot replay: %+v %v", r.Counts, err)
	}

	// Removing a logo through the ordinary editor is a relevant preimage
	// change. Neither a replay nor a new plan may put it back.
	if _, err = s.SetSiteAsset(ctx, beforeSite.PublicID, beforeReplay.Revision, "logo", ""); err != nil {
		t.Fatal(err)
	}
	if _, r, err = Run(ctx, c, apply); !errors.Is(err, ErrIncomplete) || r.Counts["conflict"] == 0 {
		t.Fatalf("editorial drift was not reported: %+v %v", r.Counts, err)
	}
	expectInt(t, pool, ctx, 0, "SELECT count(*) FROM sites WHERE id=$1 AND logo_asset_key IS NOT NULL", beforeSite.ID)

	// Verify the actual shell entry point and that stdout stays aggregate-only.
	binary := filepath.Join(base, "legacy-import")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/legacy-import")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CLI build: %v %s", err, out)
	}
	configPath := filepath.Join(base, "import.yml")
	yamlBytes, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	testWrite(t, configPath, yamlBytes)
	cmd = exec.CommandContext(ctx, binary, "verify", "--config", configPath, "--plan", planPath,
		"--plan-sha256", apply.PlanSHA, "--report", filepath.Join(base, "cli-report.json"))
	out, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(out, []byte(`"status":"incomplete"`)) || bytes.Contains(out, []byte(beforeSite.PublicID)) ||
		bytes.Contains(out, []byte(pgPassword)) {
		t.Fatalf("CLI did not report a redacted failure: %q", out)
	}

	// Rule-result changes invalidate an existing public cursor. Restoring
	// the category resumes dynamic matching without rewriting manual rows.
	execSQL(t, pool, ctx, "UPDATE game_editorial_tag_categories SET archived_at=clock_timestamp() WHERE code='genre'")
	expectInt(t, pool, ctx, 1, "SELECT count(*) FROM game_collection_effective_members WHERE collection_id=$1", coll.TargetID)
	_, err = s.PublicGameCollection(ctx, catalog.ChannelMarket{Region: "CN"}, coll.PublicIDAtLink,
		catalog.ChannelCriteria{Locale: "en", Limit: 1, After: page.Members.After})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("old dynamic collection cursor did not invalidate: %v", err)
	}
	execSQL(t, pool, ctx, "UPDATE game_editorial_tag_categories SET archived_at=NULL WHERE code='genre'")
	expectInt(t, pool, ctx, 3, "SELECT count(*) FROM game_collection_effective_members WHERE collection_id=$1", coll.TargetID)
	expectInt(t, pool, ctx, 2, "SELECT count(*) FROM game_collection_members WHERE collection_id=$1", coll.TargetID)

	testCollectionCapacity(t, ctx, c, seed, psql, pool, base, ds)
	t.Logf("isolated PG 18: %d source rows, first apply retry=%d, replay reused=%d, unique immutable PUTs=%d",
		success.SourceRows, failed.Counts["retry_pending"], replay.Counts["verified_existing"], objects.puts)
}
