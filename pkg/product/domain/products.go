package domain

// Product is the base struct for all product types
type Product struct {
	ID          string
	Name        string
	Description string
	Price       float64
	Brand       string
	Color       string
	Material    string
	Stock       int
}

// Bag represents bags, purses, backpacks, and similar items
type Bag struct {
	Product
	Capacity string
}
