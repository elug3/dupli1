package service

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/google/uuid"
)

const (
	productCreatedSubject       = "product.created"
	productUpdatedSubject       = "product.updated"
	productDeletedSubject       = "product.deleted"
	productImageUploadedSubject = "product.image_uploaded"
	variantCreatedSubject       = "product.variant_created"
	variantUpdatedSubject       = "product.variant_updated"
	variantDeletedSubject       = "product.variant_deleted"
)

type productEvent struct {
	EventType string    `json:"event_type"`
	ProductID string    `json:"product_id"`
	SKU       string    `json:"sku,omitempty"`
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

// SearchProducts returns parent styles only (no duplicate colors).
// When public is true, only active parents are returned.
// Optional filter keys "limit" and "offset" paginate results; total is the
// full match count before pagination.
func (s *ProductSearchService) SearchProducts(filter map[string]string, public bool) ([]domain.Product, int, error) {
	if s.store == nil {
		return nil, 0, fmt.Errorf("store not initialized")
	}
	f := make(map[string]string, len(filter)+1)
	for k, v := range filter {
		f[k] = v
	}
	if public {
		f["status"] = "active"
	}
	return s.store.SearchProducts(f)
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

const (
	recommendDefaultLimit = 8
	recommendMaxLimit     = 24
	recommendCandidateCap = 200
)

// Recommend returns related active parent products for a public PDP seed.
func (s *ProductSearchService) Recommend(seedID string, limit int) ([]domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	if limit <= 0 {
		limit = recommendDefaultLimit
	}
	if limit > recommendMaxLimit {
		limit = recommendMaxLimit
	}
	seed, err := s.store.GetActiveProduct(seedID)
	if err != nil {
		return nil, err
	}
	filter := map[string]string{
		"status": "active",
		"limit":  strconv.Itoa(recommendCandidateCap),
	}
	if seed.Category != "" {
		filter["category"] = seed.Category
	}
	candidates, _, err := s.store.SearchProducts(filter)
	if err != nil {
		return nil, err
	}
	// Seed from GetActiveProduct includes variants; strip for fair scoring vs list cards.
	seedCard := *seed
	seedCard.Variants = nil
	return domain.RankRecommendations(seedCard, candidates, limit), nil
}

func (s *ProductSearchService) GetPublicVariant(sku string) (*domain.Variant, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	v, err := s.store.GetVariant(sku)
	if err != nil {
		return nil, err
	}
	return s.checkPublicVariant(v)
}

func (s *ProductSearchService) GetPublicVariantBySkuID(skuID string) (*domain.Variant, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	v, err := s.store.GetVariantBySkuID(skuID)
	if err != nil {
		return nil, err
	}
	return s.checkPublicVariant(v)
}

func (s *ProductSearchService) checkPublicVariant(v *domain.Variant) (*domain.Variant, error) {
	if v.Status != "active" {
		return nil, fmt.Errorf("variant: %w", ports.ErrNotFound)
	}
	if _, err := s.store.GetActiveProduct(v.ProductID); err != nil {
		return nil, fmt.Errorf("variant: %w", ports.ErrNotFound)
	}
	return v, nil
}

func (s *ProductSearchService) CreateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	created, err := s.store.CreateProduct(p)
	if err != nil {
		return nil, err
	}
	if err := s.publish(context.Background(), productCreatedSubject, created, "", ""); err != nil {
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
	if err := s.publish(context.Background(), productUpdatedSubject, updated, "", ""); err != nil {
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
	return s.publish(context.Background(), productDeletedSubject, existing, "", "")
}

func (s *ProductSearchService) CreateVariant(productID string, v domain.Variant) (*domain.Variant, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	if _, err := s.store.GetProduct(productID); err != nil {
		return nil, err
	}
	v.ProductID = productID
	created, err := s.store.CreateVariant(v)
	if err != nil {
		return nil, err
	}
	parent, _ := s.store.GetProduct(productID)
	_ = s.publish(context.Background(), variantCreatedSubject, parent, created.SKU, "")
	return created, nil
}

// UpdateVariant merges the incoming (possibly partial) body onto the
// existing variant rather than overwriting it outright, so an update that
// only sets e.g. price can't silently blank color/size/status/images.
func (s *ProductSearchService) UpdateVariant(productID, sku string, v domain.Variant) (*domain.Variant, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	existing, err := s.store.GetVariant(sku)
	if err != nil {
		return nil, err
	}
	if existing.ProductID != productID {
		return nil, fmt.Errorf("variant %s: %w", sku, ports.ErrNotFound)
	}
	merged := existing.MergeUpdate(v)
	merged.SKU = sku
	merged.ProductID = productID
	updated, err := s.store.UpdateVariant(merged)
	if err != nil {
		return nil, err
	}
	parent, _ := s.store.GetProduct(productID)
	_ = s.publish(context.Background(), variantUpdatedSubject, parent, updated.SKU, "")
	return updated, nil
}

func (s *ProductSearchService) DeleteVariant(productID, sku string) error {
	if s.store == nil {
		return fmt.Errorf("store not initialized")
	}
	existing, err := s.store.GetVariant(sku)
	if err != nil {
		return err
	}
	if existing.ProductID != productID {
		return fmt.Errorf("variant %s: %w", sku, ports.ErrNotFound)
	}
	parent, _ := s.store.GetProduct(productID)
	if err := s.store.DeleteVariant(sku); err != nil {
		return err
	}
	return s.publish(context.Background(), variantDeletedSubject, parent, sku, "")
}

// UploadImage appends an image to the default variant (sku == productID, else first variant).
func (s *ProductSearchService) UploadImage(ctx context.Context, productID string, r io.Reader, size int64, contentType string) (*domain.Product, error) {
	p, err := s.store.GetProduct(productID)
	if err != nil {
		return nil, err
	}
	sku, err := defaultVariantSKU(p)
	if err != nil {
		return nil, err
	}
	if _, err := s.UploadVariantImage(ctx, productID, sku, r, size, contentType); err != nil {
		return nil, err
	}
	return s.store.GetProduct(productID)
}

// UploadVariantImage uploads a file and appends its URL to the variant's ImageURLs.
func (s *ProductSearchService) UploadVariantImage(ctx context.Context, productID, sku string, r io.Reader, size int64, contentType string) (*domain.Variant, error) {
	if s.imageStore == nil {
		return nil, fmt.Errorf("image store not configured")
	}
	v, err := s.store.GetVariant(sku)
	if err != nil {
		return nil, err
	}
	if v.ProductID != productID {
		return nil, fmt.Errorf("variant %s: %w", sku, ports.ErrNotFound)
	}
	objectKey := productID + "/" + sku + "/" + uuid.New().String()
	url, err := s.imageStore.Upload(ctx, objectKey, r, size, contentType)
	if err != nil {
		return nil, err
	}
	v.ImageURLs = append(v.ImageURLs, url)
	updated, err := s.store.UpdateVariant(*v)
	if err != nil {
		return nil, err
	}
	parent, _ := s.store.GetProduct(productID)
	if err := s.publish(ctx, productImageUploadedSubject, parent, sku, url); err != nil {
		return nil, err
	}
	return updated, nil
}

func defaultVariantSKU(p *domain.Product) (string, error) {
	if p == nil || len(p.Variants) == 0 {
		return "", ports.Invalid("product has no variants")
	}
	for _, v := range p.Variants {
		if v.SKU == p.ID {
			return v.SKU, nil
		}
	}
	return p.Variants[0].SKU, nil
}

func (s *ProductSearchService) publish(ctx context.Context, subject string, product *domain.Product, sku, imageURL string) error {
	if s.eventPublisher == nil || product == nil {
		return nil
	}
	return s.eventPublisher.Publish(ctx, subject, productEvent{
		EventType: subject,
		ProductID: product.ID,
		SKU:       sku,
		Name:      product.Name,
		Brand:     product.Brand,
		Category:  product.Category,
		Status:    product.Status,
		Price:     product.PriceFrom,
		ImageURL:  imageURL,
		Occurred:  s.now(),
	})
}
