package wallpaper

import (
	"context"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"time"
	"uuid"
)

type PublicBackground struct {
	ID        uuid.UUID `json:"id"`
	SFW       bool      `json:"sfw"`
	SourceURL string    `json:"source_url"`
	Preview   Asset     `json:"preview"`
	Image     Asset     `json:"image"`
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
	s.cacheAt = time.Time{}
	s.generation++
}
func (s *Service) Catalog(ctx context.Context) ([]PublicBackground, error) {
	empty := []PublicBackground{}
	if !s.configured {
		return empty, nil
	}
	if s.pool == nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// Even a fresh cache must not mask an observed database outage.
	if s.pool.Ping(ctx) != nil {
		s.invalidate()
		return nil, ErrUnavailable
	}
	s.cacheMu.Lock()
	generation := s.generation
	if !s.cacheAt.IsZero() && time.Since(s.cacheAt) < time.Minute {
		out := append(empty, s.cache...)
		s.cacheMu.Unlock()
		return out, nil
	}
	s.cacheMu.Unlock()
	rows, e := s.q.PublishedWallpapers(ctx)
	if e != nil {
		s.invalidate()
		return nil, ErrUnavailable
	}
	if len(rows) > 1000 {
		return nil, ErrUnavailable
	}
	out := make([]PublicBackground, 0, len(rows))
	for _, row := range rows {
		image, preview := s.assets(row)
		if image.URL == "" || preview.URL == "" {
			return nil, ErrUnavailable
		}
		out = append(out, PublicBackground{ID: row.ID, SFW: true, SourceURL: row.SourceUrl.String, Image: image, Preview: preview})
	}
	s.cacheMu.Lock()
	if s.generation == generation {
		s.cache = append(empty, out...)
		s.cacheAt = time.Now()
	}
	s.cacheMu.Unlock()
	return out, nil
}
