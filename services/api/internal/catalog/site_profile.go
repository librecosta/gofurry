package catalog

import (
	"context"
	json "encoding/json/v2"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

const SiteDescriptionLimit = 10 << 10

var htmlOpening = regexp.MustCompile(`<[!/?A-Za-z]`)
var digestHex = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *Service) SetSiteDescription(ctx context.Context, publicID string, revision int64, locale, body string) (int64, error) {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if !ValidPublicID(publicID) || revision < 1 || (locale != "zh" && locale != "en") ||
		len(body) > SiteDescriptionLimit || !validText(body, 0, SiteDescriptionLimit, true) ||
		strings.ContainsFunc(body, func(r rune) bool { return unicode.Is(unicode.Bidi_Control, r) }) || htmlOpening.MatchString(body) {
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
		updated, err := q.SetSiteDescription(ctx, dbgen.SetSiteDescriptionParams{SiteID: row.ID, Locale: locale, Body: body})
		if err != nil {
			return 0, err
		}
		if updated != 1 {
			return 0, ErrNotFound
		}
		next, err := q.UpdateSite(ctx, dbgen.UpdateSiteParams{ID: row.ID, Slug: row.Slug})
		return next.Revision, err
	})
}

type SiteLocalization struct {
	Localization
	Body string `json:"body,omitempty"`
}

type SiteDetail struct {
	PublicID            string             `json:"public_id"`
	Slug                string             `json:"slug"`
	Localizations       []SiteLocalization `json:"localizations"`
	Addresses           []Address          `json:"addresses"`
	Reviewed            bool               `json:"reviewed"`
	PrimaryCategory     string             `json:"primary_category,omitempty"`
	SecondaryCategories []string           `json:"secondary_categories,omitempty"`
	Tags                []NavigationTerm   `json:"tags,omitempty"`
	SupportedLanguages  []string           `json:"supported_languages,omitempty"`
	LogoURL             string             `json:"logo_url,omitempty"`
	Technology          []SiteTechnology   `json:"technology,omitempty"`
}

func (s *Service) PublishedSite(ctx context.Context, publicID string) (out SiteDetail, err error) {
	if !ValidPublicID(publicID) {
		return out, ErrNotFound
	}
	if s.pool == nil {
		return out, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.PublishedSite(ctx, publicID)
		if err != nil {
			return err
		}
		out.PublicID, out.Slug, out.Reviewed = row.PublicID, row.Slug, row.InteractionEligible
		out.Addresses = []Address{}
		var addresses []Address
		if json.Unmarshal(row.Localizations, &out.Localizations) != nil || json.Unmarshal(row.Addresses, &addresses) != nil {
			return ErrUnavailable
		}
		if !out.Reviewed {
			return nil
		}
		out.PrimaryCategory, out.SecondaryCategories = row.PrimaryCategory.String, row.SecondaryCategories
		for _, tag := range NavigationTags() {
			if slices.Contains(row.NavigationTags, tag.Code) {
				out.Tags = append(out.Tags, tag)
			}
		}
		for _, language := range row.SupportedLanguages {
			if NavigationLanguage(language) {
				out.SupportedLanguages = append(out.SupportedLanguages, language)
			}
		}
		out.LogoURL = s.assets.SiteAssetURL(storage.SiteLogo, row.LogoAssetKey.String)
		for _, address := range addresses {
			if address.URL = PublicVisitURL(address.URL); address.URL != "" {
				out.Addresses = append(out.Addresses, address)
			}
		}
		if len(out.Addresses) != 0 && out.Addresses[0].Kind == "primary" {
			out.Technology, err = readSiteTechnology(ctx, q, publicID, out.Addresses[0].URL)
		}
		return err
	})
	if err != nil {
		return SiteDetail{}, classify(err)
	}
	return out, nil
}

type SiteTechnology struct {
	Kind            string     `json:"kind"`
	Technology      string     `json:"technology,omitempty"`
	HTTPStatus      int        `json:"http_status,omitempty"`
	Source          string     `json:"source"`
	Ruleset         string     `json:"ruleset"`
	Confidence      string     `json:"confidence,omitempty"`
	FirstObserved   *time.Time `json:"first_observed_at,omitempty"`
	LatestConfirmed time.Time  `json:"latest_confirmed_at"`
}

