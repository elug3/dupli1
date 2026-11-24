package memory

import "github.com/elug3/dupli1/product/pkg/domain"

// Test fixtures the in-memory catalog starts with, so a test can create a
// product without first creating its brand, color, size and edition. The
// product service itself seeds none of these (it only ever runs on
// Postgres); a real environment gets them from scripts/seed-reference-data.py,
// whose scripts/seed/reference-data.json holds the same rows.

var fixtureBrands = []domain.Brand{
	{Code: "DUP", Name: "Dupli1 Studio"},
	{Code: "PR", Name: "Prada"},
	{Code: "BOT", Name: "Bottega Veneta"},
	{Code: "LV", Name: "Louis Vuitton"},
	{Code: "CD", Name: "Dior"},
	{Code: "CH", Name: "Chanel"},
	{Code: "GUC", Name: "Gucci"},
	{Code: "HER", Name: "Hermes"},
	{Code: "CEL", Name: "Celine"},
	{Code: "BAL", Name: "Balenciaga"},
	{Code: "FEN", Name: "Fendi"},
	{Code: "YSL", Name: "Saint Laurent"},
	{Code: "BUR", Name: "Burberry"},
}

var fixtureColors = []domain.Color{
	{Code: "BLK", Name: "Black", Hex: "#0a0a0a"},
	{Code: "WHT", Name: "White", Hex: "#ffffff"},
	{Code: "CRM", Name: "Cream", Hex: "#efe6d2"},
	{Code: "BRN", Name: "Brown", Hex: "#6b4226"},
	{Code: "GRN", Name: "Green", Hex: "#3f6b4a"},
	{Code: "RED", Name: "Red", Hex: "#b3261e"},
	{Code: "BLU", Name: "Blue", Hex: "#2f5d9a"},
	{Code: "BGE", Name: "Beige", Hex: "#d8c3a5"},
	{Code: "NVY", Name: "Navy", Hex: "#1f2a44"},
	{Code: "PNK", Name: "Pink", Hex: "#e8b4bc"},
	{Code: "GRY", Name: "Grey", Hex: "#8e8e8e"},
	{Code: "GLD", Name: "Gold", Hex: "#c8a96e"},
	{Code: "SLV", Name: "Silver", Hex: "#c0c3c7"},
	{Code: "ORG", Name: "Orange", Hex: "#d9772b"},
	{Code: "YLW", Name: "Yellow", Hex: "#e6c34a"},
	{Code: "PRP", Name: "Purple", Hex: "#5e3d7a"},
	{Code: "TAN", Name: "Tan", Hex: "#c19a6b"},
	{Code: "OLV", Name: "Olive", Hex: "#6b6b3a"},
}

var fixtureSizes = []domain.Size{
	{Code: "XXS", Name: "XXS"},
	{Code: "XS", Name: "XS"},
	{Code: "S", Name: "S"},
	{Code: "M", Name: "M"},
	{Code: "L", Name: "L"},
	{Code: "XL", Name: "XL"},
	{Code: "XXL", Name: "XXL"},
	{Code: "MIN", Name: "Mini"},
	{Code: "SML", Name: "Small"},
	{Code: "MED", Name: "Medium"},
	{Code: "LRG", Name: "Large"},
	{Code: "OS", Name: "One Size"},
}

var fixtureEditions = []domain.Edition{
	{Code: "V", Name: "Standard"},
	{Code: "A", Name: "Alternate construction"},
	{Code: "R", Name: "Limited / special edition"},
}

// fixtureTaxonomy is reference-data.json's subcategories, bag_styles and
// targets, with each list's position as its sort order.
var fixtureTaxonomy = map[domain.TaxonomyKind][]domain.CatalogTerm{
	domain.TaxonomySubCategory: {
		{Code: "handbags", Name: "Handbags", Category: "bags", SortOrder: 0},
		{Code: "tote", Name: "Tote", Category: "bags", SortOrder: 1},
		{Code: "shoulder", Name: "Shoulder", Category: "bags", SortOrder: 2},
		{Code: "cross", Name: "Crossbody", Category: "bags", SortOrder: 3},
		{Code: "mini", Name: "Mini", Category: "bags", SortOrder: 4},
		{Code: "padded", Name: "Padded Jackets", Category: "clothing", SortOrder: 0},
		{Code: "jackets", Name: "Jackets", Category: "clothing", SortOrder: 1},
	},
	domain.TaxonomyBagStyle: {
		{Code: "casual", Name: "Casual", SortOrder: 0},
		{Code: "evening", Name: "Evening", SortOrder: 1},
		{Code: "business", Name: "Business", SortOrder: 2},
		{Code: "weekend", Name: "Weekend", SortOrder: 3},
		{Code: "statement", Name: "Statement", SortOrder: 4},
	},
	domain.TaxonomyTarget: {
		{Code: "all", Name: "All", SortOrder: 0},
		{Code: "men", Name: "Men", SortOrder: 1},
		{Code: "women", Name: "Women", SortOrder: 2},
		{Code: "kids", Name: "Kids", SortOrder: 3},
	},
}
