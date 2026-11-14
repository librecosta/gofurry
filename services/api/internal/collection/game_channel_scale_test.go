//go:build integration

package collection

import (
	"errors"
	"io"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
)

func TestGameChannelScale(t *testing.T) {
	ctx := t.Context()
	cfg, _ := fixtureDB(t, "game_channel")
	must(t, database.Migrate(ctx, cfg, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, cfg)
	must(t, err)
	defer pool.Close()
	editor := catalog.New(pool)
	market := catalog.ChannelMarket{Region: "CN", Egress: "cn"}
	prior := 0
	for _, size := range []int{100, 1000, 10000} {
		execSQL(t, pool, `WITH rows AS (
   INSERT INTO games(id,public_id,slug,status,first_published_at,published_at)
   SELECT uuidv7(),'3'||lpad(n::text,15,'0'),'scale-'||n,'published',
    now()-make_interval(secs=>n),now()-make_interval(secs=>n)
   FROM generate_series($1::int,$2::int) n RETURNING id,public_id)
   INSERT INTO game_localizations SELECT id,'en','Scale Game '||public_id,'Discovered catalog fixture' FROM rows`, prior+1, size)
		execSQL(t, pool, `INSERT INTO game_sources(id,game_id,platform_code,external_key,source_url)
   SELECT uuidv7(),g.id,'steam',(3200000000+right(g.public_id,6)::bigint)::text,
     'https://store.steampowered.com/app/'||(3200000000+right(g.public_id,6)::bigint)||'/'
   FROM games g WHERE right(g.public_id,6)::int BETWEEN $1::int AND $2::int`, prior+1, size)
		execSQL(t, pool, `INSERT INTO game_source_metadata
   (game_source_id,locale,store_country,egress_region,section,binding_id,scope_key,epoch,position,event_at,state,output_schema,
   ruleset,observation_id,content_hash,observed_at,confirmed_at,payload)
   SELECT id,'en','us','cn','details',uuidv7(),repeat('a',64),uuidv7(),1,now(),'present',1,'gofurry-steam-1',
    uuidv7(),repeat('b',64),now(),now(),'{"name":"Scale fixture","developers":["Burrow Studio"],"genres":[{"id":"25"}],
     "platforms":{"windows":true},"release":{"coming_soon":true,"date_text":"Q3 2027"}}'
   FROM game_sources WHERE external_key::bigint BETWEEN 3200000000+$1::int AND 3200000000+$2::int`, prior+1, size)
		execSQL(t, pool, "ANALYZE games; ANALYZE game_sources; ANALYZE game_localizations; ANALYZE game_channel_steam")
		heading := catalog.Editorial{Slug: "scale", Localizations: []catalog.Localization{{Locale: "en", Name: "Scale Collection"}}}
		collection, err := editor.CreateGameCollection(ctx, heading)
		must(t, err)
		var ids []string
		must(t, pool.QueryRow(ctx, "SELECT array_agg(public_id ORDER BY public_id) FROM (SELECT public_id FROM games ORDER BY public_id LIMIT 96) first").Scan(&ids))
		collection, err = editor.SetGameCollectionMembers(ctx, collection.PublicID, collection.Revision, ids)
		must(t, err)
		collection, err = editor.SetGameCollectionStatus(ctx, collection.PublicID, collection.Revision, catalog.Published)
		must(t, err)
		_, err = editor.SetGameChannelFeature(ctx, "collection_1", collection.PublicID, int64(slices.Index([]int{100, 1000, 10000}, size)+1))
		must(t, err)
		measure := func(name string, check func()) {
			t.Helper()
			check() // warm prepared plans and pools
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			samples := make([]time.Duration, 0, 12)
			for range 12 {
				start := time.Now()
				check()
				samples = append(samples, time.Since(start))
			}
			runtime.ReadMemStats(&after)
			slices.Sort(samples)
			t.Logf("games=%d %s p50=%s p95=%s Go_alloc/op=%d B", size, name, samples[5], samples[11], (after.TotalAlloc-before.TotalAlloc)/12)
		}
		measure("directory first+next", func() {
			criteria := catalog.ChannelCriteria{Locale: "en"}
			first, err := editor.GameDirectory(ctx, market, criteria)
			must(t, err)
			criteria.After = first.After
			next, err := editor.GameDirectory(ctx, market, criteria)
			must(t, err)
			if len(first.Items) != 24 || len(next.Items) != 24 || first.After == "" || first.Items[23].PublicID == next.Items[0].PublicID {
				t.Fatal("scale keyset broken")
			}
		})
		measure("directory developer/genre/name", func() {
			result, err := editor.GameDirectory(ctx, market, catalog.ChannelCriteria{Locale: "en", Keyword: "burrow studio", Genres: []string{"adventure"}, Sort: "name_asc"})
			must(t, err)
			if len(result.Items) != 24 || result.After == "" {
				t.Fatal("scale filters broken")
			}
		})
		measure("homepage", func() {
			home, err := editor.GameHome(ctx, market, "en")
			must(t, err)
			if len(home.Recent) != 8 || len(home.Upcoming) != 4 || len(home.Collections) != 1 {
				t.Fatalf("wrong lane bounds: %+v", home)
			}
			for _, genre := range home.Genres {
				if genre.Code == "adventure" && genre.Count != int64(size) {
					t.Fatal("genre count drift")
				}
			}
		})
		measure("collection first+next", func() {
			criteria := catalog.ChannelCriteria{Locale: "en", Sort: "timeline"}
			first, err := editor.PublicGameCollection(ctx, market, collection.PublicID, criteria)
			must(t, err)
			criteria.After = first.Members.After
			next, err := editor.PublicGameCollection(ctx, market, collection.PublicID, criteria)
			must(t, err)
			if len(first.Members.Items) != 24 || len(next.Members.Items) != 24 || criteria.After == "" {
				t.Fatal("collection member keyset broken")
			}
		})
		var bytes int64
		must(t, pool.QueryRow(ctx, "SELECT pg_total_relation_size('game_channel_steam')+pg_total_relation_size('game_collections')+pg_total_relation_size('game_collection_members')+pg_relation_size('games_channel_added')").Scan(&bytes))
		t.Logf("games=%d discovery/collection relations+indexes=%d B", size, bytes)
		prior = size
	}
}

func TestGameCollectionLifecycle(t *testing.T) {
	ctx := t.Context()
	cfg, _ := fixtureDB(t, "game_collection")
	must(t, database.Migrate(ctx, cfg, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, cfg)
	must(t, err)
	defer pool.Close()
	s := catalog.New(pool)
	market := catalog.ChannelMarket{Region: "CN", Egress: "cn"}
	criteria := catalog.ChannelCriteria{Locale: "en"}
	createGame := func(name string) string {
		row, err := s.CreateGame(ctx, catalog.GameInput{Editorial: catalog.Editorial{Localizations: []catalog.Localization{{Locale: "en", Name: name}}}})
		must(t, err)
		row, err = s.SetGameStatus(ctx, row.ID, row.Revision, catalog.Published)
		must(t, err)
		return row.PublicID
	}
	a, b := createGame("Wolf"), createGame("Fox")
	editorial := catalog.Editorial{Slug: "anthology", Localizations: []catalog.Localization{{Locale: "en", Name: "Anthology"}, {Locale: "zh", Name: "文集"}}}
	group, err := s.CreateGameCollection(ctx, editorial)
	must(t, err)
	_, err = s.PublicGameCollection(ctx, market, group.PublicID, criteria)
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("draft publicly visible: %v", err)
	}
	group, err = s.SetGameCollectionMembers(ctx, group.PublicID, group.Revision, []string{a, b})
	must(t, err)
	group, err = s.SetGameCollectionStatus(ctx, group.PublicID, group.Revision, catalog.Published)
	must(t, err)
	stamp := *group.FirstPublishedAt
	first, err := s.PublicGameCollection(ctx, market, group.PublicID, catalog.ChannelCriteria{Locale: "en", Limit: 1})
	must(t, err)
	edited, err := s.SetGameCollectionMembers(ctx, group.PublicID, group.Revision, []string{b, a})
	must(t, err)
	_, err = s.SetGameCollectionMembers(ctx, group.PublicID, group.Revision, []string{a})
	if !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("stale edit accepted: %v", err)
	}
	_, err = s.SetGameCollectionMembers(ctx, group.PublicID, edited.Revision, []string{b, "ffffffffffffffff"})
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("invalid member accepted: %v", err)
	}
	existing, err := s.InspectGameCollection(ctx, group.PublicID)
	must(t, err)
	if !slices.Equal(existing.Members, []string{b, a}) || existing.Revision != edited.Revision {
		t.Fatal("failed edit tore collection")
	}
	_, err = s.PublicGameCollection(ctx, market, group.PublicID, catalog.ChannelCriteria{Locale: "en", Limit: 1, After: first.Members.After})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("reordered collection accepted an old cursor: %v", err)
	}
	group, err = s.SetGameCollectionStatus(ctx, group.PublicID, existing.Revision, catalog.Archived)
	must(t, err)
	_, err = s.PublicGameCollection(ctx, market, group.PublicID, criteria)
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("archived collection remained public")
	}
	group, err = s.SetGameCollectionStatus(ctx, group.PublicID, group.Revision, catalog.Published)
	must(t, err)
	if !group.FirstPublishedAt.Equal(stamp) || !slices.Equal(group.Members, []string{b, a}) {
		t.Fatal("republication changed identity or members")
	}
	// Membership is independently managed. A second collection can reuse either work.
	other, err := s.CreateGameCollection(ctx, catalog.Editorial{Slug: "companion", Localizations: []catalog.Localization{{Locale: "en", Name: "Companion"}}})
	must(t, err)
	other, err = s.SetGameCollectionMembers(ctx, other.PublicID, other.Revision, []string{a})
	must(t, err)
	_, err = s.SetGameCollectionStatus(ctx, other.PublicID, other.Revision, catalog.Published)
	must(t, err)
	count(t, pool, 2, "SELECT count(*) FROM game_collection_members m JOIN games g ON g.id=m.game_id WHERE g.public_id=$1", a)
	t.Logf("collection %s retained its first publication; revision=%d", group.PublicID, group.Revision)
}
