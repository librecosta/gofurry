package catalog

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type AssetMigrationIssue struct {
	Record string `json:"record"`
	Code   string `json:"code"`
}

type AssetMigrationReport struct {
	SourceObservations int                   `json:"source_observations"`
	DraftLocales       int                   `json:"draft_locales"`
	PublishedLocales   int                   `json:"published_locales"`
	VariantsAdded      int                   `json:"variants_added"`
	LibraryAdded       int64                 `json:"library_added"`
	MappingsAdded      int64                 `json:"mappings_added"`
	IssueCount         int                   `json:"issue_count"`
	Issues             []AssetMigrationIssue `json:"issues"`
}

func (r *AssetMigrationReport) issue(id, code string) {
	r.IssueCount++
	if len(r.Issues) < 500 {
		r.Issues = append(r.Issues, AssetMigrationIssue{Record: id, Code: code})
	}
}

// BackfillGameAssets imports persisted regional records. Each source observation
// and each game's legacy snapshot mappings commit independently.
func BackfillGameAssets(ctx context.Context, pool *pgxpool.Pool) (AssetMigrationReport, error) {
	report := AssetMigrationReport{Issues: []AssetMigrationIssue{}}
	q := dbgen.New(pool)
	for after := uuid.Nil(); ; {
		rows, err := q.GameAssetBackfillSources(ctx, after)
		if err != nil {
			return report, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			report.SourceObservations++
			after = row.ID
			var added int
			err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
				q := q.WithTx(tx)
				b, items, truncated, warning := assetSteamSource(ctx, q, row)
				if warning != nil {
					if !errors.Is(warning, errAssetNormalization) {
						return warning
					}
					report.issue(row.ID.String(), "unrepresentable_source_asset")
				}
				o, err := mediaOrigin(row)
				if err != nil {
					return errAssetAttribution
				}
				added, err = putAssetVersions(ctx, q, o, b.ExternalKey, items, truncated)
				return err
			})
			if errors.Is(err, errAssetAttribution) {
				report.issue(row.ID.String(), "unverified_or_conflicting_source_attribution")
			} else if err != nil {
				return report, err
			}
			report.VariantsAdded += added
		}
	}
	for after := uuid.Nil(); ; {
		games, err := q.GameAssetBackfillGames(ctx, after)
		if err != nil || len(games) == 0 {
			return report, err
		}
		for _, game := range games {
			after = game.ID
			err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
				q := q.WithTx(tx)
				if _, err := q.LockGamePublicID(ctx, game.PublicID); err != nil {
					return err
				}
				rows, err := q.GameAssetBackfillSnapshots(ctx, game.ID)
				if err != nil {
					return err
				}
				for _, row := range rows {
					if row.Surface == "legacy_draft" {
						report.DraftLocales++
					} else {
						report.PublishedLocales++
					}
					if err := backfillGameAssetSnapshot(ctx, q, game.ID, row, &report); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return report, err
			}
		}
	}
}

type legacyAssetReference struct {
	origin   GameMediaOrigin
	asset    sourceAsset
	image    *GameSelectedImage
	slot     string
	position int64
}

