package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gen2brain/gav1d/avif"
	"golang.org/x/image/webp"
)

type SiteAssetKind string

const (
	SiteLogo  SiteAssetKind = "logo"
	SiteCover SiteAssetKind = "cover"
)

var ErrSiteAsset = errors.New("invalid Site asset")
var siteLogoKey = regexp.MustCompile(`^sites/logos/[a-f0-9]{64}[.]webp$`)
var siteCoverKey = regexp.MustCompile(`^sites/covers/[a-f0-9]{64}[.]avif$`)

func (k SiteAssetKind) ValidKey(key string) bool {
	switch k {
	case SiteLogo:
		return siteLogoKey.MatchString(key)
	case SiteCover:
		return siteCoverKey.MatchString(key)
	}
	return false
}

func (k SiteAssetKind) format() (prefix, extension string, limit int64) {
	switch k {
	case SiteLogo:
		return "sites/logos/", "webp", 1 << 20
	case SiteCover:
		return "sites/covers/", "avif", 4 << 20
	}
	return "", "", 0
}

type SiteAsset struct {
	Object
	Width  int `json:"width"`
	Height int `json:"height"`
}

func inspectSiteAsset(kind SiteAssetKind, raw []byte) (asset SiteAsset, err error) {
	defer func() {
		if recover() != nil {
			asset, err = SiteAsset{}, ErrSiteAsset
		}
	}()
	prefix, ext, limit := kind.format()
	if limit == 0 || len(raw) < 12 || int64(len(raw)) > limit {
		return asset, ErrSiteAsset
	}
	var config image.Config
	var decoded image.Image
	if kind == SiteLogo {
		if !siteWebPEnvelope(raw) {
			return asset, ErrSiteAsset
		}
		config, err = webp.DecodeConfig(bytes.NewReader(raw))
		if err != nil || config.Width < 1 || config.Width > 1024 || config.Width != config.Height {
			return asset, ErrSiteAsset
		}
		decoded, err = webp.Decode(bytes.NewReader(raw))
	} else {
		if !siteAVIFEnvelope(raw) {
			return asset, ErrSiteAsset
		}
		config, err = avif.DecodeConfig(bytes.NewReader(raw))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 2560 || config.Height > 1440 || config.Width*9 != config.Height*16 {
			return asset, ErrSiteAsset
		}
		decoded, err = avif.Decode(bytes.NewReader(raw), avif.Options{FrameSizeLimit: 2560 * 1440, MaxFrames: 1})
	}
	if err != nil || decoded == nil || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return asset, ErrSiteAsset
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	return SiteAsset{Object: Object{Key: prefix + hash + "." + ext, Bytes: int64(len(raw)), SHA256: hash, ContentType: "image/" + ext}, Width: config.Width, Height: config.Height}, nil
}

// SiteAssetURL is a pure projection of configured storage and a typed object key.
func (s *Store) SiteAssetURL(kind SiteAssetKind, key string) string {
	if s == nil || !kind.ValidKey(key) {
		return ""
	}
	u, err := url.Parse(s.publicBase)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" ||
		u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.Contains(u.Path, "\\") {
		return ""
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return ""
		}
	}
	return u.JoinPath(key).String()
}

// ImportSiteAsset hashes the same bounded bytes that are decoded and uploaded.
func (s *Store) ImportSiteAsset(ctx context.Context, kind SiteAssetKind, input io.Reader, preflight bool) (SiteAsset, error) {
	_, _, limit := kind.format()
	if limit == 0 {
		return SiteAsset{}, ErrSiteAsset
	}
	raw, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err != nil || ctx.Err() != nil {
		return SiteAsset{}, ErrSiteAsset
	}
	asset, err := inspectSiteAsset(kind, raw)
	if err != nil || preflight {
		return asset, err
	}
	if s.SiteAssetURL(kind, asset.Key) == "" {
		return SiteAsset{}, ErrSiteAsset
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket, Key: &asset.Key, Body: bytes.NewReader(raw), ContentType: &asset.ContentType,
		IfNoneMatch: aws.String("*"), CacheControl: aws.String("public, max-age=31536000, immutable"),
		Metadata: map[string]string{"sha256": asset.SHA256},
	})
	if err != nil && !errors.Is(safeError("put", err), ErrExists) {
		return SiteAsset{}, safeError("put", err)
	}
	return s.VerifySiteAsset(ctx, kind, asset.Key)
}

// VerifySiteAsset rechecks the immutable bytes and the direct public response
// before a Site revision may reference the object. Public requests never call it.
func (s *Store) VerifySiteAsset(ctx context.Context, kind SiteAssetKind, key string) (SiteAsset, error) {
	public := s.SiteAssetURL(kind, key)
	if public == "" {
		return SiteAsset{}, ErrSiteAsset
	}
	_, extension, limit := kind.format()
	wantType := "image/" + extension
	head, err := s.Head(ctx, key)
	if err != nil {
		return SiteAsset{}, err
	}
	if head.Bytes < 1 || head.Bytes > limit || head.ContentType != wantType {
		return SiteAsset{}, ErrSiteAsset
	}
	body, object, err := s.Get(ctx, key)
	if err != nil {
		return SiteAsset{}, err
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil || int64(len(raw)) != head.Bytes || object.ContentType != wantType {
		return SiteAsset{}, ErrSiteAsset
	}
	asset, err := inspectSiteAsset(kind, raw)
	if err != nil || asset.Key != key || head.SHA256 != asset.SHA256 || object.SHA256 != asset.SHA256 {
		return SiteAsset{}, ErrSiteAsset
	}
	client := http.Client{Transport: s.transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, public, nil)
	if err != nil {
		return SiteAsset{}, ErrSiteAsset
	}
	response, err := client.Do(request)
	if err != nil {
		return SiteAsset{}, errors.New("Site asset public delivery unavailable")
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || response.StatusCode != http.StatusOK || media != wantType {
		return SiteAsset{}, ErrSiteAsset
	}
	delivered, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || !bytes.Equal(delivered, raw) {
		return SiteAsset{}, ErrSiteAsset
	}
	asset.URL = public
	asset.ETag = head.ETag
	return asset, nil
}
