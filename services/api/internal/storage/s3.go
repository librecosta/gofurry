// Package storage provides the concrete S3-compatible object operations needed
// by public backgrounds. It does not read configuration or choose a data region.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var (
	ErrNotFound   = errors.New("object not found")
	ErrExists     = errors.New("object already exists; refusing overwrite")
	backgroundKey = regexp.MustCompile(`^start/backgrounds/[a-f0-9]{64}\.(avif|webp)$`)
)

type Object struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
	ETag        string `json:"etag"`
	SHA256      string `json:"sha256"`
}

type Store struct {
	client             *s3.Client
	bucket, publicBase string
}

func New(cfg platform.StorageConfig) *Store {
	return &Store{bucket: cfg.Bucket, publicBase: cfg.PublicBaseURL, client: s3.New(s3.Options{
		Region: cfg.Region, BaseEndpoint: aws.String(cfg.Endpoint), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		HTTPClient:  &http.Client{Timeout: 30 * time.Second}, RetryMaxAttempts: 2,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})}
}

func (s *Store) object(key, contentType, etag, hash string, size int64) Object {
	u, _ := url.JoinPath(s.publicBase, key)
	return Object{Key: key, URL: u, Bytes: size, ContentType: contentType, ETag: etag, SHA256: hash}
}

func (s *Store) Head(ctx context.Context, key string) (Object, error) {
	res, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return Object{}, safeError("head", err)
	}
	return s.object(key, aws.ToString(res.ContentType), aws.ToString(res.ETag), res.Metadata["sha256"], aws.ToInt64(res.ContentLength)), nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	res, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, Object{}, safeError("get", err)
	}
	return res.Body, s.object(key, aws.ToString(res.ContentType), aws.ToString(res.ETag), res.Metadata["sha256"], aws.ToInt64(res.ContentLength)), nil
}

// PutNew is deliberately limited to content-addressed background imports. HEAD
// checks first; the conditional PUT also prevents overwrites in a concurrent race.
func (s *Store) PutNew(ctx context.Context, key, contentType, hash string, body io.ReadSeeker) error {
	if !backgroundKey.MatchString(key) || (contentType != "image/avif" && contentType != "image/webp") {
		return errors.New("writes require a hashed AVIF/WebP key under start/backgrounds/")
	}
	if key != "start/backgrounds/"+hash+"."+contentType[len("image/"):] {
		return errors.New("background key, hash and media type must agree")
	}
	if _, err := s.Head(ctx, key); err == nil {
		return ErrExists
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket, Key: &key, Body: body, ContentType: &contentType,
		IfNoneMatch: aws.String("*"), CacheControl: aws.String("public, max-age=31536000, immutable"),
		Metadata: map[string]string{"sha256": hash},
	})
	if err != nil {
		return safeError("put", err)
	}
	return nil
}

func safeError(operation string, err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return ErrNotFound
		case "PreconditionFailed", "ConditionalRequestConflict":
			return ErrExists
		}
	}
	// SDK errors can contain signed request details. Expose only the operation.
	return fmt.Errorf("object storage %s failed", operation)
}
