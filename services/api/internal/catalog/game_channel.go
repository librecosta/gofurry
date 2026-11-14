package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	json "encoding/json/v2"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"golang.org/x/text/unicode/norm"
)

var channelGenres = []string{"action", "adventure", "casual", "free_to_play", "indie", "racing", "rpg", "simulation", "sports", "strategy"}

type ChannelMarket struct{ Region, Egress string }

func (m ChannelMarket) scope() (country, currency, egress string) {
	if m.Region == "INTL" {
		return "us", "USD", m.Egress
	}
	return "cn", "CNY", m.Egress
}

type ChannelCriteria struct {
	Locale, Keyword, Store, System, Phase, Price, Sort, After string
	Genres                                                    []string
	Limit                                                     int32
}

func (c *ChannelCriteria) normalize(kind string) error {
	if c.Limit == 0 {
		c.Limit = 24
	}
	if c.Limit < 1 || c.Limit > 48 || (c.Locale != "en" && c.Locale != "zh") || !validText(c.Keyword, 0, 100, false) {
		return ErrInvalid
	}
	c.Keyword = norm.NFKC.String(strings.ToLower(strings.TrimSpace(c.Keyword)))
	if !validText(c.Keyword, 0, 200, false) {
		return ErrInvalid
	}
	if kind == "members" {
		if c.Sort == "" {
			c.Sort = "editorial"
		}
		if c.Sort != "editorial" && c.Sort != "timeline" {
			return ErrInvalid
		}
		return nil
	}
	if c.Sort == "" {
		c.Sort = "added_desc"
	}
	if !slices.Contains([]string{"added_desc", "name_asc", "release_desc", "upcoming_asc"}, c.Sort) ||
		!slices.Contains([]string{"", "steam", "itch_io"}, c.Store) || !slices.Contains([]string{"", "windows", "mac", "linux"}, c.System) ||
		!slices.Contains([]string{"", "released", "upcoming", "unknown"}, c.Phase) || !slices.Contains([]string{"", "free", "discount"}, c.Price) ||
		len(c.Genres) > len(channelGenres) {
		return ErrInvalid
	}
	c.Genres = append([]string{}, c.Genres...)
	slices.Sort(c.Genres)
	for i, genre := range c.Genres {
		if !slices.Contains(channelGenres, genre) || (i > 0 && c.Genres[i-1] == genre) {
			return ErrInvalid
		}
	}
	return nil
}

type GameCard struct {
	Summary
	FirstPublishedAt *time.Time   `json:"first_published_at"`
	CoverURL         string       `json:"cover_url,omitempty"`
	Platforms        []Source     `json:"platforms"`
	Genres           []string     `json:"genres"`
	Systems          []string     `json:"systems"`
	ReleaseState     string       `json:"release_state"`
	Release          *GameRelease `json:"release,omitempty"`
	Price            *GameQuote   `json:"price_summary,omitempty"`
}

type GameRelease struct {
	Platform      string    `json:"platform"`
	AppID         string    `json:"app_id"`
	Text          string    `json:"text"`
	Precision     string    `json:"precision"`
	Day           *string   `json:"day"`
	StoreCountry  string    `json:"store_country"`
	Locale        string    `json:"locale"`
	ObservedAt    time.Time `json:"observed_at"`
	ObservationID uuid.UUID `json:"observation_id"`
}

type GameQuote struct {
	SteamStoreScope
	Platform      string        `json:"platform"`
	AppID         string        `json:"app_id"`
	State         string        `json:"state"`
	Amounts       *SteamAmounts `json:"amounts"`
	ObservedAt    time.Time     `json:"observed_at"`
	ConfirmedAt   time.Time     `json:"confirmed_at"`
	ObservationID uuid.UUID     `json:"observation_id"`
}

type FeaturedGame struct {
	Slot string   `json:"slot"`
	Game GameCard `json:"game"`
}
type ChannelGenre struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}
type ChannelNews struct {
	PublicID      string    `json:"public_id"`
	Slug          string    `json:"slug"`
	AppID         string    `json:"app_id"`
	GID           string    `json:"gid"`
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	PublishedAt   time.Time `json:"published_at"`
	ObservationID uuid.UUID `json:"observation_id"`
}
type GameChannel struct {
	Featured    []FeaturedGame      `json:"featured"`
	Recent      []GameCard          `json:"recent"`
	Upcoming    []GameCard          `json:"upcoming"`
	Released    []GameCard          `json:"released"`
	Offers      []GameCard          `json:"offers"`
	Genres      []ChannelGenre      `json:"genres"`
	Collections []CollectionSummary `json:"collections"`
	News        []ChannelNews       `json:"news"`
}
type ChannelPage[T any] struct {
	Items []T    `json:"items"`
	After string `json:"after,omitempty"`
}

