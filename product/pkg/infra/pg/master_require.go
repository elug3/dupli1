package pg

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
)

func (s *ProductSearchStore) requireBrand(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: brand %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) requireStyle(ctx context.Context, brandCode, styleCode string) error {
	brandCode = domain.NormalizeCode(brandCode)
	styleCode = domain.NormalizeCode(styleCode)
	var ok bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM styles WHERE brand_code = $1 AND code = $2)`,
		brandCode, styleCode,
	).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: style %s/%s", domain.ErrMasterNotFound, brandCode, styleCode)
	}
	return nil
}

func (s *ProductSearchStore) requireColor(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM colors WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: color %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) requireSize(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sizes WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: size %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) requireEdition(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	if code == "" {
		return nil
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sku_editions WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: edition %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) requireSubcategory(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	if code == "" {
		return nil
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subcategories WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: subcategory %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) requireOccasion(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	if code == "" {
		return nil
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occasions WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: occasion %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) requireTarget(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	if code == "" {
		return nil
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM targets WHERE code = $1)`, code).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: target %s", domain.ErrMasterNotFound, code)
	}
	return nil
}

func (s *ProductSearchStore) brandName(ctx context.Context, code string) string {
	var name string
	_ = s.pool.QueryRow(ctx, `SELECT name FROM brands WHERE code = $1`, code).Scan(&name)
	return name
}

func (s *ProductSearchStore) colorName(ctx context.Context, code string) string {
	var name string
	_ = s.pool.QueryRow(ctx, `SELECT name FROM colors WHERE code = $1`, code).Scan(&name)
	return name
}

func (s *ProductSearchStore) sizeName(ctx context.Context, code string) string {
	var name string
	_ = s.pool.QueryRow(ctx, `SELECT name FROM sizes WHERE code = $1`, code).Scan(&name)
	return name
}

// enrichMasterNames fills display Brand/Color/Size from master tables when blank.
func (s *ProductSearchStore) enrichMasterNames(products []domain.Product) {
	ctx := context.Background()
	for i := range products {
		p := &products[i]
		if p.Brand == "" && p.BrandCode != "" {
			if name := s.brandName(ctx, p.BrandCode); name != "" {
				p.Brand = name
			}
		}
		for j := range p.Variants {
			v := &p.Variants[j]
			if v.Color == "" && v.ColorCode != "" {
				if name := s.colorName(ctx, v.ColorCode); name != "" {
					v.Color = name
				}
			}
			if v.Size == "" && v.SizeCode != "" {
				if name := s.sizeName(ctx, v.SizeCode); name != "" {
					v.Size = name
				}
			}
		}
	}
}
