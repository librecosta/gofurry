package wallpaper

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type Metadata struct {
	Title     string `json:"title"`
	SourceURL string `json:"source_url"`
	Credit    string `json:"credit"`
	SortOrder int32  `json:"sort_order"`
}

func (m *Metadata) Validate() error {
	m.Title = strings.TrimSpace(m.Title)
	m.Credit = strings.TrimSpace(m.Credit)
	if !BoundedText(m.Title, 160, true) || !BoundedText(m.Credit, 240, false) || m.SortOrder < -1000000 || m.SortOrder > 1000000 {
		return ErrInvalid
	}
	if m.SourceURL != "" {
		u, e := url.Parse(m.SourceURL)
		if e != nil || len(m.SourceURL) > 2048 || !BoundedText(m.SourceURL, 2048, false) || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return ErrInvalid
		}
		query, e := url.ParseQuery(u.RawQuery)
		if e != nil {
			return ErrInvalid
		}
		for name := range query {
			key := strings.ToLower(name)
			if key == "key" || key == "sig" || key == "auth" || key == "code" {
				return ErrInvalid
			}
			for _, secret := range []string{"token", "password", "secret", "credential", "authorization", "signature", "api_key", "apikey"} {
				if strings.Contains(key, secret) {
					return ErrInvalid
				}
			}
		}
		// OAuth-style fragments are never appropriate public source metadata.
		if strings.Contains(strings.ToLower(u.Fragment), "token=") || strings.Contains(strings.ToLower(u.Fragment), "code=") {
			return ErrInvalid
		}
	}
	return nil
}
func databaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "23505" || pe.Code == "23514") {
		return ErrConflict
	}
	return ErrUnavailable
}
func (s *Service) write(ctx context.Context, fn func(context.Context, *dbgen.Queries) error) error {
	if s.pool == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return ErrUnavailable
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if e = fn(ctx, s.q.WithTx(tx)); e != nil {
		return e
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil // Never replay an unknown commit outcome.
}

// Audit is an explicit allowlist. Never marshal a request, credentials or arbitrary models.
type Summary struct {
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
	ImageKey   string `json:"image_key,omitempty"`
	PreviewKey string `json:"preview_key,omitempty"`
	SourceURL  string `json:"source_url,omitempty"`
	Credit     string `json:"credit,omitempty"`
	SortOrder  int32  `json:"sort_order,omitempty"`
	Revision   int64  `json:"revision,omitempty"`
}

func summary(w dbgen.StartWallpaper) Summary {
	return Summary{w.Title, w.Status, w.ImageKey, w.PreviewKey, w.SourceUrl.String, w.Credit.String, w.SortOrder, w.Revision}
}
func audit(ctx context.Context, q *dbgen.Queries, actor Actor, action string, id uuid.UUID, before, after Summary) error {
	b, e := json.Marshal(before)
	if e != nil {
		return ErrInvalid
	}
	a, e := json.Marshal(after)
	if e != nil {
		return ErrInvalid
	}
	if q.AppendWallpaperAudit(ctx, dbgen.AppendWallpaperAuditParams{ID: uuid.NewV7(), ActorAccountID: actor.ID, ActorName: actor.Name, ActorRole: actor.Role, Action: action, WallpaperID: &id, RequestID: actor.RequestID, BeforeData: b, AfterData: a}) != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Service) AcquireUpload() (func(), error) {
	select {
	case s.uploads <- struct{}{}:
		return func() { <-s.uploads }, nil
	default:
		return nil, ErrBusy
	}
}
func (s *Service) CreateDraft(ctx context.Context, actor Actor, m Metadata, image, preview Asset) (result dbgen.StartWallpaper, err error) {
	if m.Validate() != nil {
		return result, ErrInvalid
	}
	if s.pool == nil {
		return result, ErrUnavailable
	}
	return s.create(ctx, actor, m, image, preview, "draft", "wallpaper.created")
}
func (s *Service) create(ctx context.Context, actor Actor, m Metadata, image, preview Asset, status, action string) (result dbgen.StartWallpaper, err error) {
	err = s.write(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		var e error
		result, e = q.CreateWallpaper(ctx, dbgen.CreateWallpaperParams{ID: uuid.NewV7(), Title: m.Title, Status: status, ImageKey: image.Key, ImageSha256: image.SHA256, ImageType: image.ContentType, ImageBytes: image.Bytes, Width: int32(image.Width), Height: int32(image.Height), PreviewKey: preview.Key, PreviewSha256: preview.SHA256, PreviewBytes: preview.Bytes, PreviewWidth: int32(preview.Width), PreviewHeight: int32(preview.Height), SourceUrl: pgtype.Text{String: m.SourceURL, Valid: m.SourceURL != ""}, Credit: pgtype.Text{String: m.Credit, Valid: m.Credit != ""}, SortOrder: m.SortOrder})
		if e != nil {
			return databaseError(e)
		}
		return audit(ctx, q, actor, action, result.ID, Summary{}, summary(result))
	})
	return
}
func (s *Service) Upload(ctx context.Context, actor Actor, m Metadata, imageFile, previewFile *os.File, imageType, previewType string) (dbgen.StartWallpaper, error) {
	var zero dbgen.StartWallpaper
	if m.Validate() != nil {
		return zero, ErrInvalid
	}
	if s.pool == nil || s.objects == nil {
		return zero, ErrUnavailable
	}
	// Serial full decode keeps the 36MP/4MP memory budget process-bounded.
	select {
	case s.decodes <- struct{}{}:
	case <-ctx.Done():
		return zero, ErrUnavailable
	}
	image, e := ValidateAsset(ctx, imageFile, imageType, false)
	var preview Asset
	if e == nil {
		preview, e = ValidateAsset(ctx, previewFile, previewType, true)
	}
	<-s.decodes
	if e != nil {
		return zero, e
	}
	if _, e = s.q.WallpaperByImage(ctx, image.Key); e == nil {
		return zero, ErrConflict
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return zero, ErrUnavailable
	}
	if e = s.ensure(ctx, image, imageFile); e != nil {
		return zero, e
	}
	if e = s.ensure(ctx, preview, previewFile); e != nil {
		return zero, e
	}
	return s.CreateDraft(ctx, actor, m, image, preview)
}
