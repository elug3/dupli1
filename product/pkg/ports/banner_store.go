package ports

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// BannerStore keeps storefront banners. Get, Update and Delete answer
// ErrNotFound for an unknown id.
type BannerStore interface {
	ListBanners(ctx context.Context) ([]domain.Banner, error)
	GetBanner(ctx context.Context, id string) (*domain.Banner, error)
	CreateBanner(ctx context.Context, b domain.Banner) error
	UpdateBanner(ctx context.Context, b domain.Banner) error
	DeleteBanner(ctx context.Context, id string) error
}
