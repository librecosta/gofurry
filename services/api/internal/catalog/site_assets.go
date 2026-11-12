package catalog

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

// PublicVisitURL revalidates persisted business URLs before discovery projection.
func PublicVisitURL(raw string) string {
	normalized, _ := normalizeURL(raw)
	return normalized
}

func (s *Service) SetSiteAsset(ctx context.Context, publicID string, revision int64, kind storage.SiteAssetKind, key string) (int64, error) {
	if !ValidPublicID(publicID) || revision < 1 ||
		(kind != storage.SiteLogo && kind != storage.SiteCover) || key != "" && !kind.ValidKey(key) {
		return 0, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (int64, error) {
		row, err := q.LockNavigationSite(ctx, publicID)
		if err != nil {
			return 0, err
		}
		if err = editable(revision, row.Revision, row.Status); err != nil {
			return 0, err
		}
		in := dbgen.SetSiteAssetsParams{ID: row.ID, LogoAssetKey: row.LogoAssetKey, FeaturedCoverAssetKey: row.FeaturedCoverAssetKey}
		ref := pgtype.Text{String: key, Valid: key != ""}
		if kind == storage.SiteLogo {
			in.LogoAssetKey = ref
		} else {
			in.FeaturedCoverAssetKey = ref
		}
		return q.SetSiteAssets(ctx, in)
	})
}
