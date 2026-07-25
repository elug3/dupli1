package domain

// MergeUpdate returns a copy of the variant with any non-zero-value fields
// from incoming applied on top. Used by UpdateVariant so a partial request
// body (e.g. color-only) can't silently blank out size/status/images —
// omitted fields keep their current value instead of being overwritten with
// the JSON zero value. Identity fields (SkuID, SKU, ProductID, CreatedAt)
// and price fields (owned by the parent product) are never taken from incoming.
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
	if incoming.Status != "" {
		merged.Status = incoming.Status
	}
	if len(incoming.ImageURLs) > 0 {
		merged.ImageURLs = incoming.ImageURLs
	}
	// Price / SellingPrice stay on the parent product; clear any request values.
	merged.Price = existing.Price
	merged.SellingPrice = existing.SellingPrice
	return merged
}

// ApplyParentPrice copies the parent product's price onto a variant for API
// responses (cart/order still read price from the variant JSON).
func (v *Variant) ApplyParentPrice(p Product) {
	if v == nil {
		return
	}
	v.Price = p.Price
	v.SellingPrice = p.SellingPrice
}

// EnrichFromVariants fills summary and legacy display fields from variants.
// Price stays on the parent (Price / SellingPrice); PriceFrom mirrors them for
// older list-card clients. When includeVariants is false, Variants is left empty.
func (p *Product) EnrichFromVariants(variants []Variant, includeVariants bool) {
	p.PriceFrom = p.Price
	p.SellingPriceFrom = p.SellingPrice

	if includeVariants {
		stamped := make([]Variant, len(variants))
		for i := range variants {
			stamped[i] = variants[i]
			stamped[i].ApplyParentPrice(*p)
		}
		p.Variants = stamped
	} else {
		p.Variants = nil
	}

	colors := make([]string, 0)
	sizes := make([]string, 0)
	colorSeen := map[string]bool{}
	sizeSeen := map[string]bool{}
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
	}

	p.AvailableColors = colors
	p.AvailableSizes = sizes
	if defaultVariant != nil {
		p.Color = defaultVariant.Color
		p.ImageURLs = defaultVariant.ImageURLs
		if len(defaultVariant.ImageURLs) > 0 {
			p.DefaultImageURL = defaultVariant.ImageURLs[0]
		}
	}
}
