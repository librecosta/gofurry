package catalog

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type siteInputError string

func (e siteInputError) Error() string { return string(e) }
func (e siteInputError) Unwrap() error { return ErrInvalid }

const (
	ErrSiteNameRequired      = siteInputError("site needs a name")
	ErrSiteChineseRequired   = siteInputError("site needs a Chinese name")
	ErrSiteAddressRequired   = siteInputError("site needs a primary URL")
	ErrSiteAddressInvalid    = siteInputError("site primary URL is invalid")
	ErrSiteCategoryRequired  = siteInputError("site needs a primary category")
	ErrSiteSecondaryConflict = siteInputError("site secondary categories conflict with the primary category")
	ErrSiteReviewRequired    = siteInputError("site needs SFW review")
)

type SiteAddressConflict struct{ PublicID string }

func (e SiteAddressConflict) Error() string { return "site address already exists: " + e.PublicID }
func (e SiteAddressConflict) Unwrap() error { return ErrConflict }

var explicitURLScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

func normalizeSiteIntakeAddress(addresses []Address) ([]Address, *url.URL, error) {
	for i, address := range addresses {
		if address.Kind != "primary" {
			continue
		}
		raw := strings.TrimSpace(address.URL)
		if raw == "" {
			break
		}
		if !explicitURLScheme.MatchString(raw) {
			raw = "https://" + raw
		}
		normalized, err := normalizeURL(raw)
		if err != nil {
			return nil, nil, ErrSiteAddressInvalid
		}
		parsed, err := url.Parse(normalized)
		if err != nil {
			return nil, nil, ErrSiteAddressInvalid
		}
		addresses = slices.Clone(addresses)
		addresses[i].URL = normalized
		return addresses, parsed, nil
	}
	return nil, nil, ErrSiteAddressRequired
}

func siteIntakeSlug(u *url.URL) string {
	host := u.Hostname()
	if first, rest, ok := strings.Cut(host, "."); ok &&
		(first == "www" || strings.HasPrefix(first, "www") && decimal.MatchString(first[3:])) {
		host = rest
	}
	slug := strings.Join(strings.FieldsFunc(host, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}), "-")
	if len(slug) > 80 {
		slug = strings.TrimRight(slug[:80], "-")
	}
	if slug == "" {
		return "site"
	}
	return slug
}

func checkSiteIntakeAddress(ctx context.Context, q *dbgen.Queries, u *url.URL) error {
	// Root URLs with and without a slash identify the same page. Keep paths,
	// queries and explicit ports so separate sites on one domain can coexist.
	root := u.Path == "" || u.Path == "/"
	if root {
		u.Path = ""
	}
	key := strings.TrimPrefix(u.String(), u.Scheme+":")
	if err := q.LockSiteIntakeAddress(ctx, key); err != nil {
		return err
	}
	var candidates []string
	for _, scheme := range []string{"http", "https"} {
		candidate := *u
		candidate.Scheme = scheme
		candidates = append(candidates, candidate.String())
		if root {
			candidate.Path = "/"
			candidates = append(candidates, candidate.String())
		}
	}
	id, err := q.FindSiteByActiveAddress(ctx, candidates)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return SiteAddressConflict{PublicID: id}
}

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
			if err := requireSitePresentation(ctx, q, id); err != nil {
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
		action := "publish"
		if desired == Archived {
			action = "archive"
		} else if row.PublishedAt.Valid {
			action = "restore"
		}
		return changeSiteVisibility(ctx, q, row, action, PublicationDecision{Actor: "catalog", Reason: "Explicit editorial status change"})
	})
}

func requireSitePublication(ctx context.Context, q *dbgen.Queries, site dbgen.Site) error {
	if err := requireSitePresentation(ctx, q, site.ID); err != nil {
		return err
	}
	if !site.PrimaryCategory.Valid {
		return ErrSiteCategoryRequired
	}
	if site.ContentRating != "sfw" {
		return ErrSiteReviewRequired
	}
	return nil
}

func requireSitePresentation(ctx context.Context, q *dbgen.Queries, id uuid.UUID) error {
	fields, err := q.SitePublicationFields(ctx, id)
	if err != nil {
		return err
	}
	switch {
	case !fields.HasName:
		return ErrSiteNameRequired
	case !fields.HasAddress:
		return ErrSiteAddressRequired
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
