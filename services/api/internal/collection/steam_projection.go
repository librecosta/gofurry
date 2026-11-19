package collection

import (
	"context"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func saveSteamMetadata(ctx context.Context, q *dbgen.Queries, generation, epoch uuid.UUID, source *uuid.UUID, scopeKey string) error {
	return q.SaveSteamMetadata(ctx, dbgen.SaveSteamMetadataParams{
		Generation: generation, Epoch: epoch, SourceID: source, ScopeKey: pgtype.Text{String: scopeKey, Valid: scopeKey != ""},
	})
}
