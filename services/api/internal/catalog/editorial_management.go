package catalog

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type RevisionedSite struct {
	PublicID string `json:"public_id"`
	Revision int64  `json:"revision"`
}

type SiteCategories struct {
	Primary       string   `json:"primary"`
	Secondary     []string `json:"secondary,omitzero"`
	KeepSecondary bool     `json:"keep_secondary,omitzero"`
}

type BulkSiteEdit struct {
	Sites      []RevisionedSite `json:"sites"`
	Categories *SiteCategories  `json:"categories,omitzero"`
}

func (s *Service) BulkEditSites(ctx context.Context, in BulkSiteEdit, region, actor string) (ManagementResult, error) {
	if len(in.Sites) == 0 || len(in.Sites) > 30 || in.Categories == nil {
		return ManagementResult{}, ErrInvalid
	}
	c := in.Categories
	// The explicit flag also prevents older Management servers from decoding
	// a primary-only edit as a category replacement.
	keepSecondary := c.Secondary == nil
	secondary, ok := navigationCodes(c.Secondary, 2, NavigationCategory)
	if !ok || c.KeepSecondary && !keepSecondary || c.Primary == "" && len(secondary) > 0 ||
		c.Primary != "" && (!NavigationCategory(c.Primary) || slices.Contains(secondary, c.Primary)) {
		return ManagementResult{}, ErrInvalid
	}
	targets := slices.Clone(in.Sites)
	slices.SortFunc(targets, func(a, b RevisionedSite) int { return strings.Compare(a.PublicID, b.PublicID) })
	for i, target := range targets {
		if !ValidPublicID(target.PublicID) || target.Revision < 1 || i > 0 && targets[i-1].PublicID == target.PublicID {
			return ManagementResult{}, ErrInvalid
		}
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		affected := 0
		for _, target := range targets {
			row, err := q.LockNavigationSite(ctx, target.PublicID)
			if err != nil {
				return ManagementResult{}, err
			}
			if err = editable(target.Revision, row.Revision, row.Status); err != nil {
				return ManagementResult{}, err
			}
			nextSecondary := secondary
			if keepSecondary {
				nextSecondary = row.SecondaryCategories
				if c.Primary == "" && len(nextSecondary) > 0 || slices.Contains(nextSecondary, c.Primary) {
					return ManagementResult{}, ErrSiteSecondaryConflict
				}
			}
			if c.Primary == row.PrimaryCategory.String && slices.Equal(nextSecondary, row.SecondaryCategories) {
				continue
			}
			edit := NavigationEdit{Rating: row.ContentRating, Primary: c.Primary, Secondary: nextSecondary,
				Tags: row.NavigationTags, Actor: actor, Reason: "Admin bulk taxonomy edit"}
			if _, err = s.InTransaction(q).ReviewSiteNavigation(ctx, region, row.PublicID, row.Revision, edit); err != nil {
				return ManagementResult{}, err
			}
			affected++
		}
		return ManagementResult{Target: "sites_" + strconv.Itoa(affected), Affected: &affected}, nil
	})
}

const maxEditorialRelations = 128

type EditorialTagAssignment struct {
	Code string `json:"code"`
	Role string `json:"role"`
}

type EditorialTagEdit struct {
	Revision int64                    `json:"revision"`
	Tags     []EditorialTagAssignment `json:"tags"`
}

type EditorialGameTags struct {
	Revision int64                         `json:"revision"`
	Tags     []dbgen.ManagementGameTagsRow `json:"tags"`
}

func (s *Service) ManagementGameTags(ctx context.Context, id string) (EditorialGameTags, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (EditorialGameTags, error) {
		row, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return EditorialGameTags{}, err
		}
		tags, err := q.ManagementGameTags(ctx, row.ID)
		return EditorialGameTags{Revision: row.Revision, Tags: tags}, err
	})
}

func editorialReference(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalid
	}
	return err
}

func (s *Service) EditManagementGameTags(ctx context.Context, id string, in EditorialTagEdit) (ManagementResult, error) {
	if in.Tags == nil || len(in.Tags) > maxEditorialRelations {
		return ManagementResult{}, ErrInvalid
	}
	desired := make(map[string]string, len(in.Tags))
	roles := map[string]bool{}
	for _, tag := range in.Tags {
		if !ValidEditorialCode(tag.Code) || desired[tag.Code] != "" ||
			tag.Role != "normal" && tag.Role != "primary" && tag.Role != "secondary" ||
			tag.Role != "normal" && roles[tag.Role] {
			return ManagementResult{}, ErrInvalid
		}
		desired[tag.Code], roles[tag.Role] = tag.Role, true
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		if err := q.LockEditorialDefinitions(ctx); err != nil {
			return ManagementResult{}, err
		}
		row, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		if err = editable(in.Revision, row.Revision, row.Status); err != nil {
			return ManagementResult{}, err
		}
		current, err := q.ManagementGameTags(ctx, row.ID)
		if err != nil {
			return ManagementResult{}, err
		}
		existing := make(map[string]string, len(current))
		changed := false
		for _, tag := range current {
			existing[tag.Code] = tag.Role
			if desired[tag.Code] == tag.Role {
				continue
			}
			changed = true
			if err = q.RemoveGameEditorialMember(ctx, dbgen.RemoveGameEditorialMemberParams{GameID: row.ID, Code: tag.Code}); err != nil {
				return ManagementResult{}, err
			}
		}
		for _, tag := range in.Tags {
			if existing[tag.Code] == tag.Role {
				continue
			}
			changed = true
			key, err := q.ResolveEditorialTag(ctx, tag.Code)
			if err != nil {
				return ManagementResult{}, editorialReference(err)
			}
			if err = q.AddGameEditorialMember(ctx, dbgen.AddGameEditorialMemberParams{GameID: row.ID, TagID: key, Role: tag.Role}); err != nil {
				return ManagementResult{}, err
			}
		}
		if changed {
			if err = q.TouchLegacyGameTags(ctx, row.ID); err != nil {
				return ManagementResult{}, err
			}
			row.Revision++
		}
		return ManagementResult{Target: row.PublicID, Revision: row.Revision}, nil
	})
}

