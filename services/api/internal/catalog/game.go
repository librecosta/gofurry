package catalog

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"uuid"
)

func (s *Service) CreateGame(ctx context.Context, in GameInput) (dbgen.Game, error) {
	var err error
	in.Editorial, err = normalizeEditorial(in.Editorial, "")
	if err != nil {
		return dbgen.Game{}, err
	}
	in.Sources, err = normalizeSources(in.Sources)
	if err != nil {
		return dbgen.Game{}, err
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
		id := uuid.NewV7()
		row, err := create(s, func(publicID string) (dbgen.Game, error) {
			return q.CreateGame(ctx, dbgen.CreateGameParams{ID: id, PublicID: publicID, Slug: in.Slug})
		})
		if err != nil {
			return row, err
		}
		return row, saveGameContent(ctx, q, row.ID, in)
	})
}

func (s *Service) EditGame(ctx context.Context, id uuid.UUID, revision int64, in GameInput) (dbgen.Game, error) {
	var err error
	in.Sources, err = normalizeSources(in.Sources)
	if err != nil {
		return dbgen.Game{}, err
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
		row, err := q.LockGame(ctx, id)
		if err != nil {
			return row, err
		}
		if err = editable(revision, row.Revision, row.Status); err != nil {
			return row, err
		}
		in.Editorial, err = normalizeEditorial(in.Editorial, row.Slug)
		if err != nil {
			return row, err
		}
		if err = saveGameContent(ctx, q, id, in); err != nil {
			return row, err
		}
		if row.Status == Published {
			if err = requireGamePublication(ctx, q, id); err != nil {
				return row, err
			}
		}
		if err = q.CloseRemovedGameBindings(ctx, &id); err != nil {
			return row, err
		}
		return q.UpdateGame(ctx, dbgen.UpdateGameParams{ID: id, Slug: in.Slug})
	})
}

func (s *Service) SetGameStatus(ctx context.Context, id uuid.UUID, revision int64, desired string) (dbgen.Game, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
		row, err := q.LockGame(ctx, id)
		if err != nil {
			return row, err
		}
		if err = editable(revision, row.Revision, row.Status); err != nil {
			return row, err
		}
		if err = canChange(row.Status, desired); err != nil {
			return row, err
		}
		if desired == Published {
			if err = requireGamePublication(ctx, q, id); err != nil {
				return row, err
			}
		}
		return q.ChangeGameStatus(ctx, dbgen.ChangeGameStatusParams{ID: id, NewStatus: desired})
	})
}

func requireGamePublication(ctx context.Context, q *dbgen.Queries, id uuid.UUID) error {
	ready, err := q.CanPublishGame(ctx, id)
	if err != nil {
		return err
	}
	if !ready {
		return ErrInvalid
	}
	return nil
}

func saveGameContent(ctx context.Context, q *dbgen.Queries, id uuid.UUID, in GameInput) error {
	if err := q.ClearGameLocalizations(ctx, id); err != nil {
		return err
	}
	for _, localization := range in.Localizations {
		if err := q.AddGameLocalization(ctx, dbgen.AddGameLocalizationParams{
			GameID: id, Locale: localization.Locale, Name: localization.Name, Summary: localization.Summary,
		}); err != nil {
			return err
		}
	}
	if err := q.DisableGameSources(ctx, id); err != nil {
		return err
	}
	for _, source := range in.Sources {
		_, err := q.SaveGameSource(ctx, dbgen.SaveGameSourceParams{
			ID: uuid.NewV7(), GameID: id, PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey, SourceUrl: source.SourceURL,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
	}
	return nil
}
