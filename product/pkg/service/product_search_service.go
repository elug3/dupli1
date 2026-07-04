package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

const (
	productCreatedSubject       = "product.created"
	productUpdatedSubject       = "product.updated"
	productDeletedSubject       = "product.deleted"
	productImageUploadedSubject = "product.image_uploaded"
)

type productEvent struct {
	EventType string    `json:"event_type"`
	ProductID string    `json:"product_id"`
	Name      string    `json:"name"`
	Brand     string    `json:"brand"`
	Category  string    `json:"category"`
	Status    string    `json:"status"`
	Price     float64   `json:"price"`
	ImageURL  string    `json:"image_url,omitempty"`
	Occurred  time.Time `json:"occurred_at"`
}

type ProductSearchService struct {
	store          ports.ProductStore
	imageStore     ports.ImageStore
	eventPublisher ports.EventPublisher
	now            func() time.Time
}

func NewProductSearchService(store ports.ProductStore, imageStore ports.ImageStore, eventPublisher ...ports.EventPublisher) *ProductSearchService {
	s := &ProductSearchService{
		store:      store,
		imageStore: imageStore,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	if len(eventPublisher) > 0 {
		s.eventPublisher = eventPublisher[0]
	}
	return s
}

// SearchProducts filters the catalog. When public is true, only active products
// are returned and cost is redacted.
func (s *ProductSearchService) SearchProducts(filter map[string]string, public bool) ([]domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	f := make(map[string]string, len(filter)+1)
	for k, v := range filter {
		f[k] = v
	}
	if public {
		f["status"] = "active"
	}
	results, err := s.store.SearchProducts(f)
	if err != nil {
		return nil, err
	}
	if public {
		for i := range results {
			results[i].Cost = 0
		}
	}
	return results, nil
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

func (s *ProductSearchService) GetPublicProduct(id string) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.GetActiveProduct(id)
}

func (s *ProductSearchService) CreateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	created, err := s.store.CreateProduct(p)
	if err != nil {
		return nil, err
	}
	if err := s.publish(context.Background(), productCreatedSubject, created, ""); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ProductSearchService) UpdateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	updated, err := s.store.UpdateProduct(p)
	if err != nil {
		return nil, err
	}
	if err := s.publish(context.Background(), productUpdatedSubject, updated, ""); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *ProductSearchService) DeleteProduct(id string) error {
	if s.store == nil {
		return fmt.Errorf("store not initialized")
	}
	existing, err := s.store.GetProduct(id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProduct(id); err != nil {
		return err
	}
	return s.publish(context.Background(), productDeletedSubject, existing, "")
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
	updated, err := s.store.UpdateProduct(*p)
	if err != nil {
		return nil, err
	}
	if err := s.publish(ctx, productImageUploadedSubject, updated, url); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *ProductSearchService) publish(ctx context.Context, subject string, product *domain.Product, imageURL string) error {
	if s.eventPublisher == nil || product == nil {
		return nil
	}
	return s.eventPublisher.Publish(ctx, subject, productEvent{
		EventType: subject,
		ProductID: product.ID,
		Name:      product.Name,
		Brand:     product.Brand,
		Category:  product.Category,
		Status:    product.Status,
		Price:     product.Price,
		ImageURL:  imageURL,
		Occurred:  s.now(),
	})
}
