package domain

// MergeUpdate returns a copy of the variant with any non-zero-value fields
// from incoming applied on top. Used by UpdateVariant so a partial request
// body (e.g. price-only) can't silently blank out color/size/status/images —
// omitted fields keep their current value instead of being overwritten with
// the JSON zero value. Identity fields (SkuID, SKU, ProductID, CreatedAt)
// are never taken from incoming; callers set those from the lookup key.
func (existing Variant) MergeUpdate(incoming Variant) Variant {
	merged := existing
	if incoming.Color != "" {
		merged.Color = incoming.Color
	}
	if incoming.Size != "" {
		merged.Size = incoming.Size
	}
	if incoming.ColorCode != "" {
		merged.ColorCode = incoming.ColorCode
	}
	if incoming.EditionCode != "" {
		merged.EditionCode = incoming.EditionCode
	}
	if incoming.SizeCode != "" {
		merged.SizeCode = incoming.SizeCode
	}
	if incoming.SellingPrice != 0 {
		merged.SellingPrice = incoming.SellingPrice
	}
	if incoming.Price != 0 {
		merged.Price = incoming.Price
	}
	if incoming.Status != "" {
		merged.Status = incoming.Status
	}
	if len(incoming.ImageURLs) > 0 {
		merged.ImageURLs = incoming.ImageURLs
	}
	return merged
}

// EnrichFromVariants fills summary and legacy fields from variants.
// When includeVariants is false, Variants is left empty (list/search cards).
func (p *Product) EnrichFromVariants(variants []Variant, includeVariants bool) {
	if includeVariants {
		p.Variants = variants
	} else {
		p.Variants = nil
	}

	colors := make([]string, 0)
	sizes := make([]string, 0)
	colorSeen := map[string]bool{}
	sizeSeen := map[string]bool{}
	var priceFrom float64
	var sellingPriceFrom float64
	var hasPrice bool
	var defaultVariant *Variant

	for i := range variants {
		v := &variants[i]
		if v.Status != "" && v.Status != "active" {
			continue
		}
		if defaultVariant == nil {
			defaultVariant = v
		}
		if v.Color != "" && !colorSeen[v.Color] {
			colorSeen[v.Color] = true
			colors = append(colors, v.Color)
		}
		if v.Size != "" && !sizeSeen[v.Size] {
			sizeSeen[v.Size] = true
			sizes = append(sizes, v.Size)
		}
		if !hasPrice || v.Price < priceFrom {
			priceFrom = v.Price
			sellingPriceFrom = v.SellingPrice
			hasPrice = true
		}
	}

	p.AvailableColors = colors
	p.AvailableSizes = sizes
	if hasPrice {
		p.PriceFrom = priceFrom
		p.Price = priceFrom
		p.SellingPriceFrom = sellingPriceFrom
		p.SellingPrice = sellingPriceFrom
	}
	if defaultVariant != nil {
		p.Color = defaultVariant.Color
		p.ImageURLs = defaultVariant.ImageURLs
		if len(defaultVariant.ImageURLs) > 0 {
			p.DefaultImageURL = defaultVariant.ImageURLs[0]
		}
	}
}
