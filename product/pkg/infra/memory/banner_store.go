package memory

import (
	"context"
	"sync"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// BannerStore is an in-memory ports.BannerStore for tests.
type BannerStore struct {
	mu      sync.Mutex
	banners map[string]domain.Banner
}

func NewBannerStore() *BannerStore {
	return &BannerStore{banners: map[string]domain.Banner{}}
}

func (s *BannerStore) ListBanners(context.Context) ([]domain.Banner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Banner, 0, len(s.banners))
	for _, b := range s.banners {
		out = append(out, b)
	}
	return out, nil
}

func (s *BannerStore) GetBanner(_ context.Context, id string) (*domain.Banner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.banners[id]
	if !ok {
		return nil, ports.NotFound("banner " + id)
	}
	return &b, nil
}

func (s *BannerStore) CreateBanner(_ context.Context, b domain.Banner) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.banners[b.ID]; ok {
		return ports.ErrConflict
	}
	s.banners[b.ID] = b
	return nil
}

func (s *BannerStore) UpdateBanner(_ context.Context, b domain.Banner) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.banners[b.ID]; !ok {
		return ports.NotFound("banner " + b.ID)
	}
	s.banners[b.ID] = b
	return nil
}

func (s *BannerStore) DeleteBanner(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.banners[id]; !ok {
		return ports.NotFound("banner " + id)
	}
	delete(s.banners, id)
	return nil
}
