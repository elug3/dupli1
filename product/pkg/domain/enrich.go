package domain

import (
	"fmt"
	"math"
	"strings"
)

const listingThumbSuffix = ".w600.jpg"

// SyncListingImageURLs realigns listingImageUrls with a new imageUrls slice.
// Used when imageUrls is replaced without an explicit listingImageUrls body
// (e.g. manage-web deleteVariantImage). Surviving images keep their listing
// URL by URL match; new entries derive the {key}.w600.jpg sibling path.
func SyncListingImageURLs(oldImages, oldListings, newImages []string) []string {
	byImage := make(map[string]string, len(oldImages))
	for i, img := range oldImages {
		if img == "" {
			continue
		}
		if i < len(oldListings) && oldListings[i] != "" {
			byImage[img] = oldListings[i]
		}
	}
	out := make([]string, len(newImages))
	for i, img := range newImages {
		if listing, ok := byImage[img]; ok {
			out[i] = listing
			continue
		}
		out[i] = DeriveListingImageURL(img)
	}
	return out
}

// DeriveListingImageURL returns the listing-thumb URL for a full-size image URL.
func DeriveListingImageURL(imageURL string) string {
	if strings.HasSuffix(imageURL, listingThumbSuffix) {
		return imageURL
	}
	return imageURL + listingThumbSuffix
}

// MergeUpdate returns a copy of the variant with any non-zero-value fields
// from incoming applied on top. Used by UpdateVariant so a partial request
// body (e.g. color-only) can't silently blank out size/status/images —
// omitted fields keep their current value instead of being overwritten with
// the JSON zero value. Identity fields (SkuID, SKU, ProductID, CreatedAt)
// is never taken from incoming; price overrides are, where 0 clears one.
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
	if len(incoming.ListingImageURLs) > 0 {
		merged.ListingImageURLs = incoming.ListingImageURLs
	}
	// Dimensions: nil means omit (keep existing); non-nil replaces
	// (including {} which NormalizeDimensions treats as clear).
	if incoming.Dimensions != nil {
		merged.Dimensions = incoming.Dimensions
	}
	merged.PriceOverride = mergeOverride(existing.PriceOverride, incoming.PriceOverride)
	merged.OfficialPriceOverride = mergeOverride(existing.OfficialPriceOverride, incoming.OfficialPriceOverride)
	// Price / OfficialPrice are response-only effective values.
	merged.Price = existing.Price
	merged.OfficialPrice = existing.OfficialPrice
	return merged
}

// mergeOverride keeps current when incoming is nil, clears on 0, else replaces.
func mergeOverride(current, incoming *float64) *float64 {
	if incoming == nil {
		return current
	}
	if *incoming == 0 {
		return nil
	}
	v := *incoming
	return &v
}

// ValidateVariantPrices rejects negative or fractional SKU price overrides
// (KRW is whole won).
func ValidateVariantPrices(v Variant) error {
	for name, p := range map[string]*float64{
		"priceOverride":         v.PriceOverride,
		"officialPriceOverride": v.OfficialPriceOverride,
	} {
		if p == nil {
			continue
		}
		if *p < 0 || *p != math.Trunc(*p) {
			return fmt.Errorf("%s must be a non-negative whole won amount", name)
		}
	}
	return nil
}

// MergeUpdate returns a copy of the product with non-zero / non-empty fields
// from incoming applied on top. Used by UpdateProduct so a partial body
// (e.g. style-only) cannot wipe price, name, or other omitted fields with
// JSON zero values. BrandCode / StyleCode / counters / timestamps are never
// taken from incoming.
func (existing Product) MergeUpdate(incoming Product) Product {
	merged := existing
	if incoming.Name != "" {
		merged.Name = incoming.Name
	}
	if incoming.Description != "" {
		merged.Description = incoming.Description
	}
	if incoming.Brand != "" {
		merged.Brand = incoming.Brand
	}
	if incoming.Material != "" {
		merged.Material = incoming.Material
	}
	if incoming.Category != "" {
		merged.Category = incoming.Category
	}
	if incoming.SubCategory != "" {
		merged.SubCategory = incoming.SubCategory
	}
	if incoming.Style != "" {
		merged.Style = incoming.Style
	}
	if incoming.Target != "" {
		merged.Target = incoming.Target
	}
	if incoming.Status != "" {
		merged.Status = incoming.Status
	}
	if incoming.Capacity != "" {
		merged.Capacity = incoming.Capacity
	}
	if incoming.Tags != nil {
		merged.Tags = incoming.Tags
	}
	if incoming.SizeChart != nil {
		// [] clears the chart; an omitted field keeps it.
		merged.SizeChart = incoming.SizeChart
	}
	if incoming.Attributes != nil {
		merged.Attributes = incoming.Attributes
	}
	if incoming.Price != 0 {
		merged.Price = incoming.Price
	}
	if incoming.OfficialPrice != 0 {
		merged.OfficialPrice = incoming.OfficialPrice
	}
	// Identity / denormalized counters stay on the existing row.
	merged.ID = existing.ID
	merged.BrandCode = existing.BrandCode
	merged.StyleCode = existing.StyleCode
	merged.CreatedAt = existing.CreatedAt
	merged.UpdatedAt = existing.UpdatedAt
	merged.CreatedBy = existing.CreatedBy
	merged.ViewCount = existing.ViewCount
	merged.SoldCount = existing.SoldCount
	merged.WishlistCount = existing.WishlistCount
	return merged
}

// ApplyParentPrice sets the variant's effective price for API responses
// (cart/order read price from the variant JSON): its own override when set,
// otherwise the parent product's.
func (v *Variant) ApplyParentPrice(p Product) {
	if v == nil {
		return
	}
	v.Price = p.Price
	if v.PriceOverride != nil {
		v.Price = *v.PriceOverride
	}
	v.OfficialPrice = p.OfficialPrice
	if v.OfficialPriceOverride != nil {
		v.OfficialPrice = *v.OfficialPriceOverride
	}
}

// priceFrom returns the lowest effective price across active variants, or 0
// when they all share one price (nothing to show "from" for).
func priceFrom(p Product, variants []Variant) float64 {
	var lo, hi float64
	seen := false
	for _, v := range variants {
		if v.Status != "" && v.Status != "active" {
			continue
		}
		v.ApplyParentPrice(p)
		if !seen || v.Price < lo {
			lo = v.Price
		}
		if !seen || v.Price > hi {
			hi = v.Price
		}
		seen = true
	}
	if !seen || lo == hi {
		return 0
	}
	return lo
}

// EnrichFromVariants fills summary and legacy display fields from variants.
// Price / OfficialPrice stay on the parent. When includeVariants is false,
// Variants is left empty (list/search cards).
func (p *Product) EnrichFromVariants(variants []Variant, includeVariants bool) {
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

	p.PriceFrom = priceFrom(*p, variants)
	p.AvailableColors = colors
	p.AvailableSizes = sizes
	if defaultVariant != nil {
		p.Color = defaultVariant.Color
		p.ImageURLs = defaultVariant.ImageURLs
		if len(defaultVariant.ImageURLs) > 0 {
			p.DefaultImageURL = defaultVariant.ImageURLs[0]
		}
		if len(defaultVariant.ListingImageURLs) > 0 {
			p.DefaultListingImageURL = defaultVariant.ListingImageURLs[0]
		}
	}
}