var publicTechnologies = []string{
	"wordpress", "ghost", "drupal", "discourse", "xenforo", "mediawiki",
	"nextjs", "nuxt", "astro", "vercel", "netlify", "github-pages", "cloudflare",
	"fastly", "cloudfront", "nginx", "apache", "caddy",
}

func readSiteTechnology(ctx context.Context, q *dbgen.Queries, id, address string) ([]SiteTechnology, error) {
	primary, err := url.Parse(address)
	if err != nil || primary.Hostname() == "" {
		return nil, nil
	}
	if _, err = netip.ParseAddr(primary.Hostname()); err == nil {
		return nil, nil
	}
	keys := []string{"http.response", "tls.certificate"}
	for _, name := range publicTechnologies {
		keys = append(keys, "tech.detected."+name)
	}
	rows, err := q.SiteTechnology(ctx, dbgen.SiteTechnologyParams{PublicID: id, FactKeys: keys})
	if err != nil {
		return nil, err
	}
	var out []SiteTechnology
	seen := make(map[string]bool)
	for _, row := range rows {
		if row.ExternalKey != primary.Scheme+"://"+primary.Host || seen[row.FactKey] || !row.ConfirmedAt.Valid || !row.ObservedAt.Valid {
			continue
		}
		fact, ok := publicTechnology(row, primary.Hostname())
		if !ok {
			continue
		}
		seen[row.FactKey] = true
		out = append(out, fact)
		if len(out) == 5 {
			break
		}
	}
	return out, nil
}

func publicTechnology(row dbgen.SiteTechnologyRow, host string) (SiteTechnology, bool) {
	fact := SiteTechnology{Source: host, Ruleset: row.Ruleset, LatestConfirmed: row.ConfirmedAt.Time.UTC()}
	if row.FirstObservedAt.Valid {
		at := row.FirstObservedAt.Time.UTC()
		fact.FirstObserved = &at
	}
	switch row.FactKey {
	case "http.response":
		var value struct {
			Origin string `json:"origin"`
			Status int    `json:"status"`
		}
		fact.Kind = "http"
		if row.Ruleset != "web-passive-1" || json.Unmarshal(row.Value, &value) != nil ||
			value.Origin != row.ExternalKey || value.Status < 100 || value.Status > 599 {
			return fact, false
		}
		fact.HTTPStatus = value.Status
	case "tls.certificate":
		var value struct {
			Origin      string    `json:"origin"`
			Hostname    string    `json:"hostname"`
			Verified    bool      `json:"verified"`
			NotBefore   time.Time `json:"not_before"`
			NotAfter    time.Time `json:"not_after"`
			Certificate string    `json:"certificate_sha256"`
			SPKI        string    `json:"spki_sha256"`
		}
		fact.Kind = "tls"
		if row.Ruleset != "gofurry-evidence-1" || json.Unmarshal(row.Value, &value) != nil || !value.Verified ||
			value.Origin != row.ExternalKey || value.Hostname != host || value.NotBefore.IsZero() ||
			value.NotBefore.After(row.ObservedAt.Time) || !value.NotAfter.After(fact.LatestConfirmed) ||
			!digestHex.MatchString(value.Certificate) || !digestHex.MatchString(value.SPKI) {
			return fact, false
		}
	default:
		var value struct {
			ID         string   `json:"id"`
			Confidence string   `json:"confidence"`
			Evidence   []string `json:"evidence"`
		}
		fact.Kind = "fingerprint"
		if row.Ruleset != "web-passive-1" || json.Unmarshal(row.Value, &value) != nil ||
			row.FactKey != "tech.detected."+value.ID || !slices.Contains(publicTechnologies, value.ID) ||
			(value.Confidence != "high" && value.Confidence != "medium") || len(value.Evidence) < 1 || len(value.Evidence) > 3 {
			return fact, false
		}
		fact.Technology, fact.Confidence = value.ID, value.Confidence
	}
	return fact, true
}
