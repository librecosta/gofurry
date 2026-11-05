package management

import (
	"bytes"
	json "encoding/json/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"io"
	"mime"
	"strconv"
	"uuid"
)

func body(c fiber.Ctx, value any) error {
	typ, _, e := mime.ParseMediaType(c.Get("Content-Type"))
	if e != nil || typ != "application/json" {
		return fiber.ErrUnsupportedMediaType
	}
	var r io.Reader = c.BodyStream()
	if r == nil {
		r = bytes.NewReader(c.Body())
	}
	b, e := io.ReadAll(io.LimitReader(r, 16385))
	if e != nil || len(b) > 16384 {
		return fiber.ErrRequestEntityTooLarge
	}
	if json.Unmarshal(b, value, json.RejectUnknownMembers(true)) != nil {
		return fiber.ErrBadRequest
	}
	return nil
}
func pathID(c fiber.Ctx) (uuid.UUID, error) {
	id, e := uuid.Parse(c.Params("id"))
	if e != nil || id == uuid.Nil() || id.String() != c.Params("id") {
		return id, fiber.ErrBadRequest
	}
	return id, nil
}
func registerWallpapers(app *fiber.App, service *wallpaper.Service) {
	const base = "/internal/v1/start/wallpapers"
	app.Get(base, func(c fiber.Ctx) error {
		page, size := 1, 20
		var e error
		if v := c.Query("page"); v != "" {
			page, e = strconv.Atoi(v)
			if e != nil {
				return fiber.ErrBadRequest
			}
		}
		if v := c.Query("page_size"); v != "" {
			size, e = strconv.Atoi(v)
			if e != nil {
				return fiber.ErrBadRequest
			}
		}
		out, e := service.List(c.Context(), c.Query("status"), c.Query("q"), page, size)
		if e != nil {
			return domainError(e)
		}
		return c.JSON(out)
	})
	app.Get(base+"/:id", func(c fiber.Ctx) error {
		id, e := pathID(c)
		if e != nil {
			return e
		}
		out, e := service.Detail(c.Context(), id)
		if e != nil {
			return domainError(e)
		}
		return c.JSON(out)
	})
	app.Post(base, upload(service))
	app.Patch(base+"/:id", func(c fiber.Ctx) error {
		id, e := pathID(c)
		if e != nil {
			return e
		}
		var input struct {
			wallpaper.Metadata
			Revision int64 `json:"revision"`
		}
		if e = body(c, &input); e != nil {
			return e
		}
		out, e := service.Edit(c.Context(), Current(c), id, input.Revision, input.Metadata)
		if e != nil {
			return domainError(e)
		}
		return c.JSON(out)
	})
	for _, op := range []struct{ path, target string }{{"publish", "published"}, {"archive", "archived"}, {"restore", "draft"}} {
		app.Post(base+"/:id/"+op.path, func(c fiber.Ctx) error {
			id, e := pathID(c)
			if e != nil {
				return e
			}
			var input struct {
				Revision         int64 `json:"revision"`
				ConfirmPublicUse bool  `json:"confirm_public_use"`
			}
			if e = body(c, &input); e != nil {
				return e
			}
			out, e := service.Transition(c.Context(), Current(c), id, input.Revision, op.target, input.ConfirmPublicUse)
			if e != nil {
				return domainError(e)
			}
			return c.JSON(out)
		})
	}
	app.Delete(base+"/:id", func(c fiber.Ctx) error {
		id, e := pathID(c)
		if e != nil {
			return e
		}
		var input struct {
			Revision int64 `json:"revision"`
		}
		if e = body(c, &input); e != nil {
			return e
		}
		if e = service.Delete(c.Context(), Current(c), id, input.Revision); e != nil {
			return domainError(e)
		}
		return c.SendStatus(204)
	})
}
