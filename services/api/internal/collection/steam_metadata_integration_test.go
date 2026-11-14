//go:build integration

package collection

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func checkSteamReadModel(t *testing.T, editor *catalog.Service, id, unrelated string) {
	t.Helper()
	p, err := editor.ReadGamePresentation(t.Context(), id, "cn")
	must(t, err)
	if len(p.Localizations) != 1 || p.Localizations[0].Name != "Curated name" || len(p.Approved) != 0 || len(p.Sources) != 2 {
		t.Fatalf("manual work presentation changed: %+v", p)
	}
	steam := p.Sources[1]
	if p.Sources[0].PlatformCode != "itch_io" || len(p.Sources[0].Prices) != 0 || p.Sources[0].Players != nil ||
		steam.PlatformCode != "steam" || steam.ExternalKey != "42" || len(steam.Metadata) != 12 || len(steam.Prices) != 4 ||
		len(steam.News) != 1 || steam.News[0].Item.GID != "123456" || steam.Players == nil || steam.Players.Count != 0 {
		t.Fatalf("scoped Steam observations not separated from the other platform: %+v", p.Sources)
	}
	for _, item := range steam.Metadata {
		if item.State != "observed" || item.ObservationID == uuid.Nil() || item.TargetID == nil ||
			item.Namespace == nil || *item.Namespace != fixtureStream || item.Ruleset != steamProjectionRules ||
			item.ConfirmedAt.IsZero() || !digest.MatchString(item.ContentHash) || !digest.MatchString(item.ScopeKey) {
			t.Fatalf("missing observation attribution: %+v", item)
		}
		if item.Section == "media" {
			m := item.Media
			if m == nil || len(m.Screenshots) != 1 || len(m.Movies) != 1 || strings.Contains(m.Header, "?") || m.Capsule == "" {
				t.Fatalf("bad media: %+v", m)
			}
			continue
		}
		d := item.Details
		localized := item.Locale == "zh-cn" && item.StoreCountry != "hk"
		if d == nil || !slices.Equal(d.Developers, []string{"Fox Studio"}) || !slices.Equal(d.Publishers, []string{"Pine Press"}) ||
			d.Release == nil || d.Release.DateText != "Q3 2027" || d.Release.ComingSoon == nil || !*d.Release.ComingSoon ||
			d.Platforms == nil || d.Platforms.Mac == nil || *d.Platforms.Mac || d.Platforms.Windows == nil || !*d.Platforms.Windows ||
			d.Platforms.Linux != nil || !strings.Contains(d.SupportedLanguages, "Simplified Chinese") || strings.Contains(d.SupportedLanguages, "private") ||
			len(d.Genres) != 1 || len(d.Categories) != 1 || localized != (d.Name == "松林故事") || localized != item.VerifiedChinese {
			t.Fatalf("bad Storefront details: %+v", d)
		}
	}
	for _, price := range steam.Prices {
		switch price.StoreCountry {
		case "us":
			if price.State != "zero_price" || price.Amounts == nil || price.Amounts.Currency != "USD" {
				t.Fatalf("zero-price market lost its amounts: %+v", price)
			}
		case "cn":
			if price.State != "free" || price.Amounts != nil {
				t.Fatalf("free market acquired an amount: %+v", price)
			}
		default:
			t.Fatalf("unavailable price appeared: %+v", price)
		}
	}
	p, err = editor.ReadGamePresentation(t.Context(), unrelated, "cn")
	must(t, err)
	if len(p.Sources) != 0 {
		t.Fatal("unrelated work inherited the Steam source")
	}
	p, err = editor.ReadGamePresentation(t.Context(), id, "hk")
	must(t, err)
	if len(p.Sources[1].Metadata) != 0 || len(p.Sources[1].Prices) != 0 || len(p.Sources[1].News) != 0 || p.Sources[1].Players != nil {
		t.Fatal("an unrelated egress route inherited Steam data")
	}
	for _, region := range []string{"CN", "INTL"} {
		market := catalog.ChannelMarket{Region: region, Egress: "cn"}
		current, err := editor.GameDirectory(t.Context(), market, catalog.ChannelCriteria{Locale: "en", Keyword: "curated name", Store: "itch_io", System: "windows", Phase: "upcoming"})
		must(t, err)
		if len(current.Items) != 1 {
			t.Fatalf("%s source-backed directory: %+v", region, current)
		}
		game := current.Items[0]
		if game.PublicID != id || game.CoverURL == "" || game.Price == nil || game.Price.AppID != "42" ||
			game.Release == nil || game.Release.Day != nil || game.Release.Precision != "quarter" {
			t.Fatalf("%s lost scoped card facts: %+v", region, game)
		}
		home, err := editor.GameHome(t.Context(), market, "en")
		must(t, err)
		if len(home.News) != 1 || home.News[0].AppID != "42" || home.News[0].PublicID != id {
			t.Fatalf("%s lost attributed Steam news: %+v", region, home.News)
		}
		switch region {
		case "CN":
			if game.Price.State != "free" || game.Price.StoreCountry != "cn" || game.Price.Amounts != nil || len(home.Offers) != 1 {
				t.Fatalf("CN inherited a non-CN quote: %+v", game.Price)
			}
		case "INTL":
			if game.Price.State != "zero_price" || game.Price.StoreCountry != "us" || game.Price.Amounts == nil ||
				game.Price.Amounts.Currency != "USD" || len(home.Offers) != 0 {
				t.Fatalf("INTL zero amount was interpreted as free: %+v", game.Price)
			}
		}
	}
}

