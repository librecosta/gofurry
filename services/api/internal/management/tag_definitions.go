package management

import (
	"context"
	"encoding/json/jsontext"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type editorialDefinition struct {
	Code          string         `json:"code"`
	Localizations jsontext.Value `json:"localizations"`
	Archived      bool           `json:"archived"`
}

type editorialCategory struct {
	editorialDefinition
	SortOrder int32 `json:"sort_order"`
}

type editorialTag struct {
	editorialDefinition
	Category         string `json:"category"`
	CategoryName     string `json:"category_name"`
	CategoryArchived bool   `json:"category_archived"`
}

type editorialCode struct {
	Code string `json:"code"`
}

func editorialFilters(c fiber.Ctx, tags bool) (url.Values, error) {
	params := []string{"q", "state", "cursor"}
	if tags {
		params = append(params, "category")
	}
	values, err := search(c, params...)
	if err != nil {
		return nil, err
	}
	if state := values.Get("state"); state != "" && state != "active" && state != "archived" {
		return nil, fiber.ErrBadRequest
	}
	if cursor := values.Get("cursor"); cursor != "" && !catalog.ValidEditorialCode(cursor) {
		return nil, fiber.ErrBadRequest
	}
	if category := values.Get("category"); category != "" && !catalog.ValidEditorialCode(category) {
		return nil, fiber.ErrBadRequest
	}
	return values, nil
}

func (o *Operations) editorialDefinitions(g fiber.Router) {
	const tags, categories = "/games/editorial-tags", "/games/editorial-tag-categories"
	g.Get(tags, func(c fiber.Ctx) error {
		v, err := search(c, "q")
		if err != nil {
			return err
		}
		items, err := dbgen.New(o.Pool).SearchEditorialTags(c.Context(), v.Get("q"))
		if err != nil {
			return failure(err)
		}
		return c.JSON(items)
	})
	g.Get(categories, func(c fiber.Ctx) error {
		v, err := search(c, "q", "include_archived")
		if err != nil {
			return err
		}
		if option := v.Get("include_archived"); option != "" && option != "true" {
			return fiber.ErrBadRequest
		}
		items, err := dbgen.New(o.Pool).ListEditorialTagCategories(c.Context(), dbgen.ListEditorialTagCategoriesParams{
			Keyword: v.Get("q"), IncludeArchived: v.Get("include_archived") == "true",
		})
		if err != nil {
			return failure(err)
		}
		return c.JSON(items)
	})
	g.Get(tags+"/manage", o.managedEditorialTags)
	g.Get(categories+"/manage", o.managedEditorialCategories)
	g.Post(tags, editOnly, writeOperation(o, "game.tag.create", func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, _ string, in catalog.EditorialTagDefinition, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
		return s.CreateManagementEditorialTag(ctx, in)
	}))
	g.Patch(tags+"/:code", editOnly, writeOperation(o, "game.tag.edit", func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, code string, in catalog.EditorialTagEditDefinition, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
		return s.EditManagementEditorialTag(ctx, code, in)
	}))
	g.Post(categories, editOnly, writeOperation(o, "game.tag_category.create", func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, _ string, in catalog.EditorialCategoryDefinition, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
		return s.CreateManagementEditorialCategory(ctx, in)
	}))
	g.Patch(categories+"/:code", editOnly, writeOperation(o, "game.tag_category.edit", func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, code string, in catalog.EditorialCategoryEdit, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
		return s.EditManagementEditorialCategory(ctx, code, in)
	}))
	for _, action := range []string{"archive", "restore"} {
		archived := action == "archive"
		g.Post(tags+"/"+action, editOnly, writeOperation(o, "game.tag."+action, func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, _ string, in editorialCode, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return s.SetManagementEditorialTagArchived(ctx, in.Code, archived)
		}))
		g.Post(categories+"/"+action, editOnly, writeOperation(o, "game.tag_category."+action, func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, _ string, in editorialCode, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return s.SetManagementEditorialCategoryArchived(ctx, in.Code, archived)
		}))
	}
}

func (o *Operations) managedEditorialTags(c fiber.Ctx) error {
	v, err := editorialFilters(c, true)
	if err != nil {
		return err
	}
	rows, err := dbgen.New(o.Pool).ListManagedEditorialTags(c.Context(), dbgen.ListManagedEditorialTagsParams{
		Keyword: v.Get("q"), CursorCode: v.Get("cursor"), State: v.Get("state"), Category: v.Get("category"),
	})
	items := make([]editorialTag, 0, len(rows))
	for _, row := range rows {
		items = append(items, editorialTag{editorialDefinition{row.Code, row.Localizations, row.Archived}, row.Category, row.CategoryName, row.CategoryArchived})
	}
	return page(c, items, err, func(row editorialTag) string { return row.Code })
}

func (o *Operations) managedEditorialCategories(c fiber.Ctx) error {
	v, err := editorialFilters(c, false)
	if err != nil {
		return err
	}
	rows, err := dbgen.New(o.Pool).ListManagedEditorialCategories(c.Context(), dbgen.ListManagedEditorialCategoriesParams{
		Keyword: v.Get("q"), CursorCode: v.Get("cursor"), State: v.Get("state"),
	})
	items := make([]editorialCategory, 0, len(rows))
	for _, row := range rows {
		items = append(items, editorialCategory{editorialDefinition{row.Code, row.Localizations, row.Archived}, row.SortOrder})
	}
	return page(c, items, err, func(row editorialCategory) string { return row.Code })
}
