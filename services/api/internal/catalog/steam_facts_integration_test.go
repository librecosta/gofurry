package catalog

import (
	"flag"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var steamFactsYAML = flag.String("steam-facts-yaml", "", "migrated disposable loopback PostgreSQL YAML for Steam facts integration")

func TestSteamFactsPostgreSQL(t *testing.T) {
	if *steamFactsYAML == "" {
		t.Skip("explicit isolated PostgreSQL YAML required")
	}
	cfg, err := platform.LoadConfig(*steamFactsYAML)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database == nil || cfg.Database.Host != "127.0.0.1" || !strings.HasSuffix(cfg.Database.Name, "_test") {
		t.Fatal("expected an isolated loopback test database")
	}
	ctx := t.Context()
	pool, err := database.Open(ctx, *cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bounds, err := q.SteamChangeWorkBounds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	yesterday := bounds.LastDay.Time
	start := yesterday.AddDate(0, 0, -3)
	publicID := "7zzzzzzzzzzzzzz0"
	game, source, remote, first, next := uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	exec("INSERT INTO games(id,public_id,slug) VALUES($1,$2,'r4-isolated-facts')", game, publicID)
	exec("INSERT INTO game_sources(id,game_id,platform_code,external_key,source_url) VALUES($1,$2,'steam','4200000891','https://store.steampowered.com/app/4200000891/')", source, game)
	exec("INSERT INTO collection_sources(id,namespace,provider,resource_type,external_key,target_id) VALUES($1,'r4_native_test','steam','app','4200000891',$2)", remote, uuid.NewV7())
	exec("INSERT INTO collection_bindings(id,source_id,resource_type,game_id,game_source_id,purpose,effective_from,effective_until) VALUES($1,$2,'app',$3,$4,'steam',$5,$6)",
		first, remote, game, source, start.AddDate(0, 0, -1), start.AddDate(0, 0, 2))
	exec("INSERT INTO collection_bindings(id,source_id,resource_type,game_id,game_source_id,purpose,effective_from) VALUES($1,$2,'app',$3,$4,'steam',$5)",
		next, remote, game, source, start.AddDate(0, 0, 2))
	put := func(binding uuid.UUID, day time.Time, kind, value, market, egress string) {
		t.Helper()
		stamp := pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true}
		if err := q.PutSteamDailyFact(ctx, dbgen.PutSteamDailyFactParams{BindingID: binding, Kind: kind, Value: []byte(value), Market: market,
			Locale: "en", Egress: egress, ObservedAt: stamp, ConfirmedAt: stamp, ObservationID: uuid.NewV7()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"free", "windows", "mac", "linux"} {
		put(first, start, kind, "false", "us", "cn")
		put(first, start.AddDate(0, 0, 1), kind, "true", "us", "cn")
		put(next, start.AddDate(0, 0, 2), kind, "false", "us", "cn")
		put(next, start.AddDate(0, 0, 3), kind, "true", "hk", "cn")
		put(next, start.AddDate(0, 0, 3), kind, "true", "us", "intl")
	}
	put(first, start, "release", `{"state":"upcoming","precision":"quarter","window_start":"2027-04-01","window_end":"2027-06-30","raw_text":"Q2 2027"}`, "us", "cn")
	put(first, start.AddDate(0, 0, 1), "release", `{"state":"available","precision":"day","raw_text":"May 2027"}`, "us", "cn")
	for i, final := range []int64{100, 75} {
		date := start.AddDate(0, 0, i)
		exec("INSERT INTO game_price_observations(game_source_id,binding_id,observation_id,egress_region,store_country,currency,locale,utc_day,observed_at,state,original,final,discount,confirmed_at) VALUES($1,$2,$3,'cn','us','USD','en',$4,$5,'priced',100,$6,$7,$5)",
			source, first, uuid.NewV7(), pgtype.Date{Time: date, Valid: true}, date.Add(time.Hour), final, 100-final)
	}
	exec(`INSERT INTO game_player_daily(game_source_id,egress_region,utc_day,samples)
VALUES($1,'cn',$2,jsonb_build_object('2',jsonb_build_array(0,$3::timestamptz,$4::uuid,$5::uuid),
 '3',jsonb_build_array(20,$3::timestamptz,$6::uuid,$5::uuid),
 '4',jsonb_build_array(5,$3::timestamptz,$7::uuid,$8::uuid)))`, source,
		pgtype.Date{Time: yesterday, Valid: true}, yesterday.Add(time.Hour), uuid.NewV7(), next, uuid.NewV7(), uuid.NewV7(), first)
	player, err := q.SteamFactsPlayerQuality(ctx, dbgen.SteamFactsPlayerQualityParams{ID: next, EgressRegion: "cn"})
	if err != nil || player.Days != 1 || player.Samples != 2 {
		t.Fatalf("player quality %+v %v", player, err)
	}
	player, err = q.SteamFactsPlayerQuality(ctx, dbgen.SteamFactsPlayerQualityParams{ID: next, EgressRegion: "intl"})
	if err != nil || player.Days != 0 || player.Samples != 0 {
		t.Fatalf("egress isolation %+v %v", player, err)
	}
	if err := q.PutSteamFirstObserved(ctx, dbgen.PutSteamFirstObservedParams{BindingID: next,
		ObservedAt: pgtype.Timestamptz{Time: yesterday.Add(time.Hour), Valid: true}, ObservationID: uuid.NewV7()}); err != nil {
		t.Fatal(err)
	}
	review := SteamFirstAvailableEdit{TrackingID: next, WindowStart: "2020-01-01", WindowEnd: "2020-01-01",
		EvidenceURL: "https://store.steampowered.com/news/app/4200000891/view/10", Reason: "Isolated attestation fixture"}
	if got, err := RecordSteamFirstAvailable(ctx, q, publicID, review, uuid.NewV7()); err != nil || got.Revision != 1 {
		t.Fatalf("initial review %+v %v", got, err)
	}
	if _, err := RecordSteamFirstAvailable(ctx, q, publicID, review, uuid.NewV7()); err != ErrConflict {
		t.Fatalf("stale reply %v", err)
	}
	review.Revision = 1
	review.WindowStart = "2019-12-30"
	if got, err := RecordSteamFirstAvailable(ctx, q, publicID, review, uuid.NewV7()); err != nil || got.Revision != 2 {
		t.Fatalf("correction %+v %v", got, err)
	}
	day := pgtype.Date{Time: start.AddDate(0, 0, 1), Valid: true}
	n, err := projectSteamChanges(ctx, q, first, day, true)
	if err != nil || n != 7 {
		t.Fatalf("dry-run n=%d err=%v", n, err)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := tx.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("SELECT count(*) FROM steam_first_available_reviews WHERE binding_id=$1", next) != 2 {
		t.Fatal("correction overwrote evidence")
	}
	if count("SELECT count(*) FROM steam_game_changes WHERE binding_id=$1", first) != 0 {
		t.Fatal("dry run wrote rows")
	}
	n, err = projectSteamChanges(ctx, q, first, day, false)
	if err != nil || n != 7 {
		t.Fatalf("project n=%d err=%v", n, err)
	}
	var stable string
	if err := tx.QueryRow(ctx, "SELECT string_agg(id::text,',' ORDER BY id) FROM steam_game_changes WHERE binding_id=$1", first).Scan(&stable); err != nil {
		t.Fatal(err)
	}
	n, err = projectSteamChanges(ctx, q, first, day, false)
	if err != nil || n != 7 {
		t.Fatalf("rebuild n=%d err=%v", n, err)
	}
	var after string
	if err := tx.QueryRow(ctx, "SELECT string_agg(id::text,',' ORDER BY id) FROM steam_game_changes WHERE binding_id=$1", first).Scan(&after); err != nil || stable != after {
		t.Fatalf("changed event identities: %v", err)
	}
	for _, d := range []time.Time{start.AddDate(0, 0, 2), yesterday} {
		n, err = projectSteamChanges(ctx, q, next, pgtype.Date{Time: d, Valid: true}, false)
		if err != nil || n != 0 {
			t.Fatalf("identity/scope change produced n=%d err=%v", n, err)
		}
	}
	out, err := steamChanges(ctx, q, publicID, "", SteamChangeFilter{Market: "us"}, false)
	if err != nil || len(out.Items) != 7 {
		t.Fatalf("private page %d %v", len(out.Items), err)
	}
	out, err = steamChanges(ctx, q, publicID, "cn", SteamChangeFilter{Market: "us"}, true)
	if err != nil || len(out.Items) != 0 {
		t.Fatalf("draft exposed: %d %v", len(out.Items), err)
	}
	exec("INSERT INTO game_price_observations(game_source_id,binding_id,observation_id,egress_region,store_country,currency,locale,utc_day,observed_at,state,original,final,discount,confirmed_at,output_schema) VALUES($1,$2,$3,'cn','us','EUR','en',$4,$5,'priced',100,90,10,$5,3)",
		source, first, uuid.NewV7(), day, day.Time.Add(2*time.Hour))
	n, err = projectSteamChanges(ctx, q, first, day, false)
	if err != nil || n != 6 {
		t.Fatalf("currency and non-price transitions=%d err=%v", n, err)
	}
	if count("SELECT count(*) FROM steam_game_changes WHERE binding_id=$1 AND detector='game_price_transition' AND code='game_price_currency_changed'", first) != 1 {
		t.Fatal("currency transition missing or stale arithmetic events survived")
	}
}
