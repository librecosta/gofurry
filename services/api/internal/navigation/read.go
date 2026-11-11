package navigation

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const Algorithm = 1

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool, time.Now} }

type Card struct {
	catalog.Summary
	PrimaryCategory     string     `json:"primary_category"`
	SecondaryCategories []string   `json:"secondary_categories"`
	Tags                []string   `json:"tags"`
	Languages           []string   `json:"languages"`
	FirstPublishedAt    *time.Time `json:"first_published_at"`
	HeatQualified       bool       `json:"heat_qualified,omitempty"`
}

type Heat struct {
	Status          string     `json:"status"`
	Algorithm       int        `json:"algorithm"`
	WindowStart     string     `json:"window_start,omitempty"`
	WindowEnd       string     `json:"window_end,omitempty"`
	ComputedAt      *time.Time `json:"computed_at,omitempty"`
	AggregateCutoff *time.Time `json:"aggregate_cutoff,omitempty"`
}

type Category struct {
	catalog.NavigationTerm
	Count int64 `json:"count"`
}

type Featured struct {
	Primary    *Card `json:"primary"`
	Secondary1 *Card `json:"secondary_1"`
	Secondary2 *Card `json:"secondary_2"`
}

type Home struct {
	Version    int        `json:"version"`
	Featured   Featured   `json:"featured"`
	Trending   []Card     `json:"trending"`
	Recent     []Card     `json:"recent"`
	Categories []Category `json:"categories"`
	Random     *Card      `json:"random"`
	Heat       Heat       `json:"heat"`
}

type Filters struct {
	Keyword, Sort, Locale       string
	Categories, Tags, Languages []string
	PageSize                    int32
}

type Directory struct {
	Version       int                      `json:"version"`
	Items         []Card                   `json:"items"`
	Total         int64                    `json:"total"`
	Next          string                   `json:"next,omitempty"`
	Sort          string                   `json:"sort"`
	EffectiveSort string                   `json:"effective_sort"`
	Heat          Heat                     `json:"heat"`
	Tags          []catalog.NavigationTerm `json:"tag_registry"`
}

type position struct {
	Filter     string `json:"f"`
	Generation int64  `json:"g"`
	ID         string `json:"id"`
}

func cleanCodes(values []string, maximum int, valid func(string) bool) ([]string, bool) {
	if len(values) > maximum {
		return nil, false
	}
	out := append([]string{}, values...)
	slices.Sort(out)
	for i, value := range out {
		if !valid(value) || (i > 0 && value == out[i-1]) {
			return nil, false
		}
	}
	return out, true
}

func normalize(f Filters) (Filters, error) {
	var valid bool
	f.Keyword = strings.TrimSpace(f.Keyword)
	if !utf8.ValidString(f.Keyword) || utf8.RuneCountInString(f.Keyword) > 80 || strings.ContainsFunc(f.Keyword, unicode.IsControl) {
		return f, catalog.ErrInvalid
	}
	if f.Locale != "en" && f.Locale != "zh" {
		return f, catalog.ErrInvalid
	}
	if f.Sort == "" {
		f.Sort = "published_desc"
		if f.Keyword != "" {
			f.Sort = "relevance"
		}
	}
	if !slices.Contains([]string{"relevance", "published_desc", "heat_7d", "name_asc"}, f.Sort) || f.Sort == "relevance" && f.Keyword == "" {
		return f, catalog.ErrInvalid
	}
	if f.PageSize == 0 {
		f.PageSize = 24
	}
	if f.PageSize < 1 || f.PageSize > 48 {
		return f, catalog.ErrInvalid
	}
	if f.Categories, valid = cleanCodes(f.Categories, 8, catalog.NavigationCategory); !valid {
		return f, catalog.ErrInvalid
	}
	if f.Tags, valid = cleanCodes(f.Tags, 8, catalog.NavigationTag); !valid {
		return f, catalog.ErrInvalid
	}
	if f.Languages, valid = cleanCodes(f.Languages, 8, catalog.NavigationLanguage); !valid {
		return f, catalog.ErrInvalid
	}
	return f, nil
}

func keywordTags(keyword string) []string {
	matching := []string{}
	word := strings.ToLower(keyword)
	if word != "" {
		for _, tag := range catalog.NavigationTags() {
			if strings.Contains(tag.Code, word) || strings.Contains(strings.ToLower(tag.EN), word) || strings.Contains(tag.ZH, word) {
				matching = append(matching, tag.Code)
			}
		}
	}
	return matching
}

func pgDay(t time.Time) pgtype.Date { return pgtype.Date{Time: t.UTC(), Valid: true} }

func heatState(ctx context.Context, q *dbgen.Queries, now time.Time) (Heat, error) {
	out := Heat{Status: "cold", Algorithm: Algorithm}
	h, err := q.NavigationHeatState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	end, computed, cutoff := h.WindowEnd.Time.UTC(), h.ComputedAt.Time.UTC(), h.AggregateCutoff.Time.UTC()
	out.Status = "stale"
	if h.Algorithm == Algorithm && end.Format(time.DateOnly) == now.Format(time.DateOnly) &&
		!computed.After(now) && now.Sub(computed) <= 2*time.Hour {
		out.Status = "ready"
	}
	out.WindowStart, out.WindowEnd = end.AddDate(0, 0, -6).Format(time.DateOnly), end.Format(time.DateOnly)
	out.ComputedAt, out.AggregateCutoff = &computed, &cutoff
	return out, nil
}

