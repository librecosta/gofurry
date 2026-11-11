package catalog

import (
	"context"
	json "encoding/json/v2"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type Summary struct {
	PublicID      string         `json:"public_id"`
	Slug          string         `json:"slug"`
	Localizations []Localization `json:"localizations"`
}

type SiteDetail struct {
	Summary
	Addresses []Address `json:"addresses"`
}

type GameDetail struct {
	Summary
	Sources []Source `json:"sources"`
}

type Page struct {
	Items []Summary `json:"items"`
	After string    `json:"after,omitempty"`
}

func summary(id, slug string, localizations []byte) (Summary, error) {
	out := Summary{PublicID: id, Slug: slug}
	if err := json.Unmarshal(localizations, &out.Localizations); err != nil {
		return Summary{}, ErrUnavailable
	}
	return out, nil
}

func (s *Service) PublishedSite(ctx context.Context, publicID string) (SiteDetail, error) {
	if !ValidPublicID(publicID) {
		return SiteDetail{}, ErrNotFound
	}
	if s.pool == nil {
		return SiteDetail{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row, err := dbgen.New(s.pool).PublishedSite(ctx, publicID)
	if err != nil {
		return SiteDetail{}, classify(err)
	}
	base, err := summary(row.PublicID, row.Slug, row.Localizations)
	out := SiteDetail{Summary: base}
	if err != nil || json.Unmarshal(row.Addresses, &out.Addresses) != nil {
		return SiteDetail{}, ErrUnavailable
	}
	return out, nil
}

func (s *Service) PublishedGame(ctx context.Context, publicID string) (GameDetail, error) {
	if !ValidPublicID(publicID) {
		return GameDetail{}, ErrNotFound
	}
	if s.pool == nil {
		return GameDetail{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row, err := dbgen.New(s.pool).PublishedGame(ctx, publicID)
	if err != nil {
		return GameDetail{}, classify(err)
	}
	base, err := summary(row.PublicID, row.Slug, row.Localizations)
	out := GameDetail{Summary: base}
	if err != nil || json.Unmarshal(row.Sources, &out.Sources) != nil {
		return GameDetail{}, ErrUnavailable
	}
	return out, nil
}

type projection struct {
	id, slug      string
	localizations []byte
}

func (s *Service) SitePage(ctx context.Context, after string, limit int32) (Page, error) {
	return readPage(ctx, s, after, limit, dbgen.New(s.pool).PublishedSitePage, func(r dbgen.PublishedSitePageRow) projection {
		return projection{r.PublicID, r.Slug, r.Localizations}
	}, dbgen.PublishedSitePageParams{AfterPublicID: after, PageSize: limit + 1})
}

func (s *Service) GamePage(ctx context.Context, after string, limit int32) (Page, error) {
	return readPage(ctx, s, after, limit, dbgen.New(s.pool).PublishedGamePage, func(r dbgen.PublishedGamePageRow) projection {
		return projection{r.PublicID, r.Slug, r.Localizations}
	}, dbgen.PublishedGamePageParams{AfterPublicID: after, PageSize: limit + 1})
}

func readPage[T, P any](ctx context.Context, s *Service, after string, limit int32, query func(context.Context, P) ([]T, error), fields func(T) projection, params P) (Page, error) {
	if (after != "" && !ValidPublicID(after)) || limit < 1 || limit > 50 {
		return Page{}, ErrInvalid
	}
	if s.pool == nil {
		return Page{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := query(ctx, params)
	if err != nil {
		return Page{}, classify(err)
	}
	out := Page{Items: make([]Summary, 0, min(len(rows), int(limit)))}
	for _, row := range rows[:min(len(rows), int(limit))] {
		value := fields(row)
		item, err := summary(value.id, value.slug, value.localizations)
		if err != nil {
			return Page{}, err
		}
		out.Items = append(out.Items, item)
	}
	if len(rows) > int(limit) {
		out.After = out.Items[len(out.Items)-1].PublicID
	}
	return out, nil
}
