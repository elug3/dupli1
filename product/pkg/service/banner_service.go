package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// BannerImageMaxBytes caps one uploaded banner image.
const BannerImageMaxBytes = 10 << 20

// ErrBannerImagesUnavailable is an upload with no image store configured.
var ErrBannerImagesUnavailable = errors.New("banner image storage is not configured")

var bannerImageExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// BannerService manages the storefront's image banners. Staff put one up and
// take it down by hand (Active); there is no schedule.
type BannerService struct {
	store  ports.BannerStore
	images ports.ImageStore
	now    func() time.Time
}

func NewBannerService(store ports.BannerStore, images ports.ImageStore) *BannerService {
	return &BannerService{store: store, images: images, now: time.Now}
}

// List returns every banner, shown or not, for the console.
func (s *BannerService) List(ctx context.Context) ([]domain.Banner, error) {
	banners, err := s.store.ListBanners(ctx)
	if err != nil {
		return nil, err
	}
	sortBanners(banners)
	return banners, nil
}

// Live returns the banners shown in a placement, in display order.
func (s *BannerService) Live(ctx context.Context, placement domain.BannerPlacement) ([]domain.Banner, error) {
	if placement == "" {
		placement = domain.BannerPlacementHomeHero
	}
	if !placement.Valid() {
		return nil, ports.Invalid(fmt.Sprintf("placement %q is not one of home_hero", placement))
	}
	all, err := s.store.ListBanners(ctx)
	if err != nil {
		return nil, err
	}
	out := []domain.Banner{}
	for _, b := range all {
		if b.Active && b.Placement == placement {
			out = append(out, b)
		}
	}
	sortBanners(out)
	return out, nil
}

// sortBanners orders by placement, then sort_order, then the most recently
// changed first, so a banner just switched on wins a tie.
func sortBanners(banners []domain.Banner) {
	sort.SliceStable(banners, func(i, j int) bool {
		a, b := banners[i], banners[j]
		if a.Placement != b.Placement {
			return a.Placement < b.Placement
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
}

func (s *BannerService) Create(ctx context.Context, b domain.Banner, actor string) (*domain.Banner, error) {
	b.Normalize()
	if err := b.Validate(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	now := s.now().UTC()
	b.ID = ulid.Make().String()
	b.CreatedAt, b.UpdatedAt, b.UpdatedBy = now, now, actor
	if err := s.store.CreateBanner(ctx, b); err != nil {
		return nil, err
	}
	return &b, nil
}

// Update replaces a banner's fields; id and created_at stay.
func (s *BannerService) Update(ctx context.Context, id string, b domain.Banner, actor string) (*domain.Banner, error) {
	current, err := s.store.GetBanner(ctx, id)
	if err != nil {
		return nil, err
	}
	b.Normalize()
	if err := b.Validate(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	b.ID, b.CreatedAt = current.ID, current.CreatedAt
	b.UpdatedAt, b.UpdatedBy = s.now().UTC(), actor
	if err := s.store.UpdateBanner(ctx, b); err != nil {
		return nil, err
	}
	return &b, nil
}

// SetActive shows or takes down a banner without touching anything else.
func (s *BannerService) SetActive(ctx context.Context, id string, active bool, actor string) (*domain.Banner, error) {
	b, err := s.store.GetBanner(ctx, id)
	if err != nil {
		return nil, err
	}
	b.Active = active
	b.UpdatedAt, b.UpdatedBy = s.now().UTC(), actor
	if err := s.store.UpdateBanner(ctx, *b); err != nil {
		return nil, err
	}
	return b, nil
}

// Delete removes a banner. Its image stays in storage, like a replaced
// product image.
func (s *BannerService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteBanner(ctx, id)
}

// UploadImage stores one image as it was sent (no re-encoding: a hero is
// shown large) and returns its public URL for a banner to use.
func (s *BannerService) UploadImage(ctx context.Context, data []byte) (string, error) {
	if s.images == nil {
		return "", ErrBannerImagesUnavailable
	}
	if len(data) == 0 || len(data) > BannerImageMaxBytes {
		return "", ports.Invalid("a banner image is at most 10 MB")
	}
	// Trust the bytes, not the header: a renamed file is caught here.
	sniffed := strings.ToLower(strings.Split(http.DetectContentType(data), ";")[0])
	ext, ok := bannerImageExt[sniffed]
	if !ok {
		return "", ports.Invalid("use a JPEG, PNG or WebP image")
	}
	key := "banners/" + ulid.Make().String() + ext
	return s.images.Upload(ctx, key, bytes.NewReader(data), int64(len(data)), sniffed)
}
