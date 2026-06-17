package memory

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/schick/pkg/product/domain"
)

// ProductStore is an in-memory implementation of ports.ProductStore, for use in tests.
type ProductStore struct {
	Bags     []domain.Bag
	Products []domain.Product
}

func NewProductStore() *ProductStore {
	return &ProductStore{}
}

func (s *ProductStore) SearchBags(_ map[string]string) ([]domain.Bag, error) {
	return s.Bags, nil
}

func (s *ProductStore) ListProducts() ([]domain.Product, error) {
	return s.Products, nil
}

func (s *ProductStore) GetProduct(id string) (*domain.Product, error) {
	for _, p := range s.Products {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("product not found: %s", id)
}

func brandPrefix(brand string) string {
	fields := strings.Fields(strings.TrimSpace(brand))
	word := "PRD"
	if len(fields) > 0 {
		word = fields[0]
	}
	runes := []rune(strings.ToUpper(word))
	if len(runes) > 3 {
		runes = runes[:3]
	}
	for len(runes) < 3 {
		runes = append(runes, 'X')
	}
	return string(runes)
}

func (s *ProductStore) nextProductID(brand string) string {
	prefix := brandPrefix(brand)
	max := 0
	for _, p := range s.Products {
		if strings.HasPrefix(p.ID, prefix+"-") {
			if n, err := strconv.Atoi(p.ID[len(prefix)+1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, max+1)
}

func (s *ProductStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	if p.ID == "" {
		p.ID = s.nextProductID(p.Brand)
	}
	s.Products = append(s.Products, p)
	return &p, nil
}

func (s *ProductStore) UpdateProduct(p domain.Product) (*domain.Product, error) {
	for i, existing := range s.Products {
		if existing.ID == p.ID {
			s.Products[i] = p
			return &p, nil
		}
	}
	return nil, fmt.Errorf("product not found: %s", p.ID)
}

func (s *ProductStore) DeleteProduct(id string) error {
	for i, p := range s.Products {
		if p.ID == id {
			s.Products = append(s.Products[:i], s.Products[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("product not found: %s", id)
}
