package domain

// CatalogCount is one group of the catalog report as the store reads it: the
// products sharing a brand, category, subcategory and status, with their SKUs
// and the stock on those SKUs.
type CatalogCount struct {
	BrandCode   string
	Brand       string
	Category    string
	SubCategory string
	Status      string
	Products    int
	SKUs        int
	Units       int
	Reserved    int
}

// CatalogTally is what each row of the catalog report counts. Products are
// parents (styles); SKUs are every variant of them; Units is stock on hand on
// those SKUs and Reserved the part of it held for open orders.
type CatalogTally struct {
	Products int `json:"products"`
	SKUs     int `json:"skus"`
	Units    int `json:"units"`
	Reserved int `json:"reserved"`
}

// Add accumulates c into t.
func (t *CatalogTally) Add(c CatalogCount) {
	t.Products += c.Products
	t.SKUs += c.SKUs
	t.Units += c.Units
	t.Reserved += c.Reserved
}

// CatalogReport is the response of GET /api/v1/products/reports/catalog.
type CatalogReport struct {
	// Status is the product status the report was narrowed to; empty is all.
	Status string       `json:"status,omitempty"`
	Totals CatalogTally `json:"totals"`
	// ProductsByStatus counts products per status across the whole catalog,
	// whatever Status narrowed the rest to.
	ProductsByStatus map[string]int `json:"products_by_status"`
	// Brands lists every brand master, plus any brand a product names without
	// one, most products first. A brand with no products is listed with zeros.
	Brands     []CatalogBrandCount    `json:"brands"`
	Categories []CatalogCategoryCount `json:"categories"`
	// Cells is the brand × subcategory grid, non-empty cells only.
	Cells []CatalogCell `json:"cells"`
}

// CatalogBrandCount is one brand's row. Code is empty for products that name
// a brand outside the brand masters.
type CatalogBrandCount struct {
	Code string `json:"code"`
	Name string `json:"name"`
	CatalogTally
}

// CatalogCategoryCount is one category with its subcategories, in taxonomy
// order. A subcategory with an empty code holds products that have none.
type CatalogCategoryCount struct {
	Code string `json:"code"`
	Name string `json:"name"`
	CatalogTally
	SubCategories []CatalogSubCategoryCount `json:"sub_categories"`
}

type CatalogSubCategoryCount struct {
	Code string `json:"code"`
	Name string `json:"name"`
	CatalogTally
}

// CatalogCell counts the products of one brand in one subcategory.
type CatalogCell struct {
	BrandCode   string `json:"brand_code"`
	Brand       string `json:"brand"`
	Category    string `json:"category"`
	SubCategory string `json:"sub_category"`
	CatalogTally
}
