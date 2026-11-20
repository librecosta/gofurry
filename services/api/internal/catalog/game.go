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
		row, err := s.createGame(ctx, q, in.Slug)
		if err != nil {
			return row, err
		}
		for _, l := range in.Localizations {
			if err := q.AddGameLocalization(ctx, dbgen.AddGameLocalizationParams{GameID: row.ID, Locale: l.Locale, Name: l.Name, Summary: l.Summary}); err != nil {
				return row, err
			}
		}
		return row, saveGameSources(ctx, q, row.ID, in.Sources)
	})
}

func (s *Service) createGame(ctx context.Context, q *dbgen.Queries, slug string) (dbgen.Game, error) {
	id := uuid.NewV7()
	return create(s, func(publicID string) (dbgen.Game, error) {
		return q.CreateGame(ctx, dbgen.CreateGameParams{ID: id, PublicID: publicID, Slug: slug})
	})
}

// CreateIntakeGame is part of the caller's identity transaction.
func (s *Service) CreateIntakeGame(ctx context.Context, q *dbgen.Queries, name string) (dbgen.Game, error) {
	ed, err := normalizeEditorial(Editorial{Localizations: []Localization{{Locale: "en", Name: name}}}, "")
	if err != nil {
		// Decorative slugs may repeat and must also work for titles with
		// punctuation or no safe ASCII spelling.
		ed.Slug = "game"
	}
	return s.createGame(ctx, q, ed.Slug)
}

// InitializeIntakeGameText runs only when Intake has created a new identity.
func InitializeIntakeGameText(ctx context.Context, q *dbgen.Queries, game dbgen.Game, source, observation uuid.UUID, text Localization) error {
	ed, err := normalizeEditorial(Editorial{Localizations: []Localization{text}}, game.Slug)
	if err != nil || observation == uuid.Nil() {
		return nil // An incomplete first observation leaves an empty draft.
	}
	text = ed.Localizations[0]
	return q.SaveGameDraftText(ctx, dbgen.SaveGameDraftTextParams{GameID: game.ID, Locale: text.Locale,
		Name: text.Name, Summary: text.Summary, TextSourceID: &source, TextObservationID: &observation})
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
		if hasSources && row.Status != Draft {
			return row, ErrConflict
		}
		row, err = saveGameDraft(ctx, q, row, GameManagementEdit{Revision: revision, Slug: in.Slug, Localizations: in.Localizations})
		if err != nil || !hasSources {
			return row, err
		}
		return editGameSources(ctx, q, row, in.Sources, revision)
	})
}

func editGameSources(ctx context.Context, q *dbgen.Queries, row dbgen.Game, sources []Source, previousRevision int64) (dbgen.Game, error) {
	if err := saveGameSources(ctx, q, row.ID, sources); err != nil {
		return row, err
	}
	if err := q.CloseRemovedGameBindings(ctx, &row.ID); err != nil {
		return row, err
	}
	if row.Revision != previousRevision {
		return row, nil
	}
	return q.UpdateGameDraft(ctx, dbgen.UpdateGameDraftParams{GameID: row.ID, EditSlug: row.Slug})
}

func saveGameSources(ctx context.Context, q *dbgen.Queries, id uuid.UUID, sources []Source) error {
	if err := q.DisableGameSources(ctx, id); err != nil {
		return err
	}
	for _, source := range sources {
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
