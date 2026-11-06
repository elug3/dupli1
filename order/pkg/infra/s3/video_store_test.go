package s3

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/ports"
)

// Runs against a real MinIO (or any S3 API) when ORDER_S3_TEST_ENDPOINT is
// set, e.g.:
//
//	docker run -d -p 9000:9000 cgr.dev/chainguard/minio server /data
//	ORDER_S3_TEST_ENDPOINT=http://localhost:9000 go test ./pkg/infra/s3/
//
// Credentials default to MinIO's minioadmin/minioadmin.
func TestVideoStoreRoundTrip(t *testing.T) {
	endpoint := os.Getenv("ORDER_S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("ORDER_S3_TEST_ENDPOINT not set")
	}
	access, secret := os.Getenv("ORDER_S3_TEST_ACCESS_KEY"), os.Getenv("ORDER_S3_TEST_SECRET_KEY")
	if access == "" {
		access, secret = "minioadmin", "minioadmin"
	}
	store, err := NewVideoStore(endpoint, access, secret, "order-video-store-test")
	if err != nil {
		t.Fatalf("NewVideoStore: %v", err)
	}
	ctx := t.Context()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket twice: %v", err)
	}

	const key = "orders/ord-1/inspection-1.mp4"
	body := "0123456789"
	if err := store.Put(ctx, key, strings.NewReader(body), int64(len(body)), "video/mp4"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	obj, err := store.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if obj.Size() != int64(len(body)) || obj.ModTime().IsZero() {
		t.Fatalf("size = %d, modTime = %v", obj.Size(), obj.ModTime())
	}
	if _, err := obj.Seek(4, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	rest, err := io.ReadAll(obj)
	obj.Close()
	if err != nil || string(rest) != "456789" {
		t.Fatalf("read after seek = %q, %v", rest, err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Open(ctx, key); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open after delete: error = %v, want ErrNotFound", err)
	}
}
