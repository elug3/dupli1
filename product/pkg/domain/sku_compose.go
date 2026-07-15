package domain

import "fmt"

// ResolveVariantCodes fills ColorCode / SizeCode on v from display names or
// explicit codes when they are blank. Empty size defaults to OS (one size).
// EditionCode is normalized when already set; it stays optional.
func ResolveVariantCodes(v *Variant) {
	if v == nil {
		return
	}
	if v.ColorCode == "" && v.Color != "" {
		v.ColorCode = ColorCodeFromName(v.Color)
	} else if v.ColorCode != "" {
		v.ColorCode = NormalizeCode(v.ColorCode)
	}
	if v.SizeCode == "" {
		if v.Size != "" {
			v.SizeCode = SizeCodeFromName(v.Size)
		} else {
			v.SizeCode = "OS"
		}
	} else {
		v.SizeCode = NormalizeCode(v.SizeCode)
	}
	if v.EditionCode != "" {
		v.EditionCode = NormalizeCode(v.EditionCode)
	}
}

// ComposeVariantSKU builds a human SKU for a new variant.
// When the parent has brand_code + style_code and the variant has a color code,
// the luxury format Brand_Style_Color[_Edition]_Size is used.
// Otherwise the legacy {productId}-{color}-{size} helper is used.
func ComposeVariantSKU(productID, brandCode, styleCode string, v *Variant) string {
	if v == nil {
		return BuildVariantSKUBase(productID, "", "")
	}
	ResolveVariantCodes(v)
	brandCode = NormalizeCode(brandCode)
	styleCode = NormalizeCode(styleCode)
	if ValidBrandCode(brandCode) && ValidSegmentCode(styleCode) && ValidSegmentCode(v.ColorCode) {
		sku, err := BuildSKU(SKUParts{
			BrandCode:   brandCode,
			StyleCode:   styleCode,
			ColorCode:   v.ColorCode,
			EditionCode: NormalizeCode(v.EditionCode),
			SizeCode:    v.SizeCode,
		})
		if err == nil {
			return sku
		}
	}
	return BuildVariantSKUBase(productID, v.Color, v.Size)
}

// AssignProductCodes sets BrandCode / StyleCode on a new or backfilled product
// when they are blank. Style is derived from legacy ids only (BOT-001 → S001);
// ULID product ids require an explicit styleCode.
func AssignProductCodes(p *Product) {
	if p == nil {
		return
	}
	if p.BrandCode == "" {
		if p.Brand != "" {
			p.BrandCode = BrandCodeFromName(p.Brand)
		}
	} else {
		p.BrandCode = NormalizeCode(p.BrandCode)
	}
	if p.StyleCode != "" {
		p.StyleCode = NormalizeCode(p.StyleCode)
		return
	}
	if style := StyleCodeFromProductID(p.ID); style != "" {
		p.StyleCode = style
	}
}

// RequireProductSKUCodes reports whether brand and style codes are present for create.
func RequireProductSKUCodes(p *Product) error {
	if p == nil {
		return ErrMissingSKUCodes
	}
	AssignProductCodes(p)
	if !ValidBrandCode(p.BrandCode) || !ValidSegmentCode(p.StyleCode) {
		return fmt.Errorf("%w: brandCode and styleCode are required", ErrMissingSKUCodes)
	}
	return nil
}

// RequireVariantSKUCodes resolves and checks color/size codes for create.
func RequireVariantSKUCodes(v *Variant) error {
	if v == nil {
		return ErrMissingSKUCodes
	}
	ResolveVariantCodes(v)
	if !ValidSegmentCode(v.ColorCode) {
		return fmt.Errorf("%w: colorCode is required (set colorCode or color)", ErrMissingSKUCodes)
	}
	if !ValidSegmentCode(v.SizeCode) {
		return fmt.Errorf("%w: sizeCode is required", ErrMissingSKUCodes)
	}
	if v.EditionCode != "" && !ValidSegmentCode(v.EditionCode) {
		return fmt.Errorf("%w: invalid editionCode", ErrMissingSKUCodes)
	}
	return nil
}
