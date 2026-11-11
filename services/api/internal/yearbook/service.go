package yearbook

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var (
	ErrInvalid     = errors.New("invalid_yearbook_request")
	ErrMissing     = errors.New("yearbook_not_found")
	ErrUnavailable = errors.New("yearbook_database_or_snapshot_unavailable")
	ErrConflict    = errors.New("yearbook_revision_conflict")
	ErrEarly       = errors.New("yearbook_grace_period")
	ErrInputLimit  = errors.New("yearbook_input_limit")
)

const maxDocument = 64 << 10

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }

func ParseYear(value string) (int32, error) {
	year, err := strconv.ParseInt(value, 10, 32)
	if len(value) != 4 || err != nil || strconv.FormatInt(year, 10) != value || !validYear(int32(year)) {
		return 0, ErrInvalid
	}
	return int32(year), nil
}

func validYear(year int32) bool { return year >= 2000 && year <= 9998 }

func eligibleThrough(now time.Time) int32 {
	now = now.UTC()
	latest := now.Year() - 1
	if now.Before(time.Date(now.Year(), 1, 8, 0, 0, 0, 0, time.UTC)) {
		latest--
	}
	return min(9998, int32(latest))
}

func bounds(year int32) (time.Time, time.Time) {
	return time.Date(int(year), 1, 1, 0, 0, 0, 0, time.UTC), time.Date(int(year)+1, 1, 1, 0, 0, 0, 0, time.UTC)
}

func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }
func date(t time.Time) pgtype.Date         { return pgtype.Date{Time: t.UTC(), Valid: true} }

type Assessment struct {
	Year      int32    `json:"year"`
	Eligible  bool     `json:"eligible"`
	Reasons   []string `json:"reasons"`
	Document  Document `json:"document"`
	Digest    string   `json:"digest,omitempty"`
	Revision  int32    `json:"revision,omitempty"`
	Published bool     `json:"published"`
}

type PublicDocument struct {
	PublicID      string   `json:"public_id"`
	Slug          string   `json:"slug"`
	Year          int32    `json:"year"`
	Revision      int32    `json:"revision"`
	PublishedAt   string   `json:"published_at"`
	SiteAvailable bool     `json:"site_available"`
	CurrentNames  []Name   `json:"current_names"`
	Document      Document `json:"document"`
}

func documentBytes(doc Document) ([]byte, string, error) {
	raw, err := json.Marshal(doc, json.Deterministic(true))
	if err != nil || len(raw) > maxDocument {
		return nil, "", ErrInputLimit
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

func assess(ctx context.Context, q *dbgen.Queries, site dbgen.Site, year int32, now time.Time) (Assessment, []byte, error) {
	result := Assessment{Year: year, Reasons: []string{}}
	start, end := bounds(year)
	names, err := q.YearbookSiteNames(ctx, site.ID)
	if err != nil {
		return result, nil, err
	}
	rows, err := q.YearbookCoverage(ctx, dbgen.YearbookCoverageParams{SiteID: site.ID, FromDay: date(start), UntilDay: date(end)})
	if err != nil {
		return result, nil, err
	}
	facts, err := q.YearbookFacts(ctx, dbgen.YearbookFactsParams{SiteID: site.ID, FromTime: stamp(start), UntilTime: stamp(end), InputAsOf: stamp(now)})
	if err != nil {
		return result, nil, err
	}
	prior, err := q.YearbookPriorFacts(ctx, dbgen.YearbookPriorFactsParams{SiteID: site.ID, FromTime: stamp(start), InputAsOf: stamp(now)})
	if err != nil {
		return result, nil, err
	}
	if len(rows) > 4096 || len(facts) > 4096 || len(prior) > 256 {
		return result, nil, ErrInputLimit
	}
	ordered := make([]dbgen.YearbookFactsRow, 0, len(prior)+len(facts))
	for _, row := range prior {
		ordered = append(ordered, dbgen.YearbookFactsRow(row))
	}
	slices.SortFunc(ordered, func(a, b dbgen.YearbookFactsRow) int {
		return cmp.Or(a.FirstObservedAt.Time.Compare(b.FirstObservedAt.Time), cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID.String(), b.ID.String()))
	})
	ordered = append(ordered, facts...)
	coverage, confirmed := summarizeCoverage(start, end, rows)
	result.Document = Document{Algorithm: algorithm, Names: editorialNames(names), Coverage: coverage}
	baseline := factsForYear(&result.Document, ordered, confirmed, start, end)
	if year > eligibleThrough(now) {
		result.Reasons = append(result.Reasons, "grace_period_open")
	}
	if site.Status != catalog.Published || len(result.Document.Names) == 0 {
		result.Reasons = append(result.Reasons, "site_not_published")
	}
	if coverage.Days < minDays {
		result.Reasons = append(result.Reasons, "insufficient_evidence")
	}
	first, _ := time.Parse(time.DateOnly, coverage.RecordedFrom)
	last, _ := time.Parse(time.DateOnly, coverage.RecordedTo)
	if first.IsZero() || last.Sub(first) < (minSpan-1)*24*time.Hour {
		result.Reasons = append(result.Reasons, "coverage_span_too_short")
	}
	if !baseline {
		result.Reasons = append(result.Reasons, "verified_baseline_missing")
	}
	result.Eligible = len(result.Reasons) == 0
	raw, digest, err := documentBytes(result.Document)
	result.Digest = digest
	return result, raw, err
}

func known(err error) error {
	switch {
	case err == nil, errors.Is(err, ErrInputLimit), errors.Is(err, ErrConflict), errors.Is(err, ErrInvalid), errors.Is(err, ErrEarly):
		return err
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, ErrMissing):
		return ErrMissing
	default:
		return ErrUnavailable
	}
}

