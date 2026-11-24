package catalog

import (
	"context"
	"errors"
	"slices"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type GameAssetPosition struct {
	Slot     string `json:"slot"`
	Position int64  `json:"position"`
}

type GameAssetPlacement struct {
	GameAssetPosition
	VersionID  uuid.UUID `json:"version_id"`
	VariantKey *string   `json:"variant_key,omitempty"`
}

type GameAssetPresentationEdit struct {
	Revision int64                `json:"revision"`
	Locale   string               `json:"locale"`
	Set      []GameAssetPlacement `json:"set"`
	Remove   []GameAssetPosition  `json:"remove"`
}

type GameAssetPresentationPage struct {
	Revision *int64                               `json:"revision,omitempty"`
	Items    []dbgen.ListGameAssetPresentationRow `json:"items"`
	Next     *int64                               `json:"next,omitempty"`
}

func validAssetPosition(p GameAssetPosition) bool {
	return p.Slot == "cover" && p.Position == 0 || p.Slot == "gallery" && p.Position >= 0 && p.Position <= (1<<53)-1
}

func (s *Service) ManagementGameAssetPresentation(ctx context.Context, id, locale, surface, slot string, after int64) (GameAssetPresentationPage, error) {
	if !ValidPublicID(id) || locale != "zh" && locale != "en" ||
		!slices.Contains([]string{"draft", "published", "legacy_draft", "legacy_published"}, surface) ||
		(slot != "artwork" || surface == "draft" || surface == "published") && !validAssetPosition(GameAssetPosition{Slot: slot}) || after < -1 {
		return GameAssetPresentationPage{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (out GameAssetPresentationPage, err error) {
		g, err := q.ManagementGame(ctx, id)
		if err != nil {
			return out, err
		}
		p, err := q.ReadGameAssetPresentation(ctx, dbgen.ReadGameAssetPresentationParams{GameID: g.ID, Locale: locale, Surface: surface})
		if errors.Is(err, pgx.ErrNoRows) {
			out.Items = []dbgen.ListGameAssetPresentationRow{}
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out.Revision = &p.Revision
		out.Items, err = q.ListGameAssetPresentation(ctx, dbgen.ListGameAssetPresentationParams{
			GameID: g.ID, Locale: locale, Surface: surface, Slot: slot, AfterPosition: after, TakeRows: gameAssetPageSize + 1})
		if len(out.Items) > gameAssetPageSize {
			out.Items = out.Items[:gameAssetPageSize]
			out.Next = &out.Items[len(out.Items)-1].Position
		}
		return out, err
	})
}

func (s *Service) EditGameAssetPresentation(ctx context.Context, id string, in GameAssetPresentationEdit) (ManagementResult, error) {
	if s.management == nil {
		return ManagementResult{}, ErrGamePublicationIdentity
	}
	n := len(in.Set) + len(in.Remove)
	if n == 0 || n > gameAssetBatchSize || in.Locale != "en" && in.Locale != "zh" {
		return ManagementResult{}, ErrInvalid
	}
	seen := make(map[GameAssetPosition]bool, n)
	for _, p := range in.Remove {
		if !validAssetPosition(p) || seen[p] {
			return ManagementResult{}, ErrInvalid
		}
		seen[p] = true
	}
	for _, item := range in.Set {
		p := item.GameAssetPosition
		if !validAssetPosition(p) || seen[p] || !validAssetVersionID(item.VersionID) ||
			item.VariantKey != nil && (len(*item.VariantKey) > 48 || *item.VariantKey != "2x" && !assetCode.MatchString(*item.VariantKey)) {
			return ManagementResult{}, ErrInvalid
		}
		seen[p] = true
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		g, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		if err = editable(in.Revision, g.Revision, g.Status); err != nil {
			return ManagementResult{}, err
		}
		locales, err := q.ManagementGameText(ctx, g.ID)
		if err != nil {
			return ManagementResult{}, err
		}
		if !slices.ContainsFunc(locales, func(l dbgen.ManagementGameTextRow) bool { return l.Locale == in.Locale }) {
			return ManagementResult{}, ErrInvalid
		}
		if len(in.Set) > 0 {
			if err := q.EnsureGameAssetPresentation(ctx, dbgen.EnsureGameAssetPresentationParams{GameID: g.ID, Locale: in.Locale, Surface: "draft", Revision: g.Revision}); err != nil {
				return ManagementResult{}, err
			}
		}
		var changed int64
		for _, item := range in.Set {
			n, err := setDraftGameAssetItem(ctx, q, g.ID, in.Locale, item)
			if err != nil {
				return ManagementResult{}, err
			}
			changed += n
		}
		for _, p := range in.Remove {
			n, err := q.RemoveGameAssetPresentationItem(ctx, dbgen.RemoveGameAssetPresentationItemParams{
				GameID: g.ID, Locale: in.Locale, Surface: "draft", Slot: p.Slot, Position: p.Position})
			if err != nil {
				return ManagementResult{}, err
			}
			changed += n
		}
		if changed > 0 {
			if err := q.ReviseGameAssetPresentation(ctx, dbgen.ReviseGameAssetPresentationParams{
				GameID: g.ID, Locale: in.Locale, Surface: "draft", Revision: g.Revision + 1}); err != nil {
				return ManagementResult{}, err
			}
		}
		return finishGameAssetEdit(ctx, q, g, changed)
	})
}

func setDraftGameAssetItem(ctx context.Context, q *dbgen.Queries, id uuid.UUID, locale string, item GameAssetPlacement) (int64, error) {
	var key pgtype.Text
	if item.VariantKey != nil {
		key = pgtype.Text{String: *item.VariantKey, Valid: true}
	}
	v, err := q.GameAssetPresentationPick(ctx, dbgen.GameAssetPresentationPickParams{GameID: id, VersionID: item.VersionID, VariantKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvalid
	}
	if err != nil {
		return 0, err
	}
	if !v.Selected || !v.VariantValid || item.Slot == "cover" && v.Kind != "image" {
		return 0, ErrInvalid
	}
	return q.SetGameAssetPresentationItem(ctx, dbgen.SetGameAssetPresentationItemParams{
		GameID: id, Locale: locale, Surface: "draft", Slot: item.Slot, Position: item.Position, VersionID: item.VersionID, VariantKey: key})
}

func gameAssetDraftDiffers(ctx context.Context, q *dbgen.Queries, g dbgen.Game, draft, published []byte) (bool, error) {
	if gameDraftDiffers(g, draft, published) {
		return true, nil
	}
	return q.GameAssetPresentationChanged(ctx, g.ID)
}

func snapshotGameAssetPresentation(ctx context.Context, q *dbgen.Queries, id uuid.UUID, revision int64) error {
	if err := q.ClearPublishedGameAssetPresentation(ctx, id); err != nil {
		return err
	}
	if err := q.SnapshotGameAssetPresentation(ctx, dbgen.SnapshotGameAssetPresentationParams{GameID: id, Revision: revision}); err != nil {
		return err
	}
	return q.SnapshotGameAssetPresentationItems(ctx, id)
}
