package service

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/ports"
)

type ProductSearchService struct {
	store      ports.ProductStore
	imageStore ports.ImageStore
}

func NewProductSearchService(store ports.ProductStore, imageStore ports.ImageStore) *ProductSearchService {
	return &ProductSearchService{store: store, imageStore: imageStore}
}

func (s *ProductSearchService) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchBags(filter)
}

func (s *ProductSearchService) ListProducts() ([]domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.ListProducts()
}

func (s *ProductSearchService) GetProduct(id string) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.GetProduct(id)
}

func (s *ProductSearchService) CreateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.CreateProduct(p)
}

func (s *ProductSearchService) UpdateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.UpdateProduct(p)
}

func (s *ProductSearchService) DeleteProduct(id string) error {
	if s.store == nil {
		return fmt.Errorf("store not initialized")
	}
	return s.store.DeleteProduct(id)
}

// UploadImage uploads a file to the image store and appends its URL to the product's ImageURLs.
func (s *ProductSearchService) UploadImage(ctx context.Context, productID string, r io.Reader, size int64, contentType string) (*domain.Product, error) {
	if s.imageStore == nil {
		return nil, fmt.Errorf("image store not configured")
	}
	p, err := s.store.GetProduct(productID)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}
	objectKey := productID + "/" + uuid.New().String()
	url, err := s.imageStore.Upload(ctx, objectKey, r, size, contentType)
	if err != nil {
		return nil, err
	}
	p.ImageURLs = append(p.ImageURLs, url)
	return s.store.UpdateProduct(*p)
}
