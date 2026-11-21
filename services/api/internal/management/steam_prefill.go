package management

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type steamPrefillInput struct {
	AppID string `json:"app_id"`
	Retry bool   `json:"retry,omitempty"`
}

func (o *Operations) steamPrefill(g fiber.Router) {
	const path = "/games/steam-prefill"
	g.Get(path, editOnly, func(c fiber.Ctx) error {
		v, err := search(c, "app_id")
		if err != nil || !validAppID(v.Get("app_id")) {
			return fiber.ErrBadRequest
		}
		result, err := o.Collector.SteamPrefill(c.Context(), dbgen.New(o.Pool), v.Get("app_id"), o.Region)
		if err != nil {
			return failure(err)
		}
		return c.JSON(result)
	})
	g.Post(path, editOnly, func(c fiber.Ctx) error {
		var in steamPrefillInput
		if err := body(c, &in); err != nil {
			return err
		}
		result, err := o.Catalog.Manage(c.Context(), receipt(c, "game.steam_prefill"), func(ctx context.Context, _ *catalog.Service, q *dbgen.Queries) (catalog.ManagementResult, error) {
			return catalog.ManagementResult{Target: in.AppID}, o.Collector.AdmitSteamPrefill(ctx, q, in.AppID)
		})
		if err != nil {
			return failure(err)
		}
		ctx, cancel := context.WithTimeout(c.Context(), 7*time.Second)
		defer cancel()
		// The receipt confirms admission. Unknown remote submissions retain the same
		// durable generation and can be checked or resumed from the form.
		_ = o.Collector.KickSteamPrefill(ctx, dbgen.New(o.Pool), in.AppID, o.Region, in.Retry)
		return c.JSON(result)
	})
}
