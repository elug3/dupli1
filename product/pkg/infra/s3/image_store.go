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

// NewImageStore creates an S3/MinIO-backed image store.
//
// endpoint is used by this service to upload (e.g. "http://minio:9000" or
// "https://s3.us-east-1.amazonaws.com").
//
// publicEndpoint is the browser-reachable base URL embedded in imageUrls.
// It must already include any path prefix required before the object key
// (local Compose: "http://localhost:8080/product-images"; AWS CloudFront:
// "https://images.dupli1.com"). When empty it defaults to endpoint.
//
// Upload returns "{publicEndpoint}/{objectKey}" — it does not insert the
// bucket name (bucket is an API/credential concern, not part of the CDN path).
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
	return s.PublicURL(objectKey), nil
}

// PublicURL builds the browser-facing object URL from S3_PUBLIC_ENDPOINT + key.
// The bucket name is never inserted into the path (CDN / nginx prefix owns that).
func (s *ImageStore) PublicURL(objectKey string) string {
	key := strings.TrimLeft(objectKey, "/")
	return fmt.Sprintf("%s/%s", s.publicBase, key)
}
