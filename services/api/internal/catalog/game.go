package catalog

import (
	"context"
	"errors"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
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
		row, err := s.createGame(ctx, q, in.Slug, false)
		if err != nil {
			return row, err
		}
		return row, saveGameContent(ctx, q, row.ID, in, true)
	})
}

func (s *Service) createGame(ctx context.Context, q *dbgen.Queries, slug string, managed bool) (dbgen.Game, error) {
	id := uuid.NewV7()
	return create(s, func(publicID string) (dbgen.Game, error) {
		return q.CreateGame(ctx, dbgen.CreateGameParams{ID: id, PublicID: publicID, Slug: slug, IntakeManaged: managed})
	})
}

// CreateIntakeGame is part of the caller's identity transaction. Display
// data remains in the source projection and never enters editorial rows.
func (s *Service) CreateIntakeGame(ctx context.Context, q *dbgen.Queries, name string) (dbgen.Game, error) {
	ed, err := normalizeEditorial(Editorial{Localizations: []Localization{{Locale: "en", Name: name}}}, "")
	if err != nil {
		// Decorative slugs may repeat and must also work for titles with
		// punctuation or no safe ASCII spelling.
		ed.Slug = "game"
	}
	return s.createGame(ctx, q, ed.Slug, true)
}

func NormalizeGameSource(source Source) (Source, error) {
	rows, err := normalizeSources([]Source{source})
	if err != nil {
		return Source{}, err
	}
	return rows[0], nil
}

func (s *Service) EditGame(ctx context.Context, id uuid.UUID, revision int64, in GameInput) (dbgen.Game, error) {
	hasSources := in.Sources != nil
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
		if err = saveGameContent(ctx, q, id, in, hasSources || !row.IntakeManaged); err != nil {
			return row, err
		}
		if row.Status == Published {
			if err = requireGamePresentation(ctx, q, id); err != nil {
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
		d := PublicationDecision{Actor: "catalog", Reason: "Explicit editorial status change"}
		if desired == Archived {
			return changeGameVisibility(ctx, q, row, "archive", d)
		}
		if row.PublishedAt.Valid {
			return changeGameVisibility(ctx, q, row, "restore", d)
		}
		if err = q.SaveApprovedGamePresentation(ctx, id); err != nil {
			return row, err
		}
		if err = requireGamePublication(ctx, q, id); err != nil {
			return row, err
		}
		next, err := q.ChangeGameStatus(ctx, dbgen.ChangeGameStatusParams{ID: id, NewStatus: desired})
		if err == nil {
			err = d.Save(ctx, q, "game", id, next.Revision, "publish")
		}
		return next, err
	})
}

func requireGamePresentation(ctx context.Context, q *dbgen.Queries, id uuid.UUID) error {
	ready, err := q.HasGamePresentation(ctx, id)
	if err == nil && !ready {
		return ErrInvalid
	}
	return err
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

func saveGameContent(ctx context.Context, q *dbgen.Queries, id uuid.UUID, in GameInput, editSources bool) error {
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
	if !editSources {
		return nil
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
