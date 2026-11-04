package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/jackc/pgx/v4"
)

// codeByName finds the master code a display name (or code) refers to in
// one of the brands / colors / sizes tables: an exact code match first, then
// a case-insensitive name match. "" when nothing matches. The domain's
// *CodeFromName helpers only know the seed lists, so a color or brand staff
// added at runtime (Ivory → IVR) would otherwise resolve to a made-up code.
func (s *ProductSearchStore) codeByName(ctx context.Context, table, name string) (string, error) {
	switch table {
	case "brands", "colors", "sizes":
	default:
		return "", fmt.Errorf("codeByName: unsupported master table %q", table)
	}
	var code string
	err := s.pool.QueryRow(ctx,
		`SELECT code FROM `+table+`
		 WHERE code = $1 OR lower(name) = lower($2)
		 ORDER BY (code = $1) DESC, code
		 LIMIT 1`,
		domain.NormalizeCode(name), strings.TrimSpace(name),
	).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", wrapDB("resolve "+table+" code", err)
	}
	return code, nil
}

// resolveVariantCodesByName fills a blank colorCode / sizeCode from the
// variant's display color / size through the master tables. Codes still
// blank afterwards fall through to domain.ResolveVariantCodes.
func (s *ProductSearchStore) resolveVariantCodesByName(ctx context.Context, v *domain.Variant) error {
	if v.ColorCode == "" && strings.TrimSpace(v.Color) != "" {
		code, err := s.codeByName(ctx, "colors", v.Color)
		if err != nil {
			return err
		}
		v.ColorCode = code
	}
	if v.SizeCode == "" && strings.TrimSpace(v.Size) != "" {
		code, err := s.codeByName(ctx, "sizes", v.Size)
		if err != nil {
			return err
		}
		v.SizeCode = code
	}
	return nil
}

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
func (s *ProductSearchStore) enrichMasterNames(ctx context.Context, products []domain.Product) {
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
