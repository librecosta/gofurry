package handlers

import (
	"context"
	_ "embed"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

// The small, reviewed public SFW catalog is versioned metadata, not image bytes.
//
//go:embed backgrounds.json
var backgroundCatalog []byte

type backgroundAsset struct {
	storage.Object
	Width  int `json:"width"`
	Height int `json:"height"`
}

type background struct {
	ID        string          `json:"id"`
	SFW       bool            `json:"sfw"`
	SourceURL string          `json:"source_url"`
	Preview   backgroundAsset `json:"preview"`
	Image     backgroundAsset `json:"image"`
}

func Backgrounds(store *storage.Store) fiber.Handler {
	var catalog []background
	if err := platform.UnmarshalJSON(backgroundCatalog, &catalog); err != nil {
		panic("invalid embedded background catalog")
	}
	return func(c fiber.Ctx) error {
		items := make([]background, 0, len(catalog))
		if store != nil {
			ctx, cancel := context.WithTimeout(c.Context(), 8*time.Second)
			defer cancel()
			for _, item := range catalog {
				if !item.SFW {
					return fiber.ErrServiceUnavailable
				}
				for _, asset := range []*backgroundAsset{&item.Preview, &item.Image} {
					head, err := store.Head(ctx, asset.Key)
					if err != nil || head.SHA256 != asset.SHA256 || head.Bytes != asset.Bytes || head.ContentType != asset.ContentType {
						return fiber.ErrServiceUnavailable
					}
					asset.Object = head
				}
				items = append(items, item)
			}
		}
		c.Set(fiber.HeaderCacheControl, "public, max-age=300")
		return c.JSON(fiber.Map{"backgrounds": items})
	}
}
