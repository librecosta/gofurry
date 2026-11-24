package catalog

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"regexp"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const gameAssetPageSize = 32
const gameAssetBatchSize = 64

var (
	assetCode     = regexp.MustCompile("^[a-z][a-z0-9_]{0,63}$")
	assetLocale   = regexp.MustCompile("^[a-z]{2,8}(-[a-zA-Z0-9]{1,8}){0,3}$")
	assetScopeKey = regexp.MustCompile("^[0-9a-f]{64}$")
	assetEgress   = regexp.MustCompile("^[a-z0-9][a-z0-9_-]{0,34}$")
)

func validAssetVersionID(id uuid.UUID) bool { return id[6]>>4 == 7 && id[8]>>6 == 2 }

type GameAssetFilter struct {
	After                                  uuid.UUID
	AssetID                                *uuid.UUID
	Platform, Kind, Category, Locale, Role string
}

type GameAssetVersionView struct {
	VersionID     uuid.UUID          `json:"version_id"`
	Origin        jsontext.Value     `json:"origin"`
	Name          string             `json:"name,omitempty"`
	OriginalOrder pgtype.Int4        `json:"original_order"`
	Highlight     pgtype.Bool        `json:"highlight"`
	Truncated     pgtype.Bool        `json:"truncated"`
	Current       bool               `json:"current"`
	SelectedAt    pgtype.Timestamptz `json:"selected_at"`
}

type GameAssetSummary struct {
	dbgen.GameAsset
	GameAssetVersionView
	PreviewURL string `json:"preview_url,omitempty"`
}

type GameAssetDetail struct {
	dbgen.GameAsset
	GameAssetVersionView
	Variants []dbgen.GameAssetVariant `json:"variants"`
}

type GameAssetPage struct {
	Items []GameAssetSummary `json:"items"`
	Next  *uuid.UUID         `json:"next,omitempty"`
}

func gameAssetVersionView(v dbgen.GameAssetVersion, selected pgtype.Timestamptz, current bool) GameAssetVersionView {
	return GameAssetVersionView{VersionID: v.ID, Origin: v.Origin, Name: v.Name, OriginalOrder: v.OriginalOrder, Highlight: v.Highlight,
		Truncated: v.Truncated, Current: current, SelectedAt: selected}
}

func (s *Service) ManagementGameAssets(ctx context.Context, id string, f GameAssetFilter, library bool) (GameAssetPage, error) {
	if !ValidPublicID(id) || f.After != uuid.Nil() && !validAssetVersionID(f.After) || f.AssetID != nil && !validAssetVersionID(*f.AssetID) ||
		f.Kind != "" && f.Kind != "image" && f.Kind != "video" ||
		f.Platform != "" && (len(f.Platform) > 32 || !assetCode.MatchString(f.Platform)) ||
		f.Category != "" && (len(f.Category) > 48 || !assetCode.MatchString(f.Category)) ||
		f.Locale != "" && !assetLocale.MatchString(f.Locale) || f.Role != "" && !assetCode.MatchString(f.Role) {
		return GameAssetPage{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (out GameAssetPage, err error) {
		g, err := q.ManagementGame(ctx, id)
		if err != nil {
			return out, err
		}
		rows, err := q.ListGameAssetVersions(ctx, dbgen.ListGameAssetVersionsParams{GameID: g.ID, AfterID: f.After, AssetID: f.AssetID,
			LibraryOnly: library, Platform: f.Platform, Kind: f.Kind, Category: f.Category, Locale: f.Locale, SourceRole: f.Role, TakeRows: gameAssetPageSize + 1})
		if err != nil {
			return out, err
		}
		if len(rows) > gameAssetPageSize {
			rows = rows[:gameAssetPageSize]
			out.Next = &rows[len(rows)-1].GameAssetVersion.ID
		}
		out.Items = make([]GameAssetSummary, 0, len(rows))
		for _, row := range rows {
			out.Items = append(out.Items, GameAssetSummary{row.GameAsset, gameAssetVersionView(row.GameAssetVersion, row.SelectedAt, row.Current), row.PreviewUrl})
		}
		return out, nil
	})
}

func (s *Service) ManagementGameAsset(ctx context.Context, id string, asset, version uuid.UUID) (GameAssetDetail, error) {
	if !ValidPublicID(id) || !validAssetVersionID(asset) || !validAssetVersionID(version) {
		return GameAssetDetail{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (out GameAssetDetail, err error) {
		g, err := q.ManagementGame(ctx, id)
		if err != nil {
			return out, err
		}
		row, err := q.ReadGameAssetVersion(ctx, dbgen.ReadGameAssetVersionParams{GameID: g.ID, AssetID: asset, VersionID: version})
		if err != nil {
			return out, err
		}
		out.GameAsset, out.GameAssetVersionView = row.GameAsset, gameAssetVersionView(row.GameAssetVersion, row.SelectedAt, row.Current)
		out.Variants, err = q.ListGameAssetVariants(ctx, version)
		return out, err
	})
}

type GameAssetLibraryEdit struct {
	Revision int64       `json:"revision"`
	Add      []uuid.UUID `json:"add"`
	Remove   []uuid.UUID `json:"remove"`
}

func (s *Service) EditGameAssetLibrary(ctx context.Context, id string, in GameAssetLibraryEdit) (ManagementResult, error) {
	if s.management == nil {
		return ManagementResult{}, ErrGamePublicationIdentity
	}
	n := len(in.Add) + len(in.Remove)
	if n == 0 || n > gameAssetBatchSize {
		return ManagementResult{}, ErrInvalid
	}
	seen := make(map[uuid.UUID]bool, n)
	for _, batch := range [][]uuid.UUID{in.Add, in.Remove} {
		for _, version := range batch {
			if !validAssetVersionID(version) || seen[version] {
				return ManagementResult{}, ErrInvalid
			}
			seen[version] = true
		}
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		g, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		if err = editable(in.Revision, g.Revision, g.Status); err != nil {
			return ManagementResult{}, err
		}
		var changed int64
		for _, version := range in.Add {
			if _, err := q.GameAssetPresentationPick(ctx, dbgen.GameAssetPresentationPickParams{GameID: g.ID, VersionID: version}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					err = ErrInvalid
				}
				return ManagementResult{}, err
			}
			n, err := q.AddGameAssetLibrary(ctx, dbgen.AddGameAssetLibraryParams{GameID: g.ID, VersionID: version, SelectedBy: &s.management.ActorID, SelectionOrigin: "operator"})
			if err != nil {
				return ManagementResult{}, err
			}
			changed += n
		}
		for _, version := range in.Remove {
			n, err := q.RemoveGameAssetLibrary(ctx, dbgen.RemoveGameAssetLibraryParams{GameID: g.ID, VersionID: version})
			if err != nil {
				return ManagementResult{}, err
			}
			changed += n
		}
		return finishGameAssetEdit(ctx, q, g, changed)
	})
}

func finishGameAssetEdit(ctx context.Context, q *dbgen.Queries, g dbgen.Game, changed int64) (ManagementResult, error) {
	var err error
	if changed != 0 {
		g, err = q.UpdateGameDraft(ctx, dbgen.UpdateGameDraftParams{GameID: g.ID, EditSlug: gameEditSlug(g)})
	}
	n := int(changed)
	return ManagementResult{Target: g.PublicID, Revision: g.Revision, Affected: &n}, err
}