type channelCursor struct {
	Key      string
	AsOf     int64
	Bucket   int32
	Ordering int64
	Name, ID string
}

func newChannelCursor(c ChannelCriteria, market ChannelMarket, view, identity string, revision int64) (channelCursor, error) {
	after := c.After
	c.After = ""
	data, err := json.Marshal([]any{view, identity, revision, market, c})
	if err != nil {
		return channelCursor{}, ErrInvalid
	}
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:16])
	now := time.Now().UTC().UnixMicro()
	if after == "" {
		return channelCursor{Key: key, AsOf: now}, nil
	}
	var pos channelCursor
	if len(after) > 2048 {
		return pos, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(after)
	if err != nil || json.Unmarshal(raw, &pos) != nil || pos.Key != key || !ValidPublicID(pos.ID) ||
		pos.Bucket < 0 || pos.Bucket > 2 || !validText(pos.Name, 0, 500, false) || pos.AsOf <= 0 || pos.AsOf > now ||
		pos.Ordering < -9007199254740991 || pos.Ordering > 9007199254740991 {
		return channelCursor{}, ErrInvalid
	}
	return pos, nil
}

func (p channelCursor) encode() string {
	raw, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func channelTime(micro int64) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.UnixMicro(micro).UTC(), Valid: true}
}

