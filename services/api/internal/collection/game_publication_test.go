//go:build integration

package collection

import (
	"context"
	"testing"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func fixtureMediaChoices(review catalog.GameManagement) []catalog.GameMediaSelection {
	choices := []catalog.GameMediaSelection{}
	for _, locale := range []string{"en", "zh"} {
		for _, source := range review.SourceCandidates {
			if source.Section != "media" || source.Locale != locale || source.Media.Header == "" {
				continue
			}
			choices = append(choices, catalog.GameMediaSelection{Locale: locale, Media: source.Media,
				CoverURL: source.Media.Header, MediaSourceID: &source.SourceID, MediaObservationID: &source.ObservationID})
			break
		}
	}
	return choices
}

func publishFixtureGame(t *testing.T, editor *catalog.Service, id string, revision int64) (catalog.ManagementResult, error) {
	t.Helper()
	ctx := t.Context()
	preview, err := editor.ManagementGame(ctx, id)
	if err != nil {
		return catalog.ManagementResult{}, err
	}
	return editor.Manage(ctx, dbgen.RecordManagementOperationParams{
		RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "Game fixture operator", ActorRole: "operator", Action: "game.publish",
	}, func(ctx context.Context, tx *catalog.Service, _ *dbgen.Queries) (catalog.ManagementResult, error) {
		return tx.PublishManagementGame(ctx, id, catalog.GamePublicationReview{
			Revision: revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: preview.PreviewToken,
		})
	})
}

func changeFixtureGame(t *testing.T, editor *catalog.Service, id string, revision int64, action string) (catalog.ManagementResult, error) {
	t.Helper()
	return editor.Manage(t.Context(), dbgen.RecordManagementOperationParams{
		RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "Game fixture operator", ActorRole: "operator", Action: "game." + action,
	}, func(ctx context.Context, tx *catalog.Service, _ *dbgen.Queries) (catalog.ManagementResult, error) {
		return tx.ChangeManagementGameVisibility(ctx, id, revision, action, "Fixture lifecycle")
	})
}
