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
	ID          string
	Name        string
	Description string
	Price       float64
	Brand       string
	Color       string
	Material    string
	Stock       int
	Category    string
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

// Bag represents bags, purses, backpacks, and similar items
type Bag struct {
	Product
	Capacity string
}

// Clock represents timepieces
type Clock struct {
	Product
	Type string // Analog, Digital, Smart, etc.
}
