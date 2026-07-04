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
	SKU       string   `json:"sku"`
	ProductID string   `json:"productId"`
	Color     string   `json:"color"`
	Size      string   `json:"size,omitempty"`
	Price     float64  `json:"price"`
	Status    string   `json:"status"` // "active" | "draft" | "archived"
	ImageURLs []string `json:"imageUrls,omitempty"`
	CreatedAt string   `json:"createdAt,omitempty"`
}

// Product is a parent catalog style. Sellable options live on Variants.
type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Brand       string  `json:"brand"`
	Material    string  `json:"material"`
	Category    string  `json:"category"`
	Status      string  `json:"status"` // "active" | "draft" | "archived"
	Cost        float64 `json:"cost,omitempty"`
	Capacity    string  `json:"capacity,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   string  `json:"createdAt"`

	// Summary fields derived from variants (not stored on parent).
	PriceFrom         float64   `json:"priceFrom,omitempty"`
	DefaultImageURL   string    `json:"defaultImageUrl,omitempty"`
	AvailableColors   []string  `json:"availableColors,omitempty"`
	AvailableSizes    []string  `json:"availableSizes,omitempty"`
	Variants          []Variant `json:"variants,omitempty"`

	// Legacy fields mirrored from the default variant for older clients.
	Price     float64  `json:"price,omitempty"`
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
