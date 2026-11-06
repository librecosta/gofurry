package wallpaper

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"strings"
	"time"
	"uuid"
)

type PublicBackground struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Credit      string    `json:"credit"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	SFW         bool      `json:"sfw"`
	SourceURL   string    `json:"source_url"`
	Preview     Asset     `json:"preview"`
	Image       Asset     `json:"image"`
}

func (s *Service) assets(row dbgen.StartWallpaper) (Asset, Asset) {
	image := Asset{Key: row.ImageKey, SHA256: row.ImageSha256, Bytes: row.ImageBytes, ContentType: row.ImageType, Width: int(row.Width), Height: int(row.Height)}
	preview := Asset{Key: row.PreviewKey, SHA256: row.PreviewSha256, Bytes: row.PreviewBytes, ContentType: "image/webp", Width: int(row.PreviewWidth), Height: int(row.PreviewHeight)}
	if s.objects != nil {
		image.URL = s.objects.URL(image.Key)
		preview.URL = s.objects.URL(preview.Key)
	}
	return image, preview
}
func (s *Service) invalidate() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = nil
	s.generation++
}
func (s *Service) Catalog(ctx context.Context) ([]PublicBackground, error) {
	page, err := s.PublicPage(ctx, 1, 9, "")
	return page.Backgrounds, err
}

type PublicPage struct {
	Backgrounds []PublicBackground `json:"backgrounds"`
	Total       int64              `json:"total"`
	Page        int                `json:"page"`
	PageSize    int                `json:"page_size"`
}
type catalogKey struct {
	page, size int
	query      string
}
type catalogEntry struct {
	page PublicPage
	at   time.Time
}

func (s *Service) public(row dbgen.StartWallpaper) (PublicBackground, error) {
	image, preview := s.assets(row)
	if image.URL == "" || preview.URL == "" {
		return PublicBackground{}, ErrUnavailable
	}
	return PublicBackground{ID: row.ID, SFW: true, Title: row.Title, Credit: row.Credit.String, CreatedAt: row.CreatedAt.Time, PublishedAt: row.PublishedAt.Time, SourceURL: row.SourceUrl.String, Image: image, Preview: preview}, nil
}

func (s *Service) PublicPage(ctx context.Context, page, size int, query string) (PublicPage, error) {
	out := PublicPage{Backgrounds: []PublicBackground{}, Page: page, PageSize: size}
	if page < 1 || page > 1000 || size < 1 || size > 9 || !BoundedText(query, 128, false) {
		return out, ErrInvalid
	}
	key := catalogKey{page, size, strings.ToLower(strings.TrimSpace(query))}
	if !s.configured {
		return out, nil
	}
	if s.pool == nil {
		return out, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// Even a fresh cache must not mask an observed database outage.
	if s.pool.Ping(ctx) != nil {
		s.invalidate()
		return out, ErrUnavailable
	}
	s.cacheMu.Lock()
	generation := s.generation
	if entry, ok := s.cache[key]; ok && time.Since(entry.at) < time.Minute {
		out = entry.page
		out.Backgrounds = append([]PublicBackground{}, out.Backgrounds...)
		s.cacheMu.Unlock()
		return out, nil
	}
	s.cacheMu.Unlock()
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	out.Total, e = q.PublishedWallpaperCount(ctx, key.query)
	if e != nil {
		s.invalidate()
		return out, ErrUnavailable
	}
	rows, e := q.PublishedWallpapers(ctx, dbgen.PublishedWallpapersParams{Keyword: key.query, PageLimit: int32(size), PageOffset: int32((page - 1) * size)})
	if e != nil {
		s.invalidate()
		return out, ErrUnavailable
	}
	for _, row := range rows {
		item, err := s.public(row)
		if err != nil {
			return out, err
		}
		out.Backgrounds = append(out.Backgrounds, item)
	}
	if tx.Commit(ctx) != nil {
		s.invalidate()
		return out, ErrUnavailable
	}
	s.cacheMu.Lock()
	if s.generation == generation {
		if s.cache == nil {
			s.cache = make(map[catalogKey]catalogEntry)
		}
		for k, entry := range s.cache {
			if time.Since(entry.at) >= time.Minute {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= 64 {
			for k := range s.cache {
				delete(s.cache, k)
				break
			}
		}
		s.cache[key] = catalogEntry{out, time.Now()}
	}
	s.cacheMu.Unlock()
	return out, nil
}

// Detail and random always read Published rows. A genuine miss is distinct from an outage.
func (s *Service) PublicOne(ctx context.Context, id uuid.UUID, random bool) (PublicBackground, error) {
	if !s.configured {
		return PublicBackground{}, ErrNotFound
	}
	if s.pool == nil {
		return PublicBackground{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var row dbgen.StartWallpaper
	var err error
	if random {
		row, err = s.q.RandomWallpaper(ctx, id)
	} else {
		row, err = s.q.PublicWallpaper(ctx, id)
	}
	if err != nil {
		return PublicBackground{}, databaseError(err)
	}
	return s.public(row)
}
