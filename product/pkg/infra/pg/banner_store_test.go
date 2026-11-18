package pg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

func TestBannerStoreRoundTrip(t *testing.T) {
	pool := freshInventorySchema(t, requireProductDSN(t), "banner_store_test")
	banners, err := NewBannerStore(pool)
	if err != nil {
		t.Fatalf("NewBannerStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	b := domain.Banner{
		ID: "ban-1", Placement: domain.BannerPlacementHomeHero, ImageURL: "/a.jpg", MobileImageURL: "/a-m.jpg",
		Title: domain.LocalizedText{Ko: "겨울", En: "Winter"}, LinkURL: "/category/new",
		LinkLabel: domain.LocalizedText{Ko: "보기"}, SortOrder: 2, CreatedAt: now, UpdatedAt: now, UpdatedBy: "staff",
	}
	if err := banners.CreateBanner(ctx, b); err != nil {
		t.Fatalf("Create: %v", err)
	}
	b.Active = true
	b.Title.Zh = "冬季"
	if err := banners.UpdateBanner(ctx, b); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := banners.GetBanner(ctx, "ban-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Active || got.Title != b.Title || got.LinkLabel != b.LinkLabel || got.MobileImageURL != "/a-m.jpg" || got.SortOrder != 2 {
		t.Fatalf("got %+v", got)
	}
	list, err := banners.ListBanners(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v %v", list, err)
	}
	if err := banners.DeleteBanner(ctx, "ban-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := banners.GetBanner(ctx, "ban-1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
	if err := banners.UpdateBanner(ctx, b); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Update missing = %v", err)
	}
}
