package catalog

import (
	"context"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (s *Service) CreateSite(ctx context.Context, in SiteInput) (dbgen.Site, error) {
	var err error
	in.Editorial, err = normalizeEditorial(in.Editorial, "")
	if err != nil {
		return dbgen.Site{}, err
	}
	in.Addresses, err = normalizeAddresses(in.Addresses)
	if err != nil {
		return dbgen.Site{}, err
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Site, error) {
		id := uuid.NewV7()
		row, err := create(s, func(publicID string) (dbgen.Site, error) {
			return q.CreateSite(ctx, dbgen.CreateSiteParams{ID: id, PublicID: publicID, Slug: in.Slug})
		})
		if err != nil {
			return row, err
		}
		return row, saveSiteContent(ctx, q, row.ID, in)
	})
}

func (s *Service) EditSite(ctx context.Context, id uuid.UUID, revision int64, in SiteInput) (dbgen.Site, error) {
	var err error
	in.Addresses, err = normalizeAddresses(in.Addresses)
	if err != nil {
		return dbgen.Site{}, err
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Site, error) {
		row, err := q.LockSite(ctx, id)
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
		if err = saveSiteContent(ctx, q, id, in); err != nil {
			return row, err
		}
		if row.Status == Published {
			if err = requireSitePublication(ctx, q, id); err != nil {
				return row, err
			}
		}
		if err = q.CloseRemovedSiteBindings(ctx, &id); err != nil {
			return row, err
		}
		// A separate statement sees any rollup committed while the close waited on its binding lock.
		if err = q.CloseRemovedSiteCoverage(ctx, &id); err != nil {
			return row, err
		}
		return q.UpdateSite(ctx, dbgen.UpdateSiteParams{ID: id, Slug: in.Slug})
	})
}

func (s *Service) SetSiteStatus(ctx context.Context, id uuid.UUID, revision int64, desired string) (dbgen.Site, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Site, error) {
		row, err := q.LockSite(ctx, id)
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
			if err = requireSitePublication(ctx, q, id); err != nil {
				return row, err
			}
		}
		return q.ChangeSiteStatus(ctx, dbgen.ChangeSiteStatusParams{ID: id, NewStatus: desired})
	})
}

func requireSitePublication(ctx context.Context, q *dbgen.Queries, id uuid.UUID) error {
	ready, err := q.CanPublishSite(ctx, id)
	if err != nil {
		return err
	}
	if !ready {
		return ErrInvalid
	}
	return nil
}

func saveSiteContent(ctx context.Context, q *dbgen.Queries, id uuid.UUID, in SiteInput) error {
	locales := make([]string, 0, len(in.Localizations))
	for _, localization := range in.Localizations {
		locales = append(locales, localization.Locale)
	}
	if err := q.RemoveOmittedSiteLocalizations(ctx, dbgen.RemoveOmittedSiteLocalizationsParams{SiteID: id, Locales: locales}); err != nil {
		return err
	}
	for _, localization := range in.Localizations {
		if err := q.AddSiteLocalization(ctx, dbgen.AddSiteLocalizationParams{
			SiteID: id, Locale: localization.Locale, Name: localization.Name, Summary: localization.Summary,
		}); err != nil {
			return err
		}
	}
	if err := q.DisableSiteAddresses(ctx, id); err != nil {
		return err
	}
	for _, address := range in.Addresses {
		if err := q.SaveSiteAddress(ctx, dbgen.SaveSiteAddressParams{ID: uuid.NewV7(), SiteID: id, Url: address.URL, Kind: address.Kind}); err != nil {
			return err
		}
	}
	return nil
}
