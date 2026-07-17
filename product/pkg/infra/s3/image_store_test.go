package s3_test

import (
	"strings"
	"testing"

	s3store "github.com/elug3/dupli1/product/pkg/infra/s3"
)

func TestPublicURLOmitsBucketName(t *testing.T) {
	t.Parallel()

	store, err := s3store.NewImageStore(
		"https://s3.us-east-1.amazonaws.com",
		"https://images.dupli1.com",
		"ak", "sk",
		"dupli1-production-product-images-abc123",
	)
	if err != nil {
		t.Fatalf("NewImageStore: %v", err)
	}

	got := store.PublicURL("BOT-001/sku/uuid.jpg")
	want := "https://images.dupli1.com/BOT-001/sku/uuid.jpg"
	if got != want {
		t.Fatalf("PublicURL = %q, want %q", got, want)
	}
	if strings.Contains(got, "dupli1-production-product-images") {
		t.Fatalf("URL must not include bucket name: %s", got)
	}
}

func TestPublicURLLocalMinIOViaGateway(t *testing.T) {
	t.Parallel()

	store, err := s3store.NewImageStore(
		"http://minio:9000",
		"http://localhost:8080/product-images",
		"ak", "sk",
		"product-images",
	)
	if err != nil {
		t.Fatalf("NewImageStore: %v", err)
	}

	got := store.PublicURL("BOT-001/sku/uuid.jpg")
	want := "http://localhost:8080/product-images/BOT-001/sku/uuid.jpg"
	if got != want {
		t.Fatalf("PublicURL = %q, want %q", got, want)
	}
}

func TestPublicURLTrimsTrailingSlashOnBase(t *testing.T) {
	t.Parallel()

	store, err := s3store.NewImageStore(
		"https://s3.us-east-1.amazonaws.com",
		"https://images.dupli1.com/",
		"ak", "sk",
		"bucket",
	)
	if err != nil {
		t.Fatal(err)
	}
	got := store.PublicURL("/a/b")
	if got != "https://images.dupli1.com/a/b" {
		t.Fatalf("got %q", got)
	}
}
