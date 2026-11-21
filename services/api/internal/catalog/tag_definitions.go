package catalog

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type editorialConflict string

func (e editorialConflict) Error() string { return string(e) }
func (e editorialConflict) Unwrap() error { return ErrConflict }

const (
	ErrEditorialExists           = editorialConflict("editorial definition already exists")
	ErrEditorialCategoryArchived = editorialConflict("editorial category is archived")
	ErrEditorialCategoryInUse    = editorialConflict("editorial category has active tags")
	ErrEditorialTagInUse         = editorialConflict("editorial tag is assigned to games or collection rules")
)

type EditorialTagEditDefinition struct {
	Category      string         `json:"category"`
	Localizations []Localization `json:"localizations"`
}

type EditorialTagDefinition struct {
	EditorialTagEditDefinition
	Code string `json:"code,omitempty"`
}

type EditorialCategoryEdit struct {
	Localizations []Localization `json:"localizations"`
	SortOrder     int32          `json:"sort_order"`
}

type EditorialCategoryDefinition struct {
	EditorialCategoryEdit
	Code string `json:"code,omitempty"`
}

func ValidEditorialCode(code string) bool { return len(code) <= 64 && slugPattern.MatchString(code) }

func editorialDefinitionText(input []Localization) ([]Localization, []byte, error) {
	text, err := normalizeLocalizations(input)
	if err != nil || len(text) == 0 {
		return nil, nil, ErrInvalid
	}
	names, err := json.Marshal(text)
	return text, names, err
}

func editorialDefinitionCode(code, prefix string, id uuid.UUID, text []Localization) (string, error) {
	if code != "" {
		if !ValidEditorialCode(code) {
			return "", ErrInvalid
		}
		return code, nil
	}
	for _, item := range text {
		if item.Locale == "en" && latinLabel.MatchString(item.Name) {
			candidate := strings.ToLower(strings.Join(strings.FieldsFunc(item.Name, func(r rune) bool {
				return r == ' ' || r == '-' || r == '_'
			}), "-"))
			if ValidEditorialCode(candidate) {
				return candidate, nil
			}
		}
	}
	return prefix + "-" + id.String(), nil
}

func editorialResult(code string, err error) (ManagementResult, error) {
	return ManagementResult{Target: strings.ReplaceAll(code, "-", "_")}, err
}

// Definitions, game assignments and collection rules take this lock before
// any row locks. An archive check cannot race a new reference, and trigger
// updates to collections follow the same lock order for these operations.
func writeEditorialDefinition(ctx context.Context, s *Service, code string, change func(context.Context, *dbgen.Queries) error) (ManagementResult, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		if err := q.LockEditorialDefinitions(ctx); err != nil {
			return ManagementResult{}, err
		}
		return editorialResult(code, change(ctx, q))
	})
}

