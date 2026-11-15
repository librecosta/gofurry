package catalog

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type GameStory struct {
	AppID       string    `json:"app_id"`
	GID         string    `json:"gid"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

type gameNewsCursor struct {
	Game, App, GID, Source string
	Time                   int64
}

func officialStoryURL(raw, app string) string {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 220 || u.Scheme != "https" || u.User != nil || u.Host != u.Hostname() ||
		u.RawPath != "" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery {
		return ""
	}
	stem, id, ok := strings.Cut(u.Path, "/detail/")
	expected := false
	switch u.Host {
	case "steamcommunity.com":
		expected = ok && (stem == "/games/"+app+"/announcements" || stem == "/ogg/"+app+"/announcements" || stem == "/announcements")
	case "store.steampowered.com":
		id, expected = strings.CutPrefix(u.Path, "/news/app/"+app+"/view/")
	}
	if n, err := strconv.ParseUint(id, 10, 64); expected && err == nil && n > 0 && strconv.FormatUint(n, 10) == id {
		return raw
	}
	return ""
}

func gameProfileNews(ctx context.Context, q *dbgen.Queries, id, app, egress, after string, limit int32) (ChannelPage[GameStory], error) {
	pos := gameNewsCursor{Game: id, Source: app, Time: time.Now().Add(time.Minute).UnixMicro(), GID: "~", App: "~"}
	if after != "" {
		raw, err := base64.RawURLEncoding.Strict().DecodeString(after)
		if len(after) > 512 || err != nil || json.Unmarshal(raw, &pos) != nil || pos.Game != id || pos.Source != app ||
			!validAppID(pos.App) || pos.Time <= 0 || pos.Time > time.Now().UnixMicro() {
			return ChannelPage[GameStory]{}, ErrInvalid
		}
		gid, err := strconv.ParseUint(pos.GID, 10, 64)
		if err != nil || gid == 0 || strconv.FormatUint(gid, 10) != pos.GID {
			return ChannelPage[GameStory]{}, ErrInvalid
		}
	}
	rows, err := q.GameProfileNews(ctx, dbgen.GameProfileNewsParams{PublicID: id, AppID: app, Egress: egress, AfterTime: channelTime(pos.Time),
		AfterGid: pos.GID, AfterApp: pos.App, TakeRows: limit + 1})
	if err != nil {
		return ChannelPage[GameStory]{}, classify(err)
	}
	out := ChannelPage[GameStory]{Items: []GameStory{}}
	for _, row := range rows[:min(len(rows), int(limit))] {
		if safe := officialStoryURL(row.Url, row.AppID); safe != "" {
			out.Items = append(out.Items, GameStory{AppID: row.AppID, GID: row.Gid, Title: row.Title, URL: safe, PublishedAt: row.PublishedAt.Time.UTC()})
		}
	}
	if len(rows) > int(limit) {
		last := rows[limit-1]
		raw, _ := json.Marshal(gameNewsCursor{Game: id, Source: app, GID: last.Gid, App: last.AppID, Time: last.PublishedAt.Time.UnixMicro()})
		out.After = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}

type PlayerDay struct {
	Day        string    `json:"day"`
	Samples    int32     `json:"samples"`
	Maximum    int64     `json:"maximum"`
	Average    float64   `json:"average"`
	ObservedAt time.Time `json:"observed_at"`
}

type PricePoint struct {
	Day           string    `json:"day"`
	ObservedAt    time.Time `json:"observed_at"`
	ObservationID string    `json:"observation_id"`
	State         string    `json:"state"`
	Original      *int64    `json:"original"`
	Final         *int64    `json:"final"`
	Discount      *int32    `json:"discount_percent"`
}

type GameInsights struct {
	AppID        string       `json:"app_id,omitempty"`
	StoreCountry string       `json:"store_country"`
	Currency     string       `json:"currency"`
	PriceLocale  string       `json:"price_locale,omitempty"`
	Ranges       []int32      `json:"ranges"`
	Range        int32        `json:"range"`
	FromDay      string       `json:"from_day"`
	ThroughDay   string       `json:"through_day"`
	PlayerDays   []PlayerDay  `json:"player_days"`
	PriceDays    []PricePoint `json:"price_days"`
	PriceChanges []PricePoint `json:"price_changes"`
	News         []GameStory  `json:"news"`
}

func (s *Service) PublicGameNews(ctx context.Context, market ChannelMarket, id, app, after string) (ChannelPage[GameStory], error) {
	if !ValidPublicID(id) {
		return ChannelPage[GameStory]{}, ErrNotFound
	}
	_, _, egress := market.scope()
	if !steamEgress.MatchString(egress) || app != "" && !validAppID(app) {
		return ChannelPage[GameStory]{}, ErrInvalid
	}
	if s.pool == nil {
		return ChannelPage[GameStory]{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return ChannelPage[GameStory]{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := q.PublicGameIdentity(ctx, id); err != nil {
		return ChannelPage[GameStory]{}, classify(err)
	}
	out, err := gameProfileNews(ctx, q, id, app, egress, after, 6)
	if err != nil {
		return ChannelPage[GameStory]{}, err
	}
	return out, classify(tx.Commit(ctx))
}

func (s *Service) PublicGameInsights(ctx context.Context, market ChannelMarket, id, app, country, locale string, days int32) (GameInsights, error) {
	if !ValidPublicID(id) {
		return GameInsights{}, ErrNotFound
	}
	fallback, _, egress := market.scope()
	if country == "" {
		country = fallback
	}
	currency := storeCurrency(country)
	if currency == "" || !steamEgress.MatchString(egress) || app != "" && !validAppID(app) ||
		locale != "en" && locale != "zh" || days != 7 && days != 30 && days != 90 && days != 365 {
		return GameInsights{}, ErrInvalid
	}
	if s.pool == nil {
		return GameInsights{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return GameInsights{}, err
	}
	defer tx.Rollback(ctx)
	today, err := q.PublicGameIdentity(ctx, id)
	if err != nil {
		return GameInsights{}, classify(err)
	}
	day, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return GameInsights{}, ErrUnavailable
	}
	out := GameInsights{StoreCountry: country, Currency: currency, Ranges: []int32{7, 30}, Range: days,
		FromDay: day.AddDate(0, 0, 1-int(days)).Format(time.DateOnly), ThroughDay: today,
		PlayerDays: []PlayerDay{}, PriceDays: []PricePoint{}, PriceChanges: []PricePoint{}, News: []GameStory{}}
	sources, err := q.GameInsightSources(ctx, id)
	if err != nil {
		return GameInsights{}, classify(err)
	}
	for _, source := range sources {
		if app != "" && source.ExternalKey != app {
			continue
		}
		if err := loadGameInsights(ctx, q, source, egress, steamLocale(locale), &out); err != nil {
			return GameInsights{}, err
		}
		news, err := gameProfileNews(ctx, q, id, source.ExternalKey, egress, "", 20)
		if err != nil {
			return GameInsights{}, err
		}
		for _, item := range news.Items {
			if item.PublishedAt.Format(time.DateOnly) >= out.FromDay {
				out.News = append(out.News, item)
			}
		}
		break
	}
	if app != "" && out.AppID == "" {
		return GameInsights{}, ErrInvalid
	}
	return out, classify(tx.Commit(ctx))
}

func loadGameInsights(ctx context.Context, q *dbgen.Queries, source dbgen.GameInsightSourcesRow, egress, locale string, out *GameInsights) error {
	out.AppID = source.ExternalKey
	span, err := q.GameInsightCoverage(ctx, dbgen.GameInsightCoverageParams{SourceID: source.ID, Egress: egress, Country: out.StoreCountry, Currency: out.Currency})
	if err != nil {
		return classify(err)
	}
	if span.ShowNinety {
		out.Ranges = append(out.Ranges, 90)
	}
	if span.ShowYear {
		out.Ranges = append(out.Ranges, 365)
	}
	players, err := q.GamePlayerDays(ctx, dbgen.GamePlayerDaysParams{SourceID: source.ID, Egress: egress, Days: out.Range})
	if err != nil {
		return classify(err)
	}
	for _, row := range players {
		out.PlayerDays = append(out.PlayerDays, PlayerDay{Day: row.Day, Samples: row.Samples, Maximum: row.Maximum, Average: row.Average, ObservedAt: row.ObservedAt.Time.UTC()})
	}
	prices, err := q.GamePriceSeries(ctx, dbgen.GamePriceSeriesParams{SourceID: source.ID, Egress: egress,
		Country: out.StoreCountry, Currency: out.Currency, Locale: locale, Days: out.Range})
	if err != nil {
		return classify(err)
	}
	for _, row := range prices {
		out.PriceLocale = row.Locale
		point := PricePoint{Day: row.Day, ObservedAt: row.ObservedAt.Time.UTC(), ObservationID: row.ObservationID.String(), State: row.State,
			Original: amount(row.Original), Final: amount(row.Final)}
		if row.Discount.Valid {
			point.Discount = &row.Discount.Int32
		}
		if row.Daily {
			out.PriceDays = append(out.PriceDays, point)
		}
		if row.Changed && len(out.PriceChanges) < 60 {
			out.PriceChanges = append(out.PriceChanges, point)
		}
	}
	return nil
}

func amount(value pgtype.Int8) *int64 {
	if value.Valid {
		return &value.Int64
	}
	return nil
}
