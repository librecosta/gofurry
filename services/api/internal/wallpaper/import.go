package wallpaper

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"regexp"
	"uuid"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type LegacyBackground struct {
	ID        string `json:"id"`
	SFW       bool   `json:"sfw"`
	SourceURL string `json:"source_url"`
	Preview   Asset  `json:"preview"`
	Image     Asset  `json:"image"`
}

func ReadLegacy(r io.Reader) ([]LegacyBackground, error) {
	data, e := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if e != nil || len(data) > 1<<20 {
		return nil, ErrInvalid
	}
	var items []LegacyBackground
	if json.Unmarshal(data, &items, json.RejectUnknownMembers(true)) != nil || len(items) == 0 || len(items) > 1000 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !item.SFW || !BoundedText(item.ID, 160, true) || !assetValid(item.Image, false) || !assetValid(item.Preview, true) || seen[item.Image.Key] {
			return nil, ErrInvalid
		}
		seen[item.Image.Key] = true
		m := Metadata{Title: "Imported official wallpaper", SourceURL: item.SourceURL}
		if m.Validate() != nil {
			return nil, ErrInvalid
		}
	}
	return items, nil
}
func assetValid(a Asset, preview bool) bool {
	maxBytes := MaxImageBytes
	maxPixels := int64(36000000)
	maxSide := 16384
	if preview {
		maxBytes = MaxPreviewBytes
		maxPixels = 4000000
		maxSide = 4096
	}
	return hashPattern.MatchString(a.SHA256) && (a.ContentType == "image/avif" || a.ContentType == "image/webp") && (!preview || a.ContentType == "image/webp") && a.Key == "start/backgrounds/"+a.SHA256+"."+a.ContentType[6:] && a.Bytes > 0 && a.Bytes <= maxBytes && a.Width > 0 && a.Height > 0 && a.Width <= maxSide && a.Height <= maxSide && int64(a.Width)*int64(a.Height) <= maxPixels && a.URL == ""
}
func (s *Service) checkStoredImage(ctx context.Context, asset Asset, preview bool) error {
	if e := s.verify(ctx, asset); e != nil {
		return e
	}
	body, _, e := s.objects.Get(ctx, asset.Key)
	if e != nil {
		return ErrUnavailable
	}
	defer body.Close()
	f, e := os.CreateTemp("", "gofurry-wallpaper-import-*")
	if e != nil {
		return ErrUnavailable
	}
	defer func() { name := f.Name(); f.Close(); os.Remove(name) }()
	n, e := io.Copy(f, io.LimitReader(body, asset.Bytes+1))
	if e != nil || n != asset.Bytes {
		return ErrUnavailable
	}
	got, e := ValidateAsset(ctx, f, asset.ContentType, preview)
	if e != nil || got.Key != asset.Key || got.Width != asset.Width || got.Height != asset.Height || got.Bytes != asset.Bytes {
		return ErrInvalid
	}
	return nil
}
func (s *Service) Import(ctx context.Context, items []LegacyBackground) (created, skipped int, err error) {
	if s.q == nil || s.objects == nil {
		return 0, 0, ErrUnavailable
	}
	// No PUT path exists in this import. A rerun preserves any existing state,
	// including an administrator's later archive or draft; it cannot republish it.
	for _, item := range items {
		if !item.SFW || !assetValid(item.Image, false) || !assetValid(item.Preview, true) {
			return created, skipped, ErrInvalid
		}
		if e := s.checkStoredImage(ctx, item.Image, false); e != nil {
			return created, skipped, e
		}
		if e := s.checkStoredImage(ctx, item.Preview, true); e != nil {
			return created, skipped, e
		}
		existing, e := s.q.WallpaperByImage(ctx, item.Image.Key)
		if e == nil {
			if existing.PreviewKey != item.Preview.Key || existing.ImageSha256 != item.Image.SHA256 || existing.ImageBytes != item.Image.Bytes || existing.ImageType != item.Image.ContentType || existing.Width != int32(item.Image.Width) || existing.Height != int32(item.Image.Height) || existing.PreviewSha256 != item.Preview.SHA256 || existing.PreviewBytes != item.Preview.Bytes || existing.PreviewWidth != int32(item.Preview.Width) || existing.PreviewHeight != int32(item.Preview.Height) {
				return created, skipped, ErrConflict
			}
			skipped++
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return created, skipped, ErrUnavailable
		}
		actor := Actor{Name: "Development import", Role: "system", RequestID: uuid.NewV7().String()}
		m := Metadata{Title: "Imported official wallpaper", SourceURL: item.SourceURL}
		if e = m.Validate(); e != nil {
			return created, skipped, e
		}
		if _, e = s.create(ctx, actor, m, item.Image, item.Preview, "published", "wallpaper.imported"); e != nil {
			return created, skipped, e
		}
		created++
		s.invalidate()
	}
	return
}
func (s *Service) ImportObject(ctx context.Context, file *os.File, typ string) (Asset, error) {
	a, e := ValidateAsset(ctx, file, typ, false)
	if e != nil {
		return a, e
	}
	if s.objects == nil {
		return a, ErrUnavailable
	}
	if e = s.ensure(ctx, a, file); e != nil {
		return Asset{}, e
	}
	a.URL = s.objects.URL(a.Key)
	return a, nil
}
