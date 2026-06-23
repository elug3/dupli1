package ports

import "github.com/schick/pkg/product/domain"

type ProductStore interface {
	SearchConsultations(filter map[string]string) ([]domain.Consultation, error)
	SearchShoes(filter map[string]string) ([]domain.Shoes, error)
	SearchOuterwear(filter map[string]string) ([]domain.Outerwear, error)
	SearchBottoms(filter map[string]string) ([]domain.Bottoms, error)
	SearchBags(filter map[string]string) ([]domain.Bag, error)
	SearchClocks(filter map[string]string) ([]domain.Clock, error)
}
