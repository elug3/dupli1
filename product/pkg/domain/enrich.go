package domain

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
			hasPrice = true
		}
	}

	p.AvailableColors = colors
	p.AvailableSizes = sizes
	if hasPrice {
		p.PriceFrom = priceFrom
		p.Price = priceFrom
	}
	if defaultVariant != nil {
		p.Color = defaultVariant.Color
		p.ImageURLs = defaultVariant.ImageURLs
		if len(defaultVariant.ImageURLs) > 0 {
			p.DefaultImageURL = defaultVariant.ImageURLs[0]
		}
	}
}