type CollectionRules struct {
	Revision   int64                                     `json:"revision"`
	Tags       []dbgen.ManagementCollectionTagsRow       `json:"tags"`
	Exclusions []dbgen.ManagementCollectionExclusionsRow `json:"exclusions"`
}

type CollectionRulesEdit struct {
	Revision   int64     `json:"revision"`
	Tags       *[]string `json:"tags,omitzero"`
	Exclusions *[]string `json:"exclusions,omitzero"`
}

func (s *Service) ManagementCollectionRules(ctx context.Context, id string) (CollectionRules, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (CollectionRules, error) {
		row, err := q.LockGameCollection(ctx, id)
		if err != nil {
			return CollectionRules{}, err
		}
		tags, err := q.ManagementCollectionTags(ctx, row.ID)
		if err != nil {
			return CollectionRules{}, err
		}
		exclusions, err := q.ManagementCollectionExclusions(ctx, row.ID)
		return CollectionRules{Revision: row.Revision, Tags: tags, Exclusions: exclusions}, err
	})
}

func changeEditorialKeys(current, desired []string, valid func(string) bool, remove, add func(string) error) error {
	if len(desired) > maxEditorialRelations {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(desired))
	for _, code := range desired {
		if seen[code] || !valid(code) {
			return ErrInvalid
		}
		seen[code] = true
	}
	for _, code := range current {
		if !seen[code] {
			if err := remove(code); err != nil {
				return err
			}
		}
	}
	for _, code := range desired {
		if !slices.Contains(current, code) {
			if err := add(code); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) EditManagementCollectionRules(ctx context.Context, id string, in CollectionRulesEdit) (ManagementResult, error) {
	if in.Tags == nil && in.Exclusions == nil {
		return ManagementResult{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		if err := q.LockEditorialDefinitions(ctx); err != nil {
			return ManagementResult{}, err
		}
		row, err := q.LockGameCollection(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		if err = editable(in.Revision, row.Revision, row.Status); err != nil {
			return ManagementResult{}, err
		}
		if in.Tags != nil {
			if err := editCollectionTags(ctx, q, row.ID, *in.Tags); err != nil {
				return ManagementResult{}, err
			}
		}
		if in.Exclusions != nil {
			if err := editCollectionExclusions(ctx, q, row.ID, *in.Exclusions); err != nil {
				return ManagementResult{}, err
			}
		}
		row, err = q.LockGameCollection(ctx, id)
		return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
	})
}

func editCollectionTags(ctx context.Context, q *dbgen.Queries, id uuid.UUID, desired []string) error {
	rows, err := q.ManagementCollectionTags(ctx, id)
	if err != nil {
		return err
	}
	current := make([]string, 0, len(rows))
	for _, row := range rows {
		current = append(current, row.Code)
	}
	return changeEditorialKeys(current, desired, ValidEditorialCode,
		func(code string) error {
			return q.RemoveGameCollectionRule(ctx, dbgen.RemoveGameCollectionRuleParams{CollectionID: id, Code: code})
		}, func(code string) error {
			tag, err := q.ResolveEditorialTag(ctx, code)
			if err != nil {
				return editorialReference(err)
			}
			return q.AddGameCollectionRule(ctx, dbgen.AddGameCollectionRuleParams{CollectionID: id, TagID: tag})
		})
}

func editCollectionExclusions(ctx context.Context, q *dbgen.Queries, id uuid.UUID, desired []string) error {
	rows, err := q.ManagementCollectionExclusions(ctx, id)
	if err != nil {
		return err
	}
	current := make([]string, 0, len(rows))
	for _, row := range rows {
		current = append(current, row.PublicID)
	}
	return changeEditorialKeys(current, desired, ValidPublicID,
		func(value string) error {
			return q.RemoveGameCollectionExclusion(ctx, dbgen.RemoveGameCollectionExclusionParams{CollectionID: id, PublicID: value})
		}, func(value string) error {
			game, err := q.ResolveEditorialGame(ctx, value)
			if err != nil {
				return editorialReference(err)
			}
			return q.AddGameCollectionExclusion(ctx, dbgen.AddGameCollectionExclusionParams{CollectionID: id, GameID: game})
		})
}