func (s *Service) CreateManagementEditorialCategory(ctx context.Context, in EditorialCategoryDefinition) (ManagementResult, error) {
	text, names, err := editorialDefinitionText(in.Localizations)
	if err != nil {
		return ManagementResult{}, err
	}
	id := uuid.NewV7()
	code, err := editorialDefinitionCode(in.Code, "category", id, text)
	if err != nil {
		return ManagementResult{}, err
	}
	return writeEditorialDefinition(ctx, s, code, func(ctx context.Context, q *dbgen.Queries) error {
		_, err := q.CreateEditorialCategory(ctx, dbgen.CreateEditorialCategoryParams{
			ID: id, Code: code, Names: names, SortOrder: in.SortOrder, CheckNames: in.Code == "",
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEditorialExists
		}
		return err
	})
}

func (s *Service) EditManagementEditorialCategory(ctx context.Context, code string, in EditorialCategoryEdit) (ManagementResult, error) {
	_, names, err := editorialDefinitionText(in.Localizations)
	if err != nil {
		return ManagementResult{}, err
	}
	return writeEditorialDefinition(ctx, s, code, func(ctx context.Context, q *dbgen.Queries) error {
		category, err := q.LockEditorialCategory(ctx, code)
		if err != nil {
			return err
		}
		return q.UpdateEditorialCategory(ctx, dbgen.UpdateEditorialCategoryParams{ID: category.ID, Localizations: names, SortOrder: in.SortOrder})
	})
}

func (s *Service) SetManagementEditorialCategoryArchived(ctx context.Context, code string, archived bool) (ManagementResult, error) {
	if !ValidEditorialCode(code) {
		return ManagementResult{}, ErrInvalid
	}
	return writeEditorialDefinition(ctx, s, code, func(ctx context.Context, q *dbgen.Queries) error {
		category, err := q.LockEditorialCategory(ctx, code)
		if err != nil || category.ArchivedAt.Valid == archived {
			return err
		}
		if archived {
			inUse, err := q.EditorialCategoryHasTags(ctx, category.ID)
			if err != nil {
				return err
			}
			if inUse {
				return ErrEditorialCategoryInUse
			}
		}
		return q.SetEditorialCategoryArchived(ctx, dbgen.SetEditorialCategoryArchivedParams{ID: category.ID, Archived: archived})
	})
}

func (s *Service) CreateManagementEditorialTag(ctx context.Context, in EditorialTagDefinition) (ManagementResult, error) {
	if !ValidEditorialCode(in.Category) {
		return ManagementResult{}, ErrInvalid
	}
	text, names, err := editorialDefinitionText(in.Localizations)
	if err != nil {
		return ManagementResult{}, err
	}
	id := uuid.NewV7()
	code, err := editorialDefinitionCode(in.Code, "tag", id, text)
	if err != nil {
		return ManagementResult{}, err
	}
	return writeEditorialDefinition(ctx, s, code, func(ctx context.Context, q *dbgen.Queries) error {
		category, err := q.LockEditorialCategory(ctx, in.Category)
		if err != nil {
			return err
		}
		if category.ArchivedAt.Valid {
			return ErrEditorialCategoryArchived
		}
		_, err = q.CreateEditorialTag(ctx, dbgen.CreateEditorialTagParams{
			TagID: id, TagCode: code, CategoryID: category.ID, Names: names, CheckNames: in.Code == "",
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEditorialExists
		}
		return err
	})
}

func (s *Service) EditManagementEditorialTag(ctx context.Context, code string, in EditorialTagEditDefinition) (ManagementResult, error) {
	if !ValidEditorialCode(in.Category) {
		return ManagementResult{}, ErrInvalid
	}
	_, names, err := editorialDefinitionText(in.Localizations)
	if err != nil {
		return ManagementResult{}, err
	}
	return writeEditorialDefinition(ctx, s, code, func(ctx context.Context, q *dbgen.Queries) error {
		tag, err := q.LockEditorialTag(ctx, code)
		if err != nil {
			return err
		}
		category, err := q.LockEditorialCategory(ctx, in.Category)
		if err != nil {
			return err
		}
		if tag.CategoryID != category.ID && category.ArchivedAt.Valid {
			return ErrEditorialCategoryArchived
		}
		return q.UpdateEditorialTag(ctx, dbgen.UpdateEditorialTagParams{ID: tag.ID, CategoryID: category.ID, Localizations: names})
	})
}

func (s *Service) SetManagementEditorialTagArchived(ctx context.Context, code string, archived bool) (ManagementResult, error) {
	if !ValidEditorialCode(code) {
		return ManagementResult{}, ErrInvalid
	}
	return writeEditorialDefinition(ctx, s, code, func(ctx context.Context, q *dbgen.Queries) error {
		tag, err := q.LockEditorialTag(ctx, code)
		if err != nil || tag.ArchivedAt.Valid == archived {
			return err
		}
		if archived {
			inUse, err := q.EditorialTagIsReferenced(ctx, tag.ID)
			if err != nil {
				return err
			}
			if inUse {
				return ErrEditorialTagInUse
			}
		} else {
			parentArchived, err := q.EditorialTagCategoryArchived(ctx, tag.CategoryID)
			if err != nil {
				return err
			}
			if parentArchived {
				return ErrEditorialCategoryArchived
			}
		}
		return q.SetEditorialTagArchived(ctx, dbgen.SetEditorialTagArchivedParams{ID: tag.ID, Archived: archived})
	})
}