func backfillGameAssetSnapshot(ctx context.Context, q *dbgen.Queries, game uuid.UUID, row dbgen.GameAssetBackfillSnapshotsRow, report *AssetMigrationReport) error {
	var media GameMediaSelectionV2
	key := game.String() + "/" + row.Surface + "/" + row.Locale
	if json.Unmarshal(row.Media, &media, json.RejectUnknownMembers(true)) != nil || media.Version != 2 || media.SelectionRevision < 0 {
		report.issue(key, "legacy_snapshot_invalid")
		return nil
	}
	revision := row.Revision
	if row.Surface == "legacy_draft" {
		revision = media.SelectionRevision
	}
	p := dbgen.ReadGameAssetPresentationParams{GameID: game, Locale: row.Locale, Surface: row.Surface}
	prior, err := q.ReadGameAssetPresentation(ctx, p)
	if err == nil && prior.Revision != revision {
		report.issue(key, "legacy_revision_changed_after_import")
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := q.EnsureGameAssetPresentation(ctx, dbgen.EnsureGameAssetPresentationParams{GameID: game, Locale: row.Locale, Surface: row.Surface, Revision: revision}); err != nil {
		return err
	}
	for _, ref := range legacyGameAssetReferences(media) {
		err := backfillAssetReference(ctx, q, game, row, ref, report)
		if errors.Is(err, errAssetAttribution) || errors.Is(err, errAssetNormalization) {
			report.issue(key+"/"+ref.origin.ObservationID.String()+"/"+ref.slot, "legacy_asset_or_attribution_invalid")
		} else if err != nil {
			return err
		}
	}
	return nil
}

// V2 had separate ordered lists. The compatibility map stores artwork
// separately and appends the original movie list after the original screenshots.
func legacyGameAssetReferences(media GameMediaSelectionV2) []legacyAssetReference {
	var refs []legacyAssetReference
	if media.Cover != nil {
		refs = append(refs, legacyAssetReference{origin: media.Cover.Origin, image: media.Cover, slot: "cover"})
	}
	for i, v := range media.Images {
		refs = append(refs, legacyAssetReference{origin: v.Origin, image: &v, slot: "artwork", position: int64(i)})
	}
	for i, v := range media.Screenshots {
		refs = append(refs, legacyAssetReference{origin: v.Origin, asset: steamShot(v.SteamScreenshot), slot: "gallery", position: int64(i)})
	}
	for i, v := range media.Movies {
		refs = append(refs, legacyAssetReference{origin: v.Origin, asset: steamFilm(v.SteamMovie), slot: "gallery", position: int64(len(media.Screenshots) + i)})
	}
	return refs
}

func backfillAssetReference(ctx context.Context, q *dbgen.Queries, game uuid.UUID, row dbgen.GameAssetBackfillSnapshotsRow, ref legacyAssetReference, report *AssetMigrationReport) error {
	o := ref.origin
	if !validAssetVersionID(o.ObservationID) || !assetScopeKey.MatchString(o.ScopeKey) || !assetScopeKey.MatchString(o.ContentHash) ||
		(o.Locale != "en" && o.Locale != "zh-cn") || (o.StoreCountry != "cn" && o.StoreCountry != "us" && o.StoreCountry != "hk") ||
		!assetEgress.MatchString(o.EgressRegion) || !assetEgress.MatchString(o.RequestedEgressRegion) ||
		o.Ruleset != "gofurry-steam-media-2" || o.ObservedAt.IsZero() {
		return errAssetAttribution
	}
	b, err := assetSteamBinding(ctx, q, o)
	if err != nil {
		return err
	}
	if b.GameID != game {
		return errAssetAttribution
	}
	a := ref.asset
	var variant pgtype.Text
	if ref.image != nil {
		a = steamArtwork(*ref.image, b.ExternalKey, nil)
		if len(a.Variants) != 0 {
			variant = pgtype.Text{String: a.Variants[0].Key, Valid: true}
		}
	} else if o.Capability != "steam.details" || o.OutputSchema < 2 || o.OutputSchema > 3 {
		return errAssetNormalization
	}
	if len(a.Variants) == 0 {
		return errAssetNormalization
	}
	added, err := putAssetVersions(ctx, q, o, b.ExternalKey, []sourceAsset{a}, nil)
	if err != nil {
		return err
	}
	report.VariantsAdded += added
	version, err := findAssetVersion(ctx, q, o, b.ExternalKey, a)
	if err != nil {
		return err
	}
	mapped, err := q.SetGameAssetPresentationItem(ctx, dbgen.SetGameAssetPresentationItemParams{
		GameID: game, Locale: row.Locale, Surface: row.Surface, Slot: ref.slot, Position: ref.position, VersionID: version, VariantKey: variant,
	})
	if err != nil || mapped == 0 {
		return err
	}
	report.MappingsAdded += mapped
	saved, err := q.AddGameAssetLibrary(ctx, dbgen.AddGameAssetLibraryParams{GameID: game, VersionID: version, SelectionOrigin: "media_v2"})
	report.LibraryAdded += saved
	return err
}