func replaySteamMetadata(t *testing.T, pool *pgxpool.Pool, consumer dbgen.CollectionConsumer, source uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	before := scalar[int64](t, pool, "SELECT sum(xmin::text::bigint)::bigint FROM game_source_metadata")
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		group.Go(func() {
			errs <- saveSteamMetadata(ctx, dbgen.New(pool), *consumer.Generation, *consumer.Epoch, &source, "")
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		must(t, err)
	}
	if after := scalar[int64](t, pool, "SELECT sum(xmin::text::bigint)::bigint FROM game_source_metadata"); before != after {
		t.Fatal("unchanged concurrent replay rewrote metadata")
	}
}

func readAgedSteamGame(t *testing.T, pool *pgxpool.Pool, editor *catalog.Service, gameID string, checkHTTP func()) {
	t.Helper()
	ctx := t.Context()
	first, err := editor.PublishedGame(ctx, gameID)
	must(t, err)
	before, err := editor.ReadGamePresentation(ctx, gameID, "cn")
	must(t, err)
	index := slices.IndexFunc(before.Sources, func(src catalog.GameSourceSnapshot) bool {
		return src.PlatformCode == "steam" && len(src.Metadata) != 0
	})
	if index < 0 {
		t.Fatal("no durable Steam metadata to age")
	}
	execSQL(t, pool, "DELETE FROM collection_current WHERE capability='steam.details' AND scope#>>'{dimensions,section}' IN ('details','media')")
	for _, days := range []int32{31, 181, 365} {
		execSQL(t, pool, "UPDATE game_source_metadata SET observed_at=now()-make_interval(days=>$1),confirmed_at=now()-make_interval(days=>$1) WHERE payload IS NOT NULL", days)
		execSQL(t, pool, "UPDATE game_approved_presentations SET observed_at=now()-make_interval(days=>$1)", days)
		execSQL(t, pool, "UPDATE collection_current SET observed_at=now()-make_interval(days=>$1),confirmed_at=now()-make_interval(days=>$1) WHERE capability IN ('steam.players','steam.details')", days)
		after, err := editor.PublishedGame(ctx, gameID)
		must(t, err)
		if !reflect.DeepEqual(first, after) {
			t.Fatalf("published identity changed at day %d", days)
		}
		checkHTTP()
		view, err := editor.ReadGamePresentation(ctx, gameID, "cn")
		must(t, err)
		if len(view.Sources[index].Metadata) != len(before.Sources[index].Metadata) {
			t.Fatalf("metadata disappeared at day %d", days)
		}
		for i, row := range view.Sources[index].Metadata {
			old := before.Sources[index].Metadata[i]
			if row.State != "historical" || row.ObservationID != old.ObservationID || !reflect.DeepEqual(row.Details, old.Details) || !reflect.DeepEqual(row.Media, old.Media) {
				t.Fatalf("durable source data changed at day %d", days)
			}
		}
		if len(view.Sources[index].Prices) != 0 || view.Sources[index].Players != nil {
			t.Fatalf("aged price or player observation appeared at day %d", days)
		}
		page, err := editor.GameDirectory(ctx, catalog.ChannelMarket{Region: "CN", Egress: "cn"}, catalog.ChannelCriteria{Locale: "en"})
		must(t, err)
		at := slices.IndexFunc(page.Items, func(card catalog.GameCard) bool { return card.PublicID == gameID })
		if at < 0 || page.Items[at].Price != nil || page.Items[at].CoverURL == "" {
			t.Fatalf("aging removed card or cover, or kept an expired quote at %d days", days)
		}
	}
}

func assertNotPublic(t *testing.T, ctx context.Context, editor *catalog.Service, id string) {
	t.Helper()
	_, err := editor.ReadGamePresentation(ctx, id, "cn")
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("non-public internal presentation: %v", err)
	}
}
