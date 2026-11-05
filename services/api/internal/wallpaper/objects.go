package wallpaper

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"io"
	"net/http"
	"os"
	"time"
)

func matches(o storage.Object, a Asset) bool {
	return o.Key == a.Key && o.SHA256 == a.SHA256 && o.Bytes == a.Bytes && o.ContentType == a.ContentType
}
func (s *Service) ensure(ctx context.Context, a Asset, file *os.File) error {
	head, e := s.objects.Head(ctx, a.Key)
	if errors.Is(e, storage.ErrNotFound) {
		file.Seek(0, io.SeekStart)
		e = s.objects.PutNew(ctx, a.Key, a.ContentType, a.SHA256, file)
		if e != nil && !errors.Is(e, storage.ErrExists) {
			return ErrUnavailable
		}
	} else if e != nil {
		return ErrUnavailable
	} else if !matches(head, a) {
		return ErrConflict
	}
	return s.verify(ctx, a)
}
func (s *Service) verify(ctx context.Context, a Asset) error {
	if s.objects == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	head, e := s.objects.Head(ctx, a.Key)
	if e != nil || !matches(head, a) {
		return ErrUnavailable
	}
	body, meta, e := s.objects.Get(ctx, a.Key)
	if e != nil {
		return ErrUnavailable
	}
	valid := matches(meta, a) && hashBody(body, a)
	body.Close()
	if !valid {
		return ErrUnavailable
	}
	// Only the configured public base plus a validated hash key is used. Never
	// accept supplied URLs or redirects, and never inherit an environment proxy.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, s.objects.URL(a.Key), nil)
	if e != nil {
		return ErrUnavailable
	}
	res, e := client.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != a.ContentType || res.ContentLength != a.Bytes || !hashBody(res.Body, a) {
		return ErrUnavailable
	}
	return nil
}
func hashBody(r io.Reader, a Asset) bool {
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(r, a.Bytes+1))
	return e == nil && n == a.Bytes && fmt.Sprintf("%x", h.Sum(nil)) == a.SHA256
}
