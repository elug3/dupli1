package domain

// Consultation represents a service consultation offering
type Consultation struct {
	ID          string
	Title       string
	Description string
	Duration    int // in minutes
	Price       float64
	Status      string
}

// Variant is a sellable option (SKU) under a parent product style.
type Variant struct {
	// SkuID is the canonical, immutable cross-service identifier (ULID).
	SkuID     string `json:"skuId,omitempty"`
	SKU       string `json:"sku"`
	ProductID string `json:"productId"`
	Color     string `json:"color"`
	Size      string `json:"size,omitempty"`
	// Normalized SKU segment codes (see docs/product-sku-system.md).
	ColorCode   string `json:"colorCode,omitempty"`
	EditionCode string `json:"editionCode,omitempty"` // optional VariantCode segment
	SizeCode    string `json:"sizeCode,omitempty"`
	// Price and SellingPrice are filled from the parent product on read.
	// They are not stored on the SKU row (price lives on Product).
	SellingPrice float64 `json:"sellingPrice,omitempty"`
	Price        float64 `json:"price,omitempty"`
	Status       string  `json:"status"` // "active" | "draft" | "archived"
	ImageURLs    []string `json:"imageUrls,omitempty"`
	CreatedAt    string   `json:"createdAt,omitempty"`
}

// Product is a parent catalog style. Sellable options live on Variants.
type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Brand       string   `json:"brand"`
	BrandCode   string   `json:"brandCode,omitempty"`
	StyleCode   string   `json:"styleCode,omitempty"`
	Material    string   `json:"material"`
	Category    string   `json:"category"`
	// SubCategory is a bag type under category (handbags, tote, shoulder, cross, mini).
	SubCategory string `json:"subCategory,omitempty"`
	// Style is bag occasion / look (casual, evening, business, weekend, statement).
	// Distinct from StyleCode (SKU design-family master).
	Style string `json:"style,omitempty"`
	// Target is audience (men, women, kids).
	Target string `json:"target,omitempty"`
	// SellingPrice is the official/display price in KRW won (strikethrough / "was" price).
	SellingPrice float64 `json:"sellingPrice,omitempty"`
	// Price is the real sale price in KRW won (whole won; single currency).
	// Stored on the parent; all variants inherit this price.
	Price    float64  `json:"price"`
	Status   string   `json:"status"` // "active" | "draft" | "archived"
	Capacity string   `json:"capacity,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	// ViewCount is unique guest PDP views (denormalized). Public on PDP and recs.
	ViewCount int64 `json:"viewCount"`
	// SoldCount is units committed from inventory reservations (denormalized).
	// Incremented when a reservation is committed (order ship → in_transit), not on payment.
	SoldCount int64 `json:"soldCount"`
	// WishlistCount is unique owners who wishlisted this parent (denormalized).
	WishlistCount int64 `json:"wishlistCount"`
	CreatedAt     string `json:"createdAt"`
	CreatedBy     string `json:"createdBy,omitempty"`

	// Summary fields derived from variants / parent (not separate storage).
	// SellingPriceFrom mirrors SellingPrice (compat with older list-card clients).
	SellingPriceFrom float64 `json:"sellingPriceFrom,omitempty"`
	// PriceFrom mirrors Price (compat with older list-card clients).
	PriceFrom       float64   `json:"priceFrom,omitempty"`
	DefaultImageURL string    `json:"defaultImageUrl,omitempty"`
	AvailableColors []string  `json:"availableColors,omitempty"`
	AvailableSizes  []string  `json:"availableSizes,omitempty"`
	Variants        []Variant `json:"variants,omitempty"`

	// Legacy display fields mirrored from the default active variant.
	Color     string   `json:"color,omitempty"`
	Stock     int      `json:"stock,omitempty"`
	ImageURLs []string `json:"imageUrls,omitempty"`
}

// Shoes represents footwear products
type Shoes struct {
	Product
	Size   string
	Gender string
}

// Outerwear represents jackets, coats, and similar items
type Outerwear struct {
	Product
	Size   string
	Gender string
}

// Bottoms represents trousers, pants, skirts, and similar items
type Bottoms struct {
	Product
	Size   string
	Gender string
}

// Bag represents bags, purses, backpacks, and similar items.
type Bag struct {
	Product
}

// Clock represents timepieces
type Clock struct {
	Product
	Type string // Analog, Digital, Smart, etc.
}
