package collection

import (
	"context"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func saveSteamMetadata(ctx context.Context, q *dbgen.Queries, generation, epoch uuid.UUID, source *uuid.UUID, scopeKey string) error {
	games, err := q.SaveSteamMetadata(ctx, dbgen.SaveSteamMetadataParams{
		Generation: generation, Epoch: epoch, SourceID: source, ScopeKey: pgtype.Text{String: scopeKey, Valid: scopeKey != ""},
	})
	if err != nil || len(games) == 0 {
		return err
	}
	// Serialize against policy and source edits before rechecking which
	// already approved presentations may refresh.
	if err := q.LockSteamPresentationGames(ctx, games); err != nil {
		return err
	}
	return q.RefreshApprovedSteamPresentation(ctx, dbgen.RefreshApprovedSteamPresentationParams{GameIds: games, Generation: generation})
}
