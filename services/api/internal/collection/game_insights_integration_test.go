//go:build integration

package collection

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func checkGameProfileFromHarvest(t *testing.T, editor *catalog.Service, id string) {
	t.Helper()
	ctx := t.Context()
	market := catalog.ChannelMarket{Region: "CN", Egress: "cn"}
	p, err := editor.PublicGameProfile(ctx, market, id, "en")
	must(t, err)
	if p.Localizations[0].Summary != "Editorial text" || len(p.Sources) != 2 || len(p.News.Items) != 1 {
		t.Fatalf("wrong profile projection from Harvest: %+v", p)
	}
	steam := p.Sources[1]
	if steam.ExternalKey != "42" || steam.Details == nil || steam.Details.ReleasePrecision != "quarter" || steam.Details.ReleaseDay != "" ||
		steam.Media == nil || len(steam.Media.Screenshots) != 1 || len(steam.Media.Movies) != 1 || steam.Players == nil || steam.Players.Count != 0 ||
		len(steam.Quotes) != 2 || steam.Quotes[0].Currency != "CNY" || steam.Quotes[0].State != "free" ||
		steam.Quotes[1].Currency != "USD" || steam.Quotes[1].State != "zero_price" ||
		p.News.Items[0].URL != "https://steamcommunity.com/games/42/announcements/detail/123456" ||
		len(p.Sources[0].Quotes) != 0 || p.Sources[0].Players != nil {
		t.Fatalf("missing or misattributed scoped facts: %+v", p.Sources)
	}
	for country, expected := range map[string]string{"cn": "free", "us": "zero_price", "hk": ""} {
		data, err := editor.PublicGameInsights(ctx, market, id, "42", country, "en", 30)
		must(t, err)
		if len(data.PlayerDays) != 1 || data.PlayerDays[0].Maximum != 0 || data.PlayerDays[0].Average != 0 ||
			len(data.PriceChanges) != 0 || !slices.Equal(data.Ranges, []int32{7, 30}) || len(data.News) != 1 {
			t.Fatalf("unexpected initial series %s: %+v", country, data)
		}
		if expected == "" && len(data.PriceDays) > 0 || expected != "" && (len(data.PriceDays) != 1 || data.PriceDays[0].State != expected) {
			t.Fatalf("price crossed stores: %s %+v", country, data.PriceDays)
		}
	}
}

func checkGameHistoricEvidence(t *testing.T, pool *pgxpool.Pool, editor *catalog.Service, id string, source uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	market := catalog.ChannelMarket{Region: "CN", Egress: "cn"}
	before, err := editor.PublicGameInsights(ctx, market, id, "42", "us", "en", 30)
	must(t, err)
	if len(before.PriceDays) != 1 || len(before.PriceChanges) != 1 || before.PriceDays[0].Final == nil ||
		*before.PriceDays[0].Final != 1499 || before.PriceChanges[0].ObservationID != before.PriceDays[0].ObservationID {
		t.Fatalf("expected one real USD price transition: %+v", before)
	}
	// Re-applying the active snapshot price in concurrent transactions leaves
	// the same observation rows and creates no changes.
	q := dbgen.New(pool)
	active, err := q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	key := dbgen.SaveGamePriceObservationsParams{Generation: *active.Generation, SourceID: &source}
	fingerprint := func() int64 {
		return scalar[int64](t, pool, "SELECT sum(xmin::text::bigint)::bigint FROM game_price_observations")
	}
	first := fingerprint()
	var group sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		group.Go(func() { errs <- q.SaveGamePriceObservations(ctx, key) })
	}
	group.Wait()
	close(errs)
	for err := range errs {
		must(t, err)
	}
	if fingerprint() != first {
		t.Fatal("concurrent price replay rewrote retained observations")
	}
	// Shift only the disposable evidence to simulate a full year without a worker.
	// The most recent sample stays within the inclusive 365-day query window.
	for _, days := range []int32{31, 181, 364} {
		execSQL(t, pool, `UPDATE game_player_daily d SET utc_day=(now() AT TIME ZONE 'UTC')::date-$1::integer,
 samples=(SELECT jsonb_object_agg(key,jsonb_set(value,'{1}',to_jsonb(now()-make_interval(days=>$1::integer))))
 FROM jsonb_each(d.samples))`, days)
		execSQL(t, pool, `UPDATE game_price_observations SET utc_day=(now() AT TIME ZONE 'UTC')::date-$1::integer,
 observed_at=observed_at-make_interval(days=>CASE WHEN $1=31 THEN 31 WHEN $1=181 THEN 150 ELSE 183 END)`, days)
		execSQL(t, pool, `UPDATE game_evidence_players SET bucket=date_bin('30 minutes',now()-make_interval(days=>$1::integer),'2000-01-01 UTC'),
 observed_at=now()-make_interval(days=>$1::integer)`, days)
		must(t, q.PruneGameEvidencePlayers(ctx))
		count(t, pool, 0, "SELECT count(*) FROM game_evidence_players")
		out, err := editor.PublicGameInsights(ctx, market, id, "42", "us", "en", 365)
		must(t, err)
		if len(out.PlayerDays) != 1 || out.PlayerDays[0].Maximum != 0 || len(out.PriceDays) != 1 || len(out.PriceChanges) != 1 {
			t.Fatalf("retention removed or invented history at day %d: %+v", days+1, out)
		}
		p, err := editor.PublicGameProfile(ctx, market, id, "en")
		must(t, err)
		if p.PublicID != id || p.Localizations[0].Summary != "Editorial text" || p.CoverURL != "" || len(p.Sources[1].Quotes) != 0 ||
			p.Sources[1].Players != nil || p.Sources[1].Active {
			t.Fatalf("retention or source closure changed the published work: %+v", p)
		}
	}
	revision := scalar[int64](t, pool, "SELECT revision FROM games WHERE public_id=$1", id)
	_, err = changeFixtureGame(t, editor, id, revision, "archive")
	must(t, err)
	_, err = editor.PublicGameInsights(ctx, market, id, "42", "us", "en", 365)
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("archived work exposed historical facts: %v", err)
	}
}
