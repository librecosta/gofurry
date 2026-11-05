// Package wallpaper owns this deployment's official wallpaper records, not Admin identity.
package wallpaper

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

var (
	ErrInvalid     = errors.New("invalid wallpaper input")
	ErrConflict    = errors.New("wallpaper revision or state conflict")
	ErrUnavailable = errors.New("wallpaper dependency unavailable")
	ErrNotFound    = errors.New("wallpaper not found")
	ErrBusy        = errors.New("wallpaper resource budget exhausted")
)

// The small interface describes only immutable background objects. No delete,
// bucket management or arbitrary URL proxy is exposed.
type Objects interface {
	Head(context.Context, string) (storage.Object, error)
	Get(context.Context, string) (io.ReadCloser, storage.Object, error)
	PutNew(context.Context, string, string, string, io.ReadSeeker) error
	URL(string) string
}
type Actor struct {
	ID                    *uuid.UUID
	Name, Role, RequestID string
}

func BoundedText(value string, max int, required bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max || required && len(value) == 0 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

type Service struct {
	pool       *pgxpool.Pool
	q          *dbgen.Queries
	objects    Objects
	configured bool
	uploads    chan struct{}
	decodes    chan struct{}
	cacheMu    sync.Mutex
	cache      []PublicBackground
	cacheAt    time.Time
	generation uint64
}

func New(pool *pgxpool.Pool, objects Objects, configured bool) *Service {
	s := &Service{pool: pool, objects: objects, configured: configured, uploads: make(chan struct{}, 2), decodes: make(chan struct{}, 1)}
	if pool != nil {
		s.q = dbgen.New(pool)
	}
	return s
}
func (s *Service) Ready(ctx context.Context) error {
	if s.pool == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.pool.Ping(ctx) != nil || s.q.CheckWallpaperSchema(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
