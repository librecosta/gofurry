//go:build integration

package collection

import (
	"context"
	"testing"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

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
