package domain

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
	CreatedAt   string   `json:"createdAt"`
}

// Bag represents bags, purses, backpacks, and similar items
type Bag struct {
	Product
	Capacity string `json:"capacity"`
}
