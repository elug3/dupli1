package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type ImageStore struct {
	client     *minio.Client
	bucket     string
	publicBase string
}

// NewImageStore creates a MinIO-backed image store. endpoint is used to reach
// MinIO from this service (e.g. the Docker-internal "http://minio:9000").
// publicEndpoint is embedded in URLs handed back to clients (e.g. browsers),
// which may not be able to resolve the internal endpoint's host; it defaults
// to endpoint when empty, preserving the old behavior for setups where the
// two coincide (real S3, or endpoint already being publicly reachable).
func NewImageStore(endpoint, publicEndpoint, accessKey, secretKey, bucket string) (*ImageStore, error) {
	useSSL := strings.HasPrefix(endpoint, "https://")
	host := strings.TrimPrefix(endpoint, "https://")
	host = strings.TrimPrefix(host, "http://")

	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	if publicEndpoint == "" {
		publicEndpoint = endpoint
	}
	return &ImageStore{
		client:     client,
		bucket:     bucket,
		publicBase: strings.TrimRight(publicEndpoint, "/"),
	}, nil
}

// Upload stores r at objectKey and returns the public URL.
func (s *ImageStore) Upload(ctx context.Context, objectKey string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}
	return fmt.Sprintf("%s/%s/%s", s.publicBase, s.bucket, objectKey), nil
}
