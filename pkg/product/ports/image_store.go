package ports

import (
	"context"
	"io"
)

type ImageStore interface {
	// Upload stores r at objectKey and returns the public URL.
	Upload(ctx context.Context, objectKey string, r io.Reader, size int64, contentType string) (string, error)
}