func (s *Service) GameDirectory(ctx context.Context, market ChannelMarket, c ChannelCriteria) (ChannelPage[GameCard], error) {
	if err := c.normalize("directory"); err != nil {
		return ChannelPage[GameCard]{}, err
	}
	if s.pool == nil {
		return ChannelPage[GameCard]{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if c.recentDirectory() {
		return channelCards(ctx, dbgen.New(s.pool), market, c, "directory", nil, "", 0)
	}
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return ChannelPage[GameCard]{}, err
	}
	defer tx.Rollback(context.Background())
	result, err := channelCards(ctx, q, market, c, "directory", nil, "", 0)
	if err != nil {
		return ChannelPage[GameCard]{}, err
	}
	return result, classify(tx.Commit(ctx))
}

func (c ChannelCriteria) recentDirectory() bool {
	return c.Sort == "added_desc" && len(c.Genres) == 0 && c.System == "" && c.Phase == "" && c.Price == "" && c.Keyword == ""
}

func (s *Service) channelRead(ctx context.Context) (pgx.Tx, *dbgen.Queries, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	q := dbgen.New(tx)
	// PostgreSQL otherwise JIT-compiles long branches in generic plans even
	// for tiny indexed pages. Limit the setting to this read transaction.
	if err := q.DisableGameChannelJIT(ctx); err != nil {
		_ = tx.Rollback(context.Background())
		return nil, nil, ErrUnavailable
	}
	return tx, q, nil
}

func channelCards(ctx context.Context, q *dbgen.Queries, market ChannelMarket, c ChannelCriteria, kind string, id *uuid.UUID, identity string, revision int64) (ChannelPage[GameCard], error) {
	pos, err := newChannelCursor(c, market, kind, identity, revision)
	if err != nil {
		return ChannelPage[GameCard]{}, err
	}
	country, currency, egress := market.scope()
	rows, err := readChannelRows(ctx, q, c, kind, id, pos, country, currency, egress)
	if err != nil {
		return ChannelPage[GameCard]{}, classify(err)
	}
	result := ChannelPage[GameCard]{Items: make([]GameCard, 0, min(int(c.Limit), len(rows)))}
	if len(rows) > int(c.Limit) {
		rows = rows[:c.Limit]
		last := rows[len(rows)-1]
		pos.ID, pos.Bucket, pos.Ordering, pos.Name = last.PublicID, last.EBucket, last.EOrdering, last.ENameKey
		result.After = pos.encode()
	}
	for _, row := range rows {
		card, err := decodeGameCard(row.Card)
		if err != nil {
			return ChannelPage[GameCard]{}, err
		}
		result.Items = append(result.Items, card)
	}
	return result, nil
}

func readChannelRows(ctx context.Context, q *dbgen.Queries, c ChannelCriteria, kind string, id *uuid.UUID, pos channelCursor, country, currency, egress string) ([]dbgen.GameChannelCardsRow, error) {
	if kind == "directory" && c.recentDirectory() {
		language := "en"
		if c.Locale == "zh" {
			language = "zh-cn"
		}
		stream, err := q.RecentGameChannelCards(ctx, dbgen.RecentGameChannelCardsParams{
			Language: language, Country: country, Currency: currency, Egress: egress, StoreFilter: c.Store,
			AsOf: channelTime(pos.AsOf), HasAfter: pos.ID != "", AfterBucket: pos.Bucket, AfterTime: channelTime(-pos.Ordering),
			AfterID: pos.ID, TakeRows: c.Limit + 1,
		})
		if err != nil {
			return nil, err
		}
		rows := make([]dbgen.GameChannelCardsRow, 0, len(stream))
		for _, row := range stream {
			rows = append(rows, dbgen.GameChannelCardsRow(row))
		}
		return rows, nil
	}
	return q.GameChannelCards(ctx, dbgen.GameChannelCardsParams{
		Locale: c.Locale, Country: country, Currency: currency, Egress: egress, ViewKind: kind, SortKey: c.Sort,
		GenreFilters: append([]string{}, c.Genres...), StoreFilter: c.Store, SystemFilter: c.System, PhaseFilter: c.Phase, PriceFilter: c.Price, Keyword: c.Keyword,
		CollectionID: id, AsOf: channelTime(pos.AsOf), HasAfter: pos.ID != "", AfterBucket: pos.Bucket,
		AfterOrdering: pos.Ordering, AfterName: pos.Name, AfterID: pos.ID, TakeRows: c.Limit + 1,
	})
}

func decodeGameCard(raw []byte) (GameCard, error) {
	var card GameCard
	if json.Unmarshal(raw, &card) != nil || !ValidPublicID(card.PublicID) || len(card.Localizations) == 0 {
		return GameCard{}, ErrUnavailable
	}
	card.CoverURL = SteamMediaURL(card.CoverURL)
	return card, nil
}

// SteamMediaURL applies the same allowlist to durable metadata writes and public reads.
func SteamMediaURL(raw string) string {
	if len(raw) > 512 || strings.ContainsAny(raw, "<>\\") || strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Scheme != "https" || u.Host != u.Hostname() || u.Path == "" || u.Path == "/" ||
		strings.ContainsAny(u.Path, "\\<>") || strings.ContainsFunc(u.Path, unicode.IsControl) {
		return ""
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, ".steamstatic.com") && !strings.HasSuffix(host, ".steamcontent.com") &&
		host != "steamcdn-a.akamaihd.net" && host != "steamuserimages-a.akamaihd.net" {
		return ""
	}
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	return u.String()
}

func (s *Service) GameHome(ctx context.Context, market ChannelMarket, locale string) (GameChannel, error) {
	if locale != "zh" && locale != "en" {
		return GameChannel{}, ErrInvalid
	}
	if s.pool == nil {
		return GameChannel{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return GameChannel{}, ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	country, currency, egress := market.scope()
	rows, err := q.GameChannelHome(ctx, dbgen.GameChannelHomeParams{Locale: locale, Country: country, Currency: currency, Egress: egress})
	if err != nil {
		return GameChannel{}, classify(err)
	}
	result := GameChannel{Featured: []FeaturedGame{}, Recent: []GameCard{}, Upcoming: []GameCard{}, Released: []GameCard{}, Offers: []GameCard{},
		Genres: []ChannelGenre{}, Collections: []CollectionSummary{}, News: []ChannelNews{}}
	counts := make(map[string]int64, len(channelGenres))
	lanes := map[string]*[]GameCard{"recent": &result.Recent, "upcoming": &result.Upcoming, "released": &result.Released, "offers": &result.Offers}
	for _, r := range rows {
		if r.Kind == "genre" {
			var g ChannelGenre
			if json.Unmarshal(r.Payload, &g) != nil {
				return GameChannel{}, ErrUnavailable
			}
			counts[g.Code] = g.Count
			continue
		}
		card, err := decodeGameCard(r.Payload)
		if err != nil {
			return GameChannel{}, err
		}
		if r.Kind == "featured" {
			result.Featured = append(result.Featured, FeaturedGame{Slot: r.Slot, Game: card})
		} else if lane := lanes[r.Kind]; lane != nil {
			*lane = append(*lane, card)
		}
	}
	for _, g := range channelGenres {
		result.Genres = append(result.Genres, ChannelGenre{Code: g, Count: counts[g]})
	}
	collections, err := q.FeaturedGameCollections(ctx)
	if err != nil {
		return GameChannel{}, classify(err)
	}
	for _, r := range collections {
		item, err := collectionSummary(r.PublicID, r.Slug, r.Localizations, r.FirstPublishedAt)
		if err != nil {
			return GameChannel{}, err
		}
		result.Collections = append(result.Collections, item)
	}
	news, err := q.GameChannelNews(ctx, egress)
	if err != nil {
		return GameChannel{}, classify(err)
	}
	for _, r := range news {
		item := ChannelNews{PublicID: r.PublicID, Slug: r.Slug, AppID: r.ExternalKey, GID: r.Gid, PublishedAt: r.PublishedAt.Time, ObservationID: r.ObservationID}
		var story struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		}
		if json.Unmarshal(r.Value, &story) != nil {
			return GameChannel{}, ErrUnavailable
		}
		item.Title, item.URL = story.Title, story.URL
		result.News = append(result.News, item)
	}
	return result, classify(tx.Commit(ctx))
}
