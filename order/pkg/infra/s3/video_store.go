// Package s3 stores inspection videos in MinIO or any S3-compatible object
// storage (SeaweedFS on VENUS speaks the same API).
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/elug3/dupli1/order/pkg/ports"
)

// VideoStore is a private bucket of inspection videos. Objects are never
// given a public URL: order streams them to the caller after its own
// ownership check.
type VideoStore struct {
	client *minio.Client
	bucket string
}

var _ ports.InspectionVideoStore = (*VideoStore)(nil)

// NewVideoStore connects to endpoint ("http://minio:9000",
// "https://s3.example.com"); the scheme decides TLS.
func NewVideoStore(endpoint, accessKey, secretKey, bucket string) (*VideoStore, error) {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)
	if endpoint == "" || bucket == "" {
		return nil, errors.New("inspection video store: endpoint and bucket are required")
	}
	useSSL := strings.HasPrefix(endpoint, "https://")
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	host = strings.TrimRight(host, "/")
	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &VideoStore{client: client, bucket: bucket}, nil
}

// EnsureBucket creates the bucket when it does not exist yet. The bucket is
// left private (no anonymous policy).
func (s *VideoStore) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket %s: %w", s.bucket, err)
	}
	return nil
}

func (s *VideoStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("%w: upload: %v", ports.ErrInspectionVideoStorageUnavailable, err)
	}
	return nil
}

func (s *VideoStore) Open(ctx context.Context, key string) (ports.InspectionVideoObject, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: open: %v", ports.ErrInspectionVideoStorageUnavailable, err)
	}
	// GetObject is lazy; Stat makes the request and reports a missing object.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, fmt.Errorf("inspection video object %s: %w", key, ports.ErrNotFound)
		}
		return nil, fmt.Errorf("%w: stat: %v", ports.ErrInspectionVideoStorageUnavailable, err)
	}
	return &object{Object: obj, size: info.Size, modTime: info.LastModified}, nil
}

func (s *VideoStore) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("%w: delete: %v", ports.ErrInspectionVideoStorageUnavailable, err)
	}
	return nil
}

type object struct {
	*minio.Object
	size    int64
	modTime time.Time
}

func (o *object) Size() int64        { return o.size }
func (o *object) ModTime() time.Time { return o.modTime }
