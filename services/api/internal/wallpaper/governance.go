package wallpaper

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"strings"
	"time"
	"uuid"
)

type Detail struct {
	ID uuid.UUID `json:"id"`
	Metadata
	Status      string     `json:"status"`
	Revision    int64      `json:"revision"`
	Image       Asset      `json:"image"`
	Preview     Asset      `json:"preview"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at"`
}

func (s *Service) detail(row dbgen.StartWallpaper) Detail {
	image, preview := s.assets(row)
	var published *time.Time
	if row.PublishedAt.Valid {
		published = &row.PublishedAt.Time
	}
	return Detail{ID: row.ID, Metadata: Metadata{row.Title, row.SourceUrl.String, row.Credit.String, row.SortOrder}, Status: row.Status, Revision: row.Revision, Image: image, Preview: preview, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, PublishedAt: published}
}
func (s *Service) Detail(ctx context.Context, id uuid.UUID) (Detail, error) {
	if s.q == nil {
		return Detail{}, ErrUnavailable
	}
	row, e := s.q.Wallpaper(ctx, id)
	return s.detail(row), databaseError(e)
}

type Page struct {
	Items    []Detail `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}

func (s *Service) List(ctx context.Context, status, keyword string, page, size int) (out Page, err error) {
	out = Page{Items: []Detail{}, Page: page, PageSize: size}
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if (status != "" && status != "draft" && status != "published" && status != "archived") || !BoundedText(keyword, 160, false) || page < 1 || page > 1000 || size < 1 || size > 50 {
		return out, ErrInvalid
	}
	if s.q == nil {
		return out, ErrUnavailable
	}
	rows, e := s.q.WallpaperList(ctx, dbgen.WallpaperListParams{StatusFilter: status, Keyword: keyword, PageLimit: int32(size), PageOffset: int32((page - 1) * size)})
	if e != nil {
		return out, ErrUnavailable
	}
	out.Total, e = s.q.WallpaperCount(ctx, dbgen.WallpaperCountParams{StatusFilter: status, Keyword: keyword})
	if e != nil {
		return out, ErrUnavailable
	}
	for _, r := range rows {
		out.Items = append(out.Items, s.detail(r))
	}
	return out, nil
}
func (s *Service) Edit(ctx context.Context, actor Actor, id uuid.UUID, revision int64, m Metadata) (result Detail, err error) {
	if revision < 1 || m.Validate() != nil {
		return result, ErrInvalid
	}
	err = s.write(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		before, e := q.LockWallpaper(ctx, id)
		if e != nil {
			return databaseError(e)
		}
		if before.Revision != revision {
			return ErrConflict
		}
		row, e := q.EditWallpaper(ctx, dbgen.EditWallpaperParams{ID: id, Revision: revision, Title: m.Title, SourceUrl: pgtype.Text{String: m.SourceURL, Valid: m.SourceURL != ""}, Credit: pgtype.Text{String: m.Credit, Valid: m.Credit != ""}, SortOrder: m.SortOrder})
		if e != nil {
			return databaseError(e)
		}
		result = s.detail(row)
		return audit(ctx, q, actor, "wallpaper.edited", id, summary(before), summary(row))
	})
	if err == nil {
		s.invalidate()
	}
	return
}

type Mutation struct {
	Wallpaper Detail `json:"wallpaper"`
	Changed   bool   `json:"changed"`
}

func (s *Service) Transition(ctx context.Context, actor Actor, id uuid.UUID, revision int64, target string, confirmed bool) (result Mutation, err error) {
	if revision < 1 {
		return result, ErrInvalid
	}
	previous := ""
	action := ""
	switch target {
	case "published":
		previous = "draft"
		action = "wallpaper.published"
		if !confirmed {
			return result, ErrInvalid
		}
	case "archived":
		previous = "published"
		action = "wallpaper.archived"
	case "draft":
		previous = "archived"
		action = "wallpaper.restored"
	default:
		return result, ErrInvalid
	}
	if s.q == nil {
		return result, ErrUnavailable
	}
	before, e := s.q.Wallpaper(ctx, id)
	if e != nil {
		return result, databaseError(e)
	}
	if before.Revision != revision {
		return result, ErrConflict
	}
	if before.Status != previous && before.Status != target {
		return result, ErrConflict
	}
	image, preview := s.assets(before)
	if e = s.verify(ctx, image); e != nil {
		return result, e
	}
	if e = s.verify(ctx, preview); e != nil {
		return result, e
	}
	err = s.write(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		locked, e := q.LockWallpaper(ctx, id)
		if e != nil {
			return databaseError(e)
		}
		if locked.Revision != revision {
			return ErrConflict
		}
		if locked.Status == target {
			result.Wallpaper = s.detail(locked)
			return nil
		}
		if locked.Status != previous {
			return ErrConflict
		}
		row, e := q.TransitionWallpaper(ctx, dbgen.TransitionWallpaperParams{ID: id, Revision: revision, NextStatus: target, PreviousStatus: previous})
		if e != nil {
			return databaseError(e)
		}
		result = Mutation{Wallpaper: s.detail(row), Changed: true}
		return audit(ctx, q, actor, action, id, summary(locked), summary(row))
	})
	if err == nil {
		s.invalidate()
	}
	return
}
func (s *Service) Delete(ctx context.Context, actor Actor, id uuid.UUID, revision int64) error {
	if revision < 1 {
		return ErrInvalid
	}
	e := s.write(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		before, e := q.LockWallpaper(ctx, id)
		if e != nil {
			return databaseError(e)
		}
		if before.Revision != revision || before.Status != "draft" {
			return ErrConflict
		}
		if _, e = q.DeleteDraft(ctx, dbgen.DeleteDraftParams{ID: id, Revision: revision}); e != nil {
			return databaseError(e)
		}
		return audit(ctx, q, actor, "wallpaper.deleted", id, summary(before), Summary{})
	})
	if e == nil {
		s.invalidate()
	}
	return e
}
