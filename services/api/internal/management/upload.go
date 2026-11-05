package management

import (
	"bytes"
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strconv"
)

func domainError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, wallpaper.ErrInvalid):
		return fiber.ErrBadRequest
	case errors.Is(err, wallpaper.ErrConflict):
		return fiber.ErrConflict
	case errors.Is(err, wallpaper.ErrNotFound):
		return fiber.ErrNotFound
	case errors.Is(err, wallpaper.ErrBusy):
		return fiber.ErrTooManyRequests
	default:
		return fiber.ErrServiceUnavailable
	}
}
func upload(service *wallpaper.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		release, e := service.AcquireUpload()
		if e != nil {
			return domainError(e)
		}
		defer release()
		typ, params, e := mime.ParseMediaType(c.Get("Content-Type"))
		if e != nil || typ != "multipart/form-data" || params["boundary"] == "" || len(params["boundary"]) > 70 {
			return fiber.ErrUnsupportedMediaType
		}
		const maxBody = 64 << 20
		if c.Request().Header.ContentLength() > maxBody {
			return fiber.ErrRequestEntityTooLarge
		}
		var source io.Reader = c.BodyStream()
		if source == nil {
			source = bytes.NewReader(c.Body())
		}
		limited := &io.LimitedReader{R: source, N: maxBody + 1}
		reader := multipart.NewReader(limited, params["boundary"])
		files := map[string]*os.File{}
		types := map[string]string{}
		fields := map[string]string{}
		defer func() {
			for _, f := range files {
				name := f.Name()
				f.Close()
				os.Remove(name)
			}
		}()
		count := 0
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fiber.ErrBadRequest
			}
			count++
			if count > 7 {
				part.Close()
				return fiber.ErrBadRequest
			}
			name := part.FormName()
			if name == "image" || name == "preview" {
				if files[name] != nil || part.FileName() == "" {
					part.Close()
					return fiber.ErrBadRequest
				}
				limit := wallpaper.MaxImageBytes
				if name == "preview" {
					limit = wallpaper.MaxPreviewBytes
				}
				f, e := os.CreateTemp("", "gofurry-wallpaper-*")
				if e != nil {
					part.Close()
					return fiber.ErrServiceUnavailable
				}
				files[name] = f
				n, e := io.Copy(f, io.LimitReader(part, limit+1))
				part.Close()
				if n > limit {
					return fiber.ErrRequestEntityTooLarge
				}
				if e != nil || n == 0 || c.Context().Err() != nil {
					return fiber.ErrBadRequest
				}
				ct, _, e := mime.ParseMediaType(part.Header.Get("Content-Type"))
				if e != nil {
					return fiber.ErrBadRequest
				}
				types[name] = ct
			} else {
				if name != "title" && name != "source_url" && name != "credit" && name != "sort_order" {
					part.Close()
					return fiber.ErrBadRequest
				}
				if _, ok := fields[name]; ok {
					part.Close()
					return fiber.ErrBadRequest
				}
				b, e := io.ReadAll(io.LimitReader(part, 2049))
				part.Close()
				if e != nil || len(b) > 2048 || part.FileName() != "" {
					return fiber.ErrBadRequest
				}
				fields[name] = string(b)
			}
			if limited.N <= 0 {
				return fiber.ErrRequestEntityTooLarge
			}
		}
		// Consume epilogue too: chunked requests cannot bypass the full body bound.
		if _, e = io.Copy(io.Discard, limited); e != nil {
			return fiber.ErrBadRequest
		}
		if limited.N <= 0 {
			return fiber.ErrRequestEntityTooLarge
		}
		if files["image"] == nil || files["preview"] == nil {
			return fiber.ErrBadRequest
		}
		var sort int64
		if fields["sort_order"] != "" {
			sort, e = strconv.ParseInt(fields["sort_order"], 10, 32)
			if e != nil {
				return fiber.ErrBadRequest
			}
		}
		row, e := service.Upload(c.Context(), Current(c), wallpaper.Metadata{Title: fields["title"], SourceURL: fields["source_url"], Credit: fields["credit"], SortOrder: int32(sort)}, files["image"], files["preview"], types["image"], types["preview"])
		if e != nil {
			return domainError(e)
		}
		return c.Status(201).JSON(fiber.Map{"id": row.ID, "status": row.Status, "revision": row.Revision})
	}
}
