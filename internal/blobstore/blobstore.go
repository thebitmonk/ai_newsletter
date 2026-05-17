// Package blobstore is the BLOB persistence interface used by ImageGenerator
// and (later) other binary-producing pipelines.
//
// At v1 the only implementation is Cloudflare R2 via the S3-compatible API,
// chosen for zero-egress pricing per ADR-0015.
package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Object describes a stored blob.
type Object struct {
	Key       string // the path in the bucket
	URL       string // the publicly fetchable URL
	Size      int64
	MIMEType  string
}

// BlobStore puts a blob and returns its public URL. Implementations decide
// the URL scheme (R2 returns the public bucket URL + key).
type BlobStore interface {
	Put(ctx context.Context, key string, content io.Reader, size int64, mimeType string) (*Object, error)
}

// ErrConfig is returned when required env vars are missing.
var ErrConfig = errors.New("blobstore: missing required configuration")

// MissingEnvError describes which env var is unset.
type MissingEnvError struct {
	Name string
}

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("%s: %s is required", ErrConfig.Error(), e.Name)
}
func (e *MissingEnvError) Unwrap() error { return ErrConfig }
