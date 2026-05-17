package blobstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2Config is the env-derived configuration for the R2 blobstore.
type R2Config struct {
	AccountID     string // CF account id, used to build the S3 endpoint
	AccessKey     string
	SecretKey     string
	Bucket        string
	PublicBaseURL string // e.g. "https://media.example.com" or "https://pub-xxxxx.r2.dev"
}

// LoadR2ConfigFromEnv reads R2_ACCOUNT_ID, R2_ACCESS_KEY, R2_SECRET_KEY,
// R2_BUCKET, R2_PUBLIC_BASE_URL. Returns a *MissingEnvError if any is unset.
func LoadR2ConfigFromEnv() (*R2Config, error) {
	get := func(k string) (string, error) {
		v := os.Getenv(k)
		if v == "" {
			return "", &MissingEnvError{Name: k}
		}
		return v, nil
	}
	cfg := &R2Config{}
	for _, pair := range []struct {
		dst *string
		key string
	}{
		{&cfg.AccountID, "R2_ACCOUNT_ID"},
		{&cfg.AccessKey, "R2_ACCESS_KEY"},
		{&cfg.SecretKey, "R2_SECRET_KEY"},
		{&cfg.Bucket, "R2_BUCKET"},
		{&cfg.PublicBaseURL, "R2_PUBLIC_BASE_URL"},
	} {
		v, err := get(pair.key)
		if err != nil {
			return nil, err
		}
		*pair.dst = v
	}
	return cfg, nil
}

// R2 implements BlobStore against Cloudflare R2 via the S3-compatible API.
type R2 struct {
	cfg    *R2Config
	client *s3.Client
}

// NewR2 builds an R2 client from the given config. Network calls happen
// lazily on the first Put.
func NewR2(ctx context.Context, cfg *R2Config) (*R2, error) {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("auto"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("r2: aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &R2{cfg: cfg, client: client}, nil
}

// Put uploads content under key and returns the public URL.
func (r *R2) Put(ctx context.Context, key string, content io.Reader, size int64, mimeType string) (*Object, error) {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(r.cfg.Bucket),
		Key:           aws.String(key),
		Body:          content,
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(size),
	}
	if _, err := r.client.PutObject(ctx, input); err != nil {
		return nil, fmt.Errorf("r2: put %s: %w", key, err)
	}
	return &Object{
		Key:      key,
		URL:      strings.TrimRight(r.cfg.PublicBaseURL, "/") + "/" + strings.TrimLeft(key, "/"),
		Size:     size,
		MIMEType: mimeType,
	}, nil
}
