package pg

import (
	"testing"
)

// Regression for listing_image_urls column added for upload-time JPEG thumbs.
func TestVariantListingImageURLsRoundTrip(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "variant_listing_images_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stock, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	if err := store.SeedMockEcoBag(ctx, stock); err != nil {
		t.Fatalf("seed mock eco-bag: %v", err)
	}

	const sku = "DUP_ECO01_BLK_OS"
	v, err := store.GetVariant(ctx, sku)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}

	v.ImageURLs = []string{"https://cdn.test/products/full.jpg"}
	v.ListingImageURLs = []string{"https://cdn.test/products/full.w600.jpg"}
	updated, err := store.UpdateVariant(ctx, *v)
	if err != nil {
		t.Fatalf("update variant: %v", err)
	}
	if len(updated.ListingImageURLs) != 1 || updated.ListingImageURLs[0] != v.ListingImageURLs[0] {
		t.Fatalf("update returned listing urls = %v", updated.ListingImageURLs)
	}

	got, err := store.GetVariant(ctx, sku)
	if err != nil {
		t.Fatalf("reload variant: %v", err)
	}
	if len(got.ImageURLs) != 1 || got.ImageURLs[0] != v.ImageURLs[0] {
		t.Fatalf("image_urls = %v, want %v", got.ImageURLs, v.ImageURLs)
	}
	if len(got.ListingImageURLs) != 1 || got.ListingImageURLs[0] != v.ListingImageURLs[0] {
		t.Fatalf("listing_image_urls = %v, want %v", got.ListingImageURLs, v.ListingImageURLs)
	}
}
