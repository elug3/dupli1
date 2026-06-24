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

// NewImageStore creates a MinIO-backed image store.
func NewImageStore(endpoint, accessKey, secretKey, bucket string) (*ImageStore, error) {
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
	return &ImageStore{
		client:     client,
		bucket:     bucket,
		publicBase: strings.TrimRight(endpoint, "/"),
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