func (s *Service) Preview(ctx context.Context, id string, year int32) (result Assessment, err error) {
	return s.generate(ctx, id, year, false, false)
}

func (s *Service) Generate(ctx context.Context, id string, year int32) (result Assessment, err error) {
	return s.generate(ctx, id, year, true, false)
}

func (s *Service) generate(ctx context.Context, id string, year int32, apply, automatic bool) (result Assessment, err error) {
	if !catalog.ValidPublicID(id) || !validYear(year) {
		return result, ErrInvalid
	}
	if s.pool == nil {
		return result, ErrUnavailable
	}
	now := s.now().UTC()
	if apply && year > eligibleThrough(now) {
		return result, ErrEarly
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	options := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if apply {
		options.AccessMode = pgx.ReadWrite
	}
	for range 3 {
		err = pgx.BeginTxFunc(ctx, s.pool, options, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			find := q.FindYearbookSite
			if apply {
				find = q.LockYearbookSite
			}
			site, err := find(ctx, id)
			if err != nil {
				return err
			}
			key := dbgen.YearbookHeadParams{SiteID: site.ID, Year: year}
			head, err := q.YearbookHead(ctx, key)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if automatic && (site.Status != catalog.Published || head.PublishedRevision.Valid || head.Status == "hidden") {
				return nil
			}
			var raw []byte
			result, raw, err = assess(ctx, q, site, year, now)
			if err != nil || !apply || site.Status != catalog.Published {
				return err
			}
			err = q.RecordYearbookCheck(ctx, dbgen.RecordYearbookCheckParams{
				SiteID: site.ID, Year: year, PublicID: id, StoredSlug: site.Slug, CheckedAt: stamp(now),
			})
			if err != nil || !result.Eligible {
				return err
			}
			result.Revision = head.LatestRevision
			if result.Revision > 0 {
				latest, err := q.YearbookRevision(ctx, dbgen.YearbookRevisionParams{SiteID: site.ID, Year: year, Revision: result.Revision})
				if err != nil {
					return err
				}
				if latest.Digest == result.Digest {
					result.Published = head.Status == "published" && head.PublishedRevision.Int32 == result.Revision
					return nil
				}
			}
			result.Revision++
			if err = q.AddYearbookRevision(ctx, dbgen.AddYearbookRevisionParams{
				SiteID: site.ID, Year: year, Revision: result.Revision, Digest: result.Digest,
				Algorithm: algorithm, Document: raw, InputAsOf: stamp(now), GeneratedAt: stamp(now),
			}); err != nil {
				return err
			}
			if err = q.AdvanceYearbookRevision(ctx, dbgen.AdvanceYearbookRevisionParams{SiteID: site.ID, Year: year, LatestRevision: result.Revision}); err != nil {
				return err
			}
			if head.PublishedRevision.Valid || head.Status == "hidden" {
				return nil
			}
			if err = publish(ctx, q, site, year, result.Revision, result.Digest, now); err != nil {
				return err
			}
			result.Published = true
			return nil
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || (pgErr.Code != "40001" && pgErr.Code != "40P01") {
			break
		}
	}
	return result, known(err)
}

func publish(ctx context.Context, q *dbgen.Queries, site dbgen.Site, year, revision int32, digest string, now time.Time) error {
	n, err := q.PublishYearbookRevision(ctx, dbgen.PublishYearbookRevisionParams{
		SiteID: site.ID, Year: year, Revision: revision, Digest: digest, PublishedAt: stamp(now),
	})
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}

func (s *Service) Publish(ctx context.Context, id string, year, revision int32, digest string) error {
	if !catalog.ValidPublicID(id) || !validYear(year) || revision < 1 || !hash.MatchString(digest) {
		return ErrInvalid
	}
	now := s.now().UTC()
	if year > eligibleThrough(now) {
		return ErrEarly
	}
	if s.pool == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return known(pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		site, err := q.LockYearbookSite(ctx, id)
		if err != nil {
			return err
		}
		head, err := q.YearbookHead(ctx, dbgen.YearbookHeadParams{SiteID: site.ID, Year: year})
		if err != nil {
			return err
		}
		if site.Status != catalog.Published || head.LatestRevision != revision {
			return ErrConflict
		}
		if head.Status == "published" && head.PublishedRevision.Int32 == revision && head.PublishedDigest.String == digest {
			return nil
		}
		return publish(ctx, q, site, year, revision, digest, now)
	}))
}