func read[T any](ctx context.Context, pool *pgxpool.Pool, fn func(context.Context, *dbgen.Queries) (T, error)) (out T, err error) {
	if pool == nil {
		return out, catalog.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		out, err = fn(ctx, dbgen.New(tx))
		return err
	})
	switch {
	case err == nil, errors.Is(err, catalog.ErrInvalid):
		return out, err
	case errors.Is(err, pgx.ErrNoRows):
		return out, catalog.ErrNotFound
	default:
		return out, catalog.ErrUnavailable
	}
}

func (s *Service) Directory(ctx context.Context, filters Filters, cursor string) (Directory, error) {
	f, err := normalize(filters)
	if err != nil {
		return Directory{}, err
	}
	fingerprint, _ := json.Marshal(f, json.Deterministic(true))
	digest := sha256.Sum256(fingerprint)
	key := hex.EncodeToString(digest[:16])
	var after position
	if cursor != "" {
		if len(cursor) > 200 {
			return Directory{}, catalog.ErrInvalid
		}
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &after, json.RejectUnknownMembers(true)) != nil ||
			after.Filter != key || !catalog.ValidPublicID(after.ID) {
			return Directory{}, catalog.ErrInvalid
		}
	}
	return read(ctx, s.pool, func(ctx context.Context, q *dbgen.Queries) (Directory, error) {
		h, err := heatState(ctx, q, s.now().UTC())
		result := Directory{Version: catalog.NavigationVersion, Sort: f.Sort, EffectiveSort: f.Sort, Heat: h,
			Tags: catalog.NavigationTags(), Items: []Card{}}
		if err != nil {
			return result, err
		}
		generation := int64(0)
		if f.Sort == "heat_7d" {
			if h.Status == "ready" {
				generation = h.ComputedAt.UnixMicro()
			} else {
				result.EffectiveSort = "published_desc"
			}
		}
		if cursor != "" && after.Generation != generation {
			return result, catalog.ErrInvalid
		}
		row, err := q.NavigationDirectory(ctx, dbgen.NavigationDirectoryParams{Keyword: f.Keyword, OrderBy: result.EffectiveSort,
			SearchTags: keywordTags(f.Keyword), Locale: f.Locale, UseHeat: generation != 0, Categories: f.Categories, Tags: f.Tags, Languages: f.Languages,
			AfterID: after.ID, PageSize: f.PageSize + 1})
		if err != nil {
			return result, err
		}
		if !row.CursorValid {
			return result, catalog.ErrInvalid
		}
		var items []Card
		if err := json.Unmarshal(row.Items, &items); err != nil {
			return result, err
		}
		result.Total = row.Total
		if len(items) > int(f.PageSize) {
			items = items[:f.PageSize]
			last := items[len(items)-1]
			raw, _ := json.Marshal(position{key, generation, last.PublicID})
			result.Next = base64.RawURLEncoding.EncodeToString(raw)
		}
		result.Items = append(result.Items, items...)
		return result, nil
	})
}

func (s *Service) Home(ctx context.Context) (Home, error) {
	now := s.now().UTC()
	return read(ctx, s.pool, func(ctx context.Context, q *dbgen.Queries) (Home, error) {
		out := Home{Version: catalog.NavigationVersion, Trending: []Card{}, Recent: []Card{}}
		var err error
		out.Heat, err = heatState(ctx, q, now)
		if err != nil {
			return out, err
		}
		rows, err := q.NavigationHome(ctx, dbgen.NavigationHomeParams{UseHeat: out.Heat.Status == "ready", Seed: now.Format(time.DateOnly)})
		if err != nil {
			return out, err
		}
		for _, row := range rows {
			var card Card
			if err := json.Unmarshal(row.Item, &card); err != nil {
				return out, err
			}
			switch row.Section {
			case "featured":
				switch row.Key {
				case "primary":
					out.Featured.Primary = &card
				case "secondary_1":
					out.Featured.Secondary1 = &card
				case "secondary_2":
					out.Featured.Secondary2 = &card
				}
			case "trending":
				out.Trending = append(out.Trending, card)
			case "recent":
				out.Recent = append(out.Recent, card)
			case "random":
				out.Random = &card
			}
		}
		totals, err := q.NavigationCategoryCounts(ctx)
		if err != nil {
			return out, err
		}
		counts := make(map[string]int64, len(totals))
		for _, value := range totals {
			counts[value.Code] = value.Count
		}
		for _, category := range catalog.NavigationCategories() {
			out.Categories = append(out.Categories, Category{category, counts[category.Code]})
		}
		return out, nil
	})
}

type Stats struct {
	WindowStart    string `json:"window_start"`
	WindowEnd      string `json:"window_end"`
	DetailViews    *int64 `json:"detail_views,omitempty"`
	OutboundClicks *int64 `json:"outbound_clicks,omitempty"`
}

func (s *Service) Stats(ctx context.Context, id string) (Stats, error) {
	if !catalog.ValidPublicID(id) {
		return Stats{}, catalog.ErrNotFound
	}
	now := s.now().UTC()
	return read(ctx, s.pool, func(ctx context.Context, q *dbgen.Queries) (Stats, error) {
		out := Stats{WindowStart: now.AddDate(0, 0, -6).Format(time.DateOnly), WindowEnd: now.Format(time.DateOnly)}
		counts, err := q.NavigationStats(ctx, dbgen.NavigationStatsParams{PublicID: id, UtcDay: pgDay(now)})
		if err == nil && counts.ConfirmedDays >= 2 && counts.DetailViews+counts.OutboundClicks >= 10 {
			out.DetailViews, out.OutboundClicks = &counts.DetailViews, &counts.OutboundClicks
		}
		return out, err
	})
}
