package domain

import "testing"

func TestNormalizeProductTaxonomy(t *testing.T) {
	p := Product{
		SubCategory: "Handbags",
		Style:       "EVENING",
		Target:      "mem", // typo → men
	}
	if err := NormalizeProductTaxonomy(&p, ""); err != nil {
		t.Fatal(err)
	}
	if p.SubCategory != "handbags" || p.Style != "evening" || p.Target != "men" {
		t.Fatalf("got sub=%q style=%q target=%q", p.SubCategory, p.Style, p.Target)
	}

	bad := Product{SubCategory: "backpack"}
	if err := NormalizeProductTaxonomy(&bad, ""); err == nil {
		t.Fatal("expected invalid subcategory error")
	}
	if err := NormalizeProductTaxonomy(&Product{Style: "formal"}, ""); err == nil {
		t.Fatal("expected invalid style error")
	}
	if err := NormalizeProductTaxonomy(&Product{Target: "unisex"}, ""); err == nil {
		t.Fatal("expected invalid target error")
	}
	all := Product{Target: "ALL"}
	if err := NormalizeProductTaxonomy(&all, ""); err != nil {
		t.Fatal(err)
	}
	if all.Target != "all" {
		t.Fatalf("got target=%q, want all", all.Target)
	}
	empty := Product{}
	if err := NormalizeProductTaxonomy(&empty, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultMasterCatalog(t *testing.T) {
	c := DefaultMasterCatalog()
	if len(c.SubCategories) != 5 || len(c.Styles) != 5 || len(c.Targets) != 4 {
		t.Fatalf("unexpected lengths: %+v", c)
	}
	if c.SubCategories[0].Code != "handbags" || c.Styles[0].Code != "casual" || c.Targets[0].Code != "all" {
		t.Fatalf("unexpected first entries: %+v", c)
	}
}

func TestNormalizeProductTaxonomyScopesSubCategoryToCategory(t *testing.T) {
	jacket := Product{Category: "Clothing", SubCategory: "PADDED", Target: "women"}
	if err := NormalizeProductTaxonomy(&jacket, ""); err != nil {
		t.Fatal(err)
	}
	if jacket.Category != "clothing" || jacket.SubCategory != "padded" {
		t.Fatalf("got category=%q sub=%q", jacket.Category, jacket.SubCategory)
	}
	unpadded := Product{Category: "clothing", SubCategory: "Jackets", Target: "men"}
	if err := NormalizeProductTaxonomy(&unpadded, ""); err != nil {
		t.Fatal(err)
	}
	if unpadded.SubCategory != "jackets" {
		t.Fatalf("got sub=%q, want jackets", unpadded.SubCategory)
	}

	bag := Product{Category: "Bags", SubCategory: "tote"}
	if err := NormalizeProductTaxonomy(&bag, ""); err != nil {
		t.Fatal(err)
	}
	if bag.Category != "bags" {
		t.Fatalf("got category=%q, want bags", bag.Category)
	}

	// A bag term on a jacket, and a jacket term on a bag, are both refused.
	if err := NormalizeProductTaxonomy(&Product{Category: "clothing", SubCategory: "tote"}, ""); err == nil {
		t.Fatal("expected tote to be refused for clothing")
	}
	if err := NormalizeProductTaxonomy(&Product{Category: "bags", SubCategory: "padded"}, ""); err == nil {
		t.Fatal("expected padded to be refused for bags")
	}
	if err := NormalizeProductTaxonomy(&Product{Category: "bags", SubCategory: "jackets"}, ""); err == nil {
		t.Fatal("expected jackets to be refused for bags")
	}
	// A blank category is validated as bags and stays blank.
	blank := Product{SubCategory: "tote"}
	if err := NormalizeProductTaxonomy(&blank, ""); err != nil || blank.Category != "" {
		t.Fatalf("blank category: err=%v category=%q", err, blank.Category)
	}
	if err := NormalizeProductTaxonomy(&Product{SubCategory: "padded"}, ""); err == nil {
		t.Fatal("expected padded to be refused without a category")
	}
}

func TestNormalizeProductTaxonomyCategory(t *testing.T) {
	if err := NormalizeProductTaxonomy(&Product{Category: "shoes"}, ""); err == nil {
		t.Fatal("expected an unknown category to be refused on create")
	}
	// A legacy free-text category the row already carries stays editable.
	legacy := Product{Category: "handbag", SubCategory: "tote"}
	if err := NormalizeProductTaxonomy(&legacy, "handbag"); err != nil {
		t.Fatal(err)
	}
	if legacy.Category != "handbag" {
		t.Fatalf("legacy category rewritten to %q", legacy.Category)
	}
	// …but cannot be changed to another unknown one.
	if err := NormalizeProductTaxonomy(&Product{Category: "shoes"}, "handbag"); err == nil {
		t.Fatal("expected a new unknown category to be refused")
	}
}

func TestCategories(t *testing.T) {
	cats := Categories()
	if len(cats) != 2 || cats[0].Code != "bags" || cats[1].Code != "clothing" {
		t.Fatalf("categories: %+v", cats)
	}
	if len(cats[0].SubCategories) != len(SeedSubCategories) {
		t.Fatalf("bag subcategories: %+v", cats[0].SubCategories)
	}
	if len(cats[1].SubCategories) != 2 || cats[1].SubCategories[0].Code != "padded" || cats[1].SubCategories[1].Code != "jackets" {
		t.Fatalf("clothing subcategories: %+v", cats[1].SubCategories)
	}
	cats[0].SubCategories[0].Code = "mutated"
	if SeedSubCategories[0].Code != "handbags" {
		t.Fatal("Categories must return a copy")
	}
}
