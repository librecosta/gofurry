package catalog

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func eventLocale(lang int32) string {
	switch lang {
	case 0:
		return "en"
	case 6:
		return "zh"
	default:
		return ""
	}
}

type GameNewsArticle struct {
	ID          uuid.UUID      `json:"id"`
	AppID       string         `json:"app_id"`
	Title       string         `json:"title"`
	Locale      string         `json:"locale"`
	PublishedAt time.Time      `json:"published_at"`
	UpdatedAt   *time.Time     `json:"updated_at,omitempty"`
	URL         string         `json:"url"`
	HTML        string         `json:"html"`
	Summary     string         `json:"summary"`
	Tags        []string       `json:"tags,omitempty"`
	Metrics     jsontext.Value `json:"metrics,omitzero"`
}

func (s *Service) PublicGameNewsArticle(ctx context.Context, market ChannelMarket, game string, article uuid.UUID) (GameNewsArticle, error) {
	if !ValidPublicID(game) || article[6]>>4 != 7 || article[8]>>6 != 2 {
		return GameNewsArticle{}, ErrNotFound
	}
	_, _, egress := market.scope()
	if !steamEgress.MatchString(egress) {
		return GameNewsArticle{}, ErrInvalid
	}
	if s.pool == nil {
		return GameNewsArticle{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row, err := dbgen.New(s.pool).PublicSteamEventDetail(ctx, dbgen.PublicSteamEventDetailParams{PublicID: game, ID: article, EgressRegion: egress})
	if err != nil {
		return GameNewsArticle{}, classify(err)
	}
	out := GameNewsArticle{
		ID: row.ID, AppID: row.AppID, Title: row.Headline, Locale: eventLocale(row.Language), PublishedAt: row.PublishedAt.Time.UTC(),
		URL:  "https://store.steampowered.com/news/app/" + row.AppID + "/view/" + row.AnnouncementGid,
		HTML: row.Html, Summary: row.Summary, Tags: row.Tags, Metrics: row.Metrics,
	}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time.UTC()
		out.UpdatedAt = &t
	}
	return out, nil
}

type SteamEventOpsItem struct {
	dbgen.SteamEventOperationsPageRow
	Metrics  jsontext.Value `json:"metrics"`
	Metadata jsontext.Value `json:"metadata"`
}

type SteamEventOpsDetail struct {
	dbgen.SteamEventOperationsDetailRow
	Metrics  jsontext.Value `json:"metrics"`
	Metadata jsontext.Value `json:"metadata"`
	Previous jsontext.Value `json:"previous,omitzero"`
}

func (s *Service) SteamEventOperations(ctx context.Context, game, locale, query string, before uuid.UUID) (ChannelPage[SteamEventOpsItem], error) {
	if !ValidPublicID(game) || (locale != "" && locale != "en" && locale != "zh" && locale != "unknown") || !ManagementSearch(query) {
		return ChannelPage[SteamEventOpsItem]{}, ErrInvalid
	}
	if s.pool == nil {
		return ChannelPage[SteamEventOpsItem]{}, ErrUnavailable
	}
	if before == uuid.Nil() {
		for i := range before {
			before[i] = 255
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := dbgen.New(s.pool).SteamEventOperationsPage(ctx, dbgen.SteamEventOperationsPageParams{
		PublicID: game, BeforeID: before, Locale: locale, Search: query,
	})
	if err != nil {
		return ChannelPage[SteamEventOpsItem]{}, classify(err)
	}
	out := ChannelPage[SteamEventOpsItem]{Items: []SteamEventOpsItem{}}
	for _, r := range rows[:min(len(rows), 30)] {
		out.Items = append(out.Items, SteamEventOpsItem{SteamEventOperationsPageRow: r, Metrics: r.Metrics, Metadata: r.Metadata})
	}
	if len(rows) > 30 {
		out.After = rows[29].ID.String()
	}
	return out, nil
}

func (s *Service) ManagementSteamEvent(ctx context.Context, game string, id uuid.UUID) (SteamEventOpsDetail, error) {
	if !ValidPublicID(game) || id[6]>>4 != 7 || id[8]>>6 != 2 {
		return SteamEventOpsDetail{}, ErrInvalid
	}
	if s.pool == nil {
		return SteamEventOpsDetail{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row, err := dbgen.New(s.pool).SteamEventOperationsDetail(ctx, dbgen.SteamEventOperationsDetailParams{PublicID: game, ID: id})
	return SteamEventOpsDetail{SteamEventOperationsDetailRow: row, Metrics: row.Metrics, Metadata: row.Metadata, Previous: row.Previous}, classify(err)
}

type SteamEventVisibility struct {
	ArticleID uuid.UUID `json:"article_id"`
	Revision  int32     `json:"revision"`
	Hidden    *bool     `json:"hidden"`
}

func ChangeSteamEventVisibility(ctx context.Context, q *dbgen.Queries, game string, in SteamEventVisibility) (ManagementResult, error) {
	if in.ArticleID[6]>>4 != 7 || in.ArticleID[8]>>6 != 2 || in.Revision < 1 || in.Hidden == nil {
		return ManagementResult{}, ErrInvalid
	}
	next, err := q.ChangeSteamEventVisibility(ctx, dbgen.ChangeSteamEventVisibilityParams{
		PublicID: game, ID: in.ArticleID, Hidden: *in.Hidden, Revision: in.Revision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := q.SteamEventOperationsDetail(ctx, dbgen.SteamEventOperationsDetailParams{PublicID: game, ID: in.ArticleID}); err != nil {
			return ManagementResult{}, classify(err)
		}
		return ManagementResult{}, ErrConflict
	}
	return ManagementResult{Target: game, Revision: int64(next)}, classify(err)
}
