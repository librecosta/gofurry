package legacyimport

import (
	"context"
	"errors"
	"slices"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func lockTarget(ctx context.Context, q *dbgen.Queries, k key, id uuid.UUID) error {
	switch k.Table {
	case sites:
		_, err := q.LockSite(ctx, id)
		return err
	case games:
		_, err := q.LockGame(ctx, id)
		return err
	case collections:
		_, err := q.LockLegacyCollection(ctx, id)
		return err
	case categories:
		_, err := q.LockLegacyTagCategory(ctx, id)
		return err
	case tags:
		_, err := q.LockLegacyTag(ctx, id)
		return err
	default:
		return catalog.ErrInvalid
	}
}

func (e *engine) identity(q *dbgen.Queries, it intent, p planItem, sha string) (string, *targetState, error) {
	if p.Action == "adopt" {
		if p.Expected == nil || it.Link == nil || p.Expected.ID != it.Link.ID {
			return "", nil, conflict("invalid_link_plan")
		}
		if err := lockTarget(e.ctx, q, it.Key, p.Expected.ID); err != nil {
			return "", nil, err
		}
	}

	// Older administration code accepts noncanonical itch keys. Serialize
	// this narrow claim against concurrent INSERT/UPDATE before rechecking
	// normalized ownership. Canonical Steam claims use the unique key.
	if slices.ContainsFunc(it.Sources, func(s catalog.Source) bool { return s.PlatformCode == "itch_io" }) {
		if err := q.LockLegacyItchSourceClaims(e.ctx); err != nil {
			return "", nil, err
		}
	}
	action, state, code, err := assess(e.ctx, q, it)
	if err != nil {
		return "", state, err
	}
	if action != p.Action || state != nil && (p.Expected == nil || *state != *p.Expected) {
		if code == "" {
			code = "target_changed_after_plan"
		}
		return "", state, conflict(code)
	}
	origin := "created"
	switch action {
	case "adopt":
		origin = "linked"
		if it.Alias {
			origin = "alias"
		}
	case "create":
		id, err := e.create(q, it)
		if err != nil {
			return "", nil, err
		}
		created, err := readTarget(e.ctx, q, it.Key, id)
		if err != nil {
			return "", nil, err
		}
		state = &created
	default:
		return "", state, conflict("unexpected_identity_plan")
	}
	linkProof := it.LinkProof
	if origin == "created" {
		linkProof = proof{"legacy_import", e.decisions}
	}
	if err = e.addMapping(q, it, *state, origin, linkProof); err != nil {
		return "", state, err
	}
	return origin, state, e.receipt(q, it, "identity", sha, origin, *state)
}

func (e *engine) addMapping(q *dbgen.Queries, it intent, state targetState, origin string, review proof) error {
	args := dbgen.AddLegacySiteMappingParams{LegacyDeployment: deployment, LegacyID: it.Key.first(), TargetID: state.ID,
		PublicIDAtLink: state.PublicID, FirstPlanSha256: e.planSHA, Origin: origin, DecisionSha256: review.Ref, VerifiedBy: review.Actor}
	switch it.Key.Table {
	case sites:
		return q.AddLegacySiteMapping(e.ctx, args)
	case games:
		return q.AddLegacyGameMapping(e.ctx, dbgen.AddLegacyGameMappingParams(args))
	default:
		return q.AddLegacyEditorialMapping(e.ctx, dbgen.AddLegacyEditorialMappingParams{LegacyDeployment: args.LegacyDeployment,
			Kind: kind(it.Key.Table), LegacyID: args.LegacyID, TargetID: args.TargetID, PublicIDAtLink: args.PublicIDAtLink,
			FirstPlanSha256: args.FirstPlanSha256, Origin: args.Origin, DecisionSha256: args.DecisionSha256, VerifiedBy: args.VerifiedBy})
	}
}

func (e *engine) create(q *dbgen.Queries, it intent) (uuid.UUID, error) {
	bound := e.service.InTransaction(q)
	switch it.Key.Table {
	case sites:
		row, err := bound.CreateSite(e.ctx, catalog.SiteInput{Editorial: it.Editorial, Addresses: it.Addresses})
		if err != nil {
			return uuid.UUID{}, err
		}
		if it.Adult || it.Taxonomy.Primary != "" || len(it.Taxonomy.Tags) > 0 {
			rating := "unreviewed"
			if it.Adult {
				rating = "adult"
			}
			t := it.Taxonomy
			_, err = bound.ReviewSiteNavigation(e.ctx, "CN", row.PublicID, row.Revision, catalog.NavigationEdit{Rating: rating, Primary: t.Primary, Secondary: t.Secondary,
				Tags: t.Tags, Actor: "legacy_import", Reason: "D5-B1 editorial import " + e.planSHA})
		}
		return row.ID, err
	case games:
		row, err := bound.CreateGame(e.ctx, catalog.GameInput{Editorial: it.Editorial, Sources: it.Sources})
		if err != nil {
			return uuid.UUID{}, err
		}
		if it.Adult {
			_, err = catalog.SetGameBlock(e.ctx, q, row, true, catalog.PublicationDecision{Actor: "legacy_import", Reason: "D5-B1 legacy adult evidence " + e.planSHA})
		}
		return row.ID, err
	case collections:
		record, err := bound.CreateGameCollection(e.ctx, it.Editorial)
		if err != nil {
			return uuid.UUID{}, err
		}
		row, err := q.LockGameCollection(e.ctx, record.PublicID)
		return row.ID, err
	case categories:
		id := uuid.NewV7()
		return id, q.AddGameEditorialCategory(e.ctx, dbgen.AddGameEditorialCategoryParams{ID: id, Code: it.Code,
			Localizations: encode(it.Editorial.Localizations), SortOrder: it.Sort,
			ArchivedAt: pgtype.Timestamptz{Time: it.ArchivedAt, Valid: !it.ArchivedAt.IsZero()}})
	case tags:
		category, err := e.reference(q, it.Category)
		if err != nil {
			return uuid.UUID{}, err
		}
		if err = lockTarget(e.ctx, q, it.Category, category.ID); err != nil {
			return uuid.UUID{}, err
		}
		id := uuid.NewV7()
		return id, q.AddGameEditorialTag(e.ctx, dbgen.AddGameEditorialTagParams{ID: id, CategoryID: category.ID, Code: it.Code,
			Localizations: encode(it.Editorial.Localizations), ArchivedAt: pgtype.Timestamptz{Time: it.ArchivedAt, Valid: !it.ArchivedAt.IsZero()}})
	default:
		return uuid.UUID{}, catalog.ErrInvalid
	}
}

func (e *engine) reference(q *dbgen.Queries, reference key) (targetState, error) {
	if !reference.valid() {
		return targetState{}, deferred("dependency_mapping_missing")
	}
	_, state, err := e.mapped(q, intent{Key: reference}, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return targetState{}, deferred("dependency_mapping_missing")
	}
	if err != nil {
		return targetState{}, err
	}
	return *state, nil
}

func (e *engine) relations(q *dbgen.Queries, bound *catalog.Service, it intent, state targetState) error {
	// Lock referenced rows in a stable order. Mapped dependencies stay valid
	// throughout writes even when management or other writers are active.
	refs := map[string]targetState{}
	for _, r := range it.Relations {
		s, err := e.reference(q, r.Reference)
		if err != nil {
			return err
		}
		refs[r.Reference.String()] = s
	}
	dependencies := slices.Clone(it.Relations)
	slices.SortFunc(dependencies, func(a, b relation) int {
		if c := strings.Compare(a.Reference.Table, b.Reference.Table); c != 0 {
			return c
		}
		return strings.Compare(refs[a.Reference.String()].ID.String(), refs[b.Reference.String()].ID.String())
	})
	locked := map[[2]string]bool{}
	for _, r := range dependencies {
		id := refs[r.Reference.String()].ID
		lock := [2]string{r.Reference.Table, id.String()}
		if locked[lock] {
			continue
		}
		if err := lockTarget(e.ctx, q, r.Reference, id); err != nil {
			return err
		}
		locked[lock] = true
	}

	if it.Key.Table == games {
		for _, r := range it.Relations {
			if err := q.AddGameEditorialMember(e.ctx, dbgen.AddGameEditorialMemberParams{GameID: state.ID, TagID: refs[r.Reference.String()].ID, Role: r.Role}); err != nil {
				return err
			}
		}
		return q.TouchLegacyGameTags(e.ctx, state.ID)
	}
	var members []string
	var ruleIDs, excludedIDs []uuid.UUID
	for _, r := range it.Relations {
		dep := refs[r.Reference.String()]
		switch r.Key.Table {
		case manualMembers:
			members = append(members, dep.PublicID)
		case rules:
			ruleIDs = append(ruleIDs, dep.ID)
		case exclusions:
			excludedIDs = append(excludedIDs, dep.ID)
		}
	}
	if hasDuplicate(members) || hasDuplicate(ruleIDs) || hasDuplicate(excludedIDs) {
		return conflict("duplicate_target_relation")
	}
	if len(members) > 0 {
		if _, err := bound.SetGameCollectionMembers(e.ctx, state.PublicID, state.Revision, members); err != nil {
			return err
		}
	}
	for _, id := range ruleIDs {
		if err := q.AddGameCollectionRule(e.ctx, dbgen.AddGameCollectionRuleParams{CollectionID: state.ID, TagID: id}); err != nil {
			return err
		}
	}
	for _, id := range excludedIDs {
		if err := q.AddGameCollectionExclusion(e.ctx, dbgen.AddGameCollectionExclusionParams{CollectionID: state.ID, GameID: id}); err != nil {
			return err
		}
	}
	return nil
}

func hasDuplicate[T comparable](values []T) bool {
	seen := map[T]bool{}
	for _, val := range values {
		if seen[val] {
			return true
		}
		seen[val] = true
	}
	return false
}

func (e *engine) lifecycle(bound *catalog.Service, it intent, state targetState) error {
	d := catalog.PublicationDecision{Actor: "legacy_import", Reason: "D5-B1 legacy archive " + e.planSHA}
	if it.Publish {
		d.Reason = "D5-B1 approved publication " + it.PublishProof.Ref + " by " + it.PublishProof.Actor
	}
	switch it.Key.Table {
	case sites:
		action := "archive"
		if it.Publish {
			action = "publish"
		}
		_, err := bound.ChangeSitePublication(e.ctx, state.PublicID, state.Revision, action, it.Publish, d)
		return err
	case games:
		_, err := bound.PublishManagementGame(e.ctx, state.PublicID, state.Revision, d)
		return err
	case collections:
		status := catalog.Archived
		if it.Publish {
			status = catalog.Published
		}
		// Collection lifecycle uses its import receipt for auditing.
		_, err := bound.SetGameCollectionStatus(e.ctx, state.PublicID, state.Revision, status)
		return err
	default:
		return catalog.ErrInvalid
	}
}
