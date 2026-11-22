package collection

import (
	"context"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type SteamGameExists string

func (e SteamGameExists) Error() string { return "steam_game_exists:" + string(e) }

// CreateSteamGame joins source ownership and collection intent to the same
// local transaction as the Draft and the Management receipt.
func (s *Service) CreateSteamGame(ctx context.Context, cat *catalog.Service, q *dbgen.Queries, in catalog.GameManagementEdit, region string) (catalog.ManagementResult, error) {
	if !steamID(in.SteamAppID) || in.Revision != 0 || len(in.Localizations) == 0 || len(in.TextAdoptions) != 0 || len(in.DraftMedia) != 0 {
		return catalog.ManagementResult{}, ErrIntakeInput
	}
	source, err := intakeSource("steam", in.SteamAppID)
	if err != nil {
		return catalog.ManagementResult{}, err
	}
	// Keep prefill provenance available until source ownership commits.
	if s != nil && s.namespace != "" {
		if err := q.LockSteamPrefillAdmission(ctx, s.namespace); err != nil {
			return catalog.ManagementResult{}, err
		}
	}
	existing, err := q.ExistingSteamGame(ctx, in.SteamAppID)
	if err == nil {
		return catalog.ManagementResult{}, SteamGameExists(existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return catalog.ManagementResult{}, err
	}
	text := in.Localizations
	game, err := cat.CreateGame(ctx, catalog.GameInput{
		Editorial: catalog.Editorial{Slug: catalog.NewGameSlug(text), Localizations: text}, Sources: []catalog.Source{source},
	})
	if errors.Is(err, catalog.ErrConflict) {
		if existing, lookup := q.ExistingSteamGame(ctx, in.SteamAppID); lookup == nil {
			return catalog.ManagementResult{}, SteamGameExists(existing)
		}
	}
	if err != nil {
		return catalog.ManagementResult{}, err
	}
	row, err := q.ExistingIntakeSource(ctx, dbgen.ExistingIntakeSourceParams{PlatformCode: "steam", ExternalKey: in.SteamAppID})
	if err != nil {
		return catalog.ManagementResult{}, err
	}
	if s != nil && s.namespace != "" {
		err = intakeSchedule(ctx, q, dbgen.GameIntakeCandidate{Namespace: s.namespace, EgressRegion: s.editorialEgress(region), ExternalKey: in.SteamAppID}, row)
		if err != nil {
			return catalog.ManagementResult{}, err
		}
		cached, err := q.ListSteamEditorialPrefills(ctx, dbgen.ListSteamEditorialPrefillsParams{Namespace: s.namespace, AppID: in.SteamAppID})
		if err != nil {
			return catalog.ManagementResult{}, err
		}
		for _, offer := range cached {
			if offer.TextObservationID == nil {
				continue
			}
			if err := q.AttributeNewGameDraftText(ctx, dbgen.AttributeNewGameDraftTextParams{
				GameID: game.ID, Locale: editorialLocale(offer.Locale), Name: offer.Name, Summary: offer.Summary,
				TextSourceID: &row.ID, TextObservationID: offer.TextObservationID,
			}); err != nil {
				return catalog.ManagementResult{}, err
			}
		}
	}
	return catalog.ManagementResult{Target: game.PublicID, Revision: game.Revision}, nil
}

func (s *Service) editorialEgress(region string) string {
	if s.egress != "" {
		return s.egress
	}
	return strings.ToLower(region)
}

func (s *Service) RegisterSteam(ctx context.Context, q *dbgen.Queries, id, app, region string) error {
	if s == nil || s.namespace == "" || !steamID(app) {
		return ErrIntent
	}
	row, err := q.CollectionSteamSource(ctx, dbgen.CollectionSteamSourceParams{PublicID: id, ExternalKey: app})
	if err != nil {
		return err
	}
	return intakeSchedule(ctx, q, dbgen.GameIntakeCandidate{Namespace: s.namespace, EgressRegion: s.editorialEgress(region), ExternalKey: app},
		dbgen.GameSource{ID: row.ID, GameID: row.GameID})
}

var planAnchor = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func (s *Service) RegisterSite(ctx context.Context, q *dbgen.Queries, id, region string) error {
	if s == nil || s.namespace == "" {
		return nil
	}
	address, err := q.CollectionPrimarySiteAddress(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	bindings, err := s.bindSiteAddress(ctx, q, address.SiteID, address.ID, address.Url, address.Kind)
	if errors.Is(err, ErrIntent) {
		return nil // Valid catalog addresses without a supported collection target remain unbound.
	}
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		plans := []string{"web.dns"}
		if binding.ResourceType == "origin" {
			plans = []string{"web.http"}
			if strings.HasPrefix(address.Url, "https://") {
				plans = append(plans, "web.tls")
			}
		}
		for _, capability := range plans {
			p := Plan{Capability: capability, EgressRegion: s.editorialEgress(region), Interval: 24 * time.Hour, Anchor: planAnchor}
			if _, err := s.attachEditorialPlan(ctx, q, binding.ID, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) attachEditorialPlan(ctx context.Context, q *dbgen.Queries, binding uuid.UUID, p Plan) (uuid.UUID, error) {
	b, err := q.OpenCollectionBinding(ctx, dbgen.OpenCollectionBindingParams{ID: binding, Namespace: s.namespace})
	if err != nil {
		return uuid.Nil(), err
	}
	params, scope, policy, err := p.params(b.ResourceType, b.ExternalKey)
	if err != nil {
		return uuid.Nil(), err
	}
	id, err := q.FindMatchingCollectionPlan(ctx, dbgen.FindMatchingCollectionPlanParams{
		SourceID: b.SourceID, Capability: p.Capability, Params: params, Scope: scope, Policy: policy, OutputSchema: outputSchema(p.OutputSchema),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		row, saveErr := q.SaveCollectionPlan(ctx, dbgen.SaveCollectionPlanParams{
			ID: uuid.NewV7(), SourceID: b.SourceID, Capability: p.Capability, Params: params, Scope: scope,
			Policy: policy, AnchorAt: timestamp(p.Anchor), IntervalSeconds: int32(p.Interval / time.Second), OutputSchema: outputSchema(p.OutputSchema),
		})
		id, err = row.ID, saveErr
	}
	if err != nil {
		return uuid.Nil(), err
	}
	attached, err := q.AttachCollectionPlan(ctx, dbgen.AttachCollectionPlanParams{PlanID: id, BindingID: b.ID, SourceID: b.SourceID})
	if err == nil && !attached {
		err = ErrIntent
	}
	return id, err
}