func (s *Service) Hide(ctx context.Context, id string, year int32) error {
	if !catalog.ValidPublicID(id) || !validYear(year) {
		return ErrInvalid
	}
	if s.pool == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	n, err := dbgen.New(s.pool).HideYearbook(ctx, dbgen.HideYearbookParams{PublicID: id, Year: year, CheckedAt: stamp(s.now())})
	if err == nil && n == 0 {
		return ErrMissing
	}
	return known(err)
}

func (s *Service) Public(ctx context.Context, id string, year int32) (PublicDocument, error) {
	last := eligibleThrough(s.now())
	if !catalog.ValidPublicID(id) || !validYear(year) || year > last {
		return PublicDocument{}, ErrMissing
	}
	if s.pool == nil {
		return PublicDocument{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row, err := dbgen.New(s.pool).PublishedYearbook(ctx, dbgen.PublishedYearbookParams{PublicID: id, Year: year, LastYear: last})
	if err != nil {
		return PublicDocument{}, known(err)
	}
	var doc Document
	if len(row.Document) > maxDocument || json.Unmarshal(row.Document, &doc, json.RejectUnknownMembers(true)) != nil || doc.Algorithm != algorithm {
		return PublicDocument{}, ErrUnavailable
	}
	_, digest, err := documentBytes(doc)
	if err != nil || digest != row.Digest {
		return PublicDocument{}, ErrUnavailable
	}
	var names []dbgen.YearbookSiteNamesRow
	if err := json.Unmarshal(row.CurrentNames, &names); err != nil {
		return PublicDocument{}, ErrUnavailable
	}
	return PublicDocument{row.PublicID, row.Slug, row.Year, row.Revision, instant(row.PublishedAt.Time), row.SiteAvailable, editorialNames(names), doc}, nil
}

func (s *Service) Years(ctx context.Context, id string) ([]int32, error) {
	if !catalog.ValidPublicID(id) {
		return nil, ErrMissing
	}
	if s.pool == nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	q := dbgen.New(s.pool)
	years, err := q.PublishedYearbookList(ctx, dbgen.PublishedYearbookListParams{PublicID: id, LastYear: eligibleThrough(s.now())})
	if err != nil || len(years) > 0 {
		return years, known(err)
	}
	site, err := q.FindYearbookSite(ctx, id)
	if err == nil && site.Status != catalog.Published {
		err = ErrMissing
	}
	return years, known(err)
}
