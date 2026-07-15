package domain

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
// when they are blank. Style defaults from the product id (BOT-001 → S001).
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
	if p.StyleCode == "" {
		p.StyleCode = StyleCodeFromProductID(p.ID)
	} else {
		p.StyleCode = NormalizeCode(p.StyleCode)
	}
}
