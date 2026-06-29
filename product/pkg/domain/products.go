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

// Product is the base struct for all product types
type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Price       float64  `json:"price"`
	Cost        float64  `json:"cost,omitempty"`
	Brand       string   `json:"brand"`
	Color       string   `json:"color"`
	Material    string   `json:"material"`
	Stock       int      `json:"stock"`
	Category    string   `json:"category"`
	Status      string   `json:"status"` // "active" | "draft" | "archived"
	ImageURLs   []string `json:"imageUrls,omitempty"`
	Capacity    string   `json:"capacity,omitempty"`
	Tags        []string `json:"tags,omitempty"` // "new", "hot", "top"
	CreatedAt   string   `json:"createdAt"`
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
