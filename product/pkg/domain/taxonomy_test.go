package domain

import "testing"

// testTaxonomy is what scripts/seed/reference-data.json loads, as staff
// would hold it in the store.
func testTaxonomy() Taxonomy {
	sub := func(cat string, codes ...[2]string) []CatalogTerm {
		var out []CatalogTerm
		for i, c := range codes {
			out = append(out, CatalogTerm{Code: c[0], Name: c[1], Category: cat, SortOrder: i})
		}
		return out
	}
	terms := func(codes ...[2]string) []CatalogTerm { return sub("", codes...) }
	return Taxonomy{
		SubCategories: append(
			sub("bags", [2]string{"handbags", "Handbags"}, [2]string{"tote", "Tote"}, [2]string{"shoulder", "Shoulder"},
				[2]string{"cross", "Crossbody"}, [2]string{"mini", "Mini"}),
			sub("clothing", [2]string{"padded", "Padded Jackets"}, [2]string{"jackets", "Jackets"})...),
		Styles: terms([2]string{"casual", "Casual"}, [2]string{"evening", "Evening"}, [2]string{"business", "Business"},
			[2]string{"weekend", "Weekend"}, [2]string{"statement", "Statement"}),
		Targets: terms([2]string{"all", "All"}, [2]string{"men", "Men"}, [2]string{"women", "Women"}, [2]string{"kids", "Kids"}),
	}
}

func TestNormalizeProductTaxonomy(t *testing.T) {
	p := Product{
		SubCategory: "Handbags",
		Style:       "EVENING",
		Target:      "mem", // typo → men
	}
	if err := NormalizeProductTaxonomy(&p, "", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
	if p.SubCategory != "handbags" || p.Style != "evening" || p.Target != "men" {
		t.Fatalf("got sub=%q style=%q target=%q", p.SubCategory, p.Style, p.Target)
	}

	bad := Product{SubCategory: "backpack"}
	if err := NormalizeProductTaxonomy(&bad, "", testTaxonomy()); err == nil {
		t.Fatal("expected invalid subcategory error")
	}
	if err := NormalizeProductTaxonomy(&Product{Style: "formal"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected invalid style error")
	}
	if err := NormalizeProductTaxonomy(&Product{Target: "unisex"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected invalid target error")
	}
	all := Product{Target: "ALL"}
	if err := NormalizeProductTaxonomy(&all, "", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
	if all.Target != "all" {
		t.Fatalf("got target=%q, want all", all.Target)
	}
	empty := Product{}
	if err := NormalizeProductTaxonomy(&empty, "", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
}

// The terms are whatever the store holds: a term staff add is accepted, and
// one they delete is refused.
func TestNormalizeProductTaxonomyFollowsTheStore(t *testing.T) {
	tax := testTaxonomy()
	tax.Styles = append(tax.Styles, CatalogTerm{Code: "resort", Name: "Resort"})
	p := Product{Style: "Resort"}
	if err := NormalizeProductTaxonomy(&p, "", tax); err != nil || p.Style != "resort" {
		t.Fatalf("added style: err=%v style=%q", err, p.Style)
	}
	if err := NormalizeProductTaxonomy(&Product{SubCategory: "tote"}, "", Taxonomy{}); err == nil {
		t.Fatal("expected tote to be refused when the store holds no subcategories")
	}
	if err := NormalizeProductTaxonomy(&Product{}, "", Taxonomy{}); err != nil {
		t.Fatalf("blank terms with an empty store: %v", err)
	}
}

func TestValidTaxonomyCode(t *testing.T) {
	for _, c := range []string{"tote", "cross-body", "a1", "x_y"} {
		if !ValidTaxonomyCode(c) {
			t.Errorf("%q should be valid", c)
		}
	}
	for _, c := range []string{"", "Tote", "-x", "_x", "a b", "a/b", "abcdefghijabcdefghijabcdefghijabc"} {
		if ValidTaxonomyCode(c) {
			t.Errorf("%q should be invalid", c)
		}
	}
}

func TestNormalizeProductTaxonomyScopesSubCategoryToCategory(t *testing.T) {
	jacket := Product{Category: "Clothing", SubCategory: "PADDED", Target: "women"}
	if err := NormalizeProductTaxonomy(&jacket, "", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
	if jacket.Category != "clothing" || jacket.SubCategory != "padded" {
		t.Fatalf("got category=%q sub=%q", jacket.Category, jacket.SubCategory)
	}
	unpadded := Product{Category: "clothing", SubCategory: "Jackets", Target: "men"}
	if err := NormalizeProductTaxonomy(&unpadded, "", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
	if unpadded.SubCategory != "jackets" {
		t.Fatalf("got sub=%q, want jackets", unpadded.SubCategory)
	}

	bag := Product{Category: "Bags", SubCategory: "tote"}
	if err := NormalizeProductTaxonomy(&bag, "", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
	if bag.Category != "bags" {
		t.Fatalf("got category=%q, want bags", bag.Category)
	}

	// A bag term on a jacket, and a jacket term on a bag, are both refused.
	if err := NormalizeProductTaxonomy(&Product{Category: "clothing", SubCategory: "tote"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected tote to be refused for clothing")
	}
	if err := NormalizeProductTaxonomy(&Product{Category: "bags", SubCategory: "padded"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected padded to be refused for bags")
	}
	if err := NormalizeProductTaxonomy(&Product{Category: "bags", SubCategory: "jackets"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected jackets to be refused for bags")
	}
	// A blank category is validated as bags and stays blank.
	blank := Product{SubCategory: "tote"}
	if err := NormalizeProductTaxonomy(&blank, "", testTaxonomy()); err != nil || blank.Category != "" {
		t.Fatalf("blank category: err=%v category=%q", err, blank.Category)
	}
	if err := NormalizeProductTaxonomy(&Product{SubCategory: "padded"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected padded to be refused without a category")
	}
}

func TestNormalizeProductTaxonomyCategory(t *testing.T) {
	if err := NormalizeProductTaxonomy(&Product{Category: "shoes"}, "", testTaxonomy()); err == nil {
		t.Fatal("expected an unknown category to be refused on create")
	}
	// A legacy free-text category the row already carries stays editable.
	legacy := Product{Category: "handbag", SubCategory: "tote"}
	if err := NormalizeProductTaxonomy(&legacy, "handbag", testTaxonomy()); err != nil {
		t.Fatal(err)
	}
	if legacy.Category != "handbag" {
		t.Fatalf("legacy category rewritten to %q", legacy.Category)
	}
	// …but cannot be changed to another unknown one.
	if err := NormalizeProductTaxonomy(&Product{Category: "shoes"}, "handbag", testTaxonomy()); err == nil {
		t.Fatal("expected a new unknown category to be refused")
	}
}

func TestCategories(t *testing.T) {
	cats := Categories(testTaxonomy())
	if len(cats) != 2 || cats[0].Code != "bags" || cats[1].Code != "clothing" {
		t.Fatalf("categories: %+v", cats)
	}
	if len(cats[0].SubCategories) != 5 || cats[0].SubCategories[0].Code != "handbags" {
		t.Fatalf("bag subcategories: %+v", cats[0].SubCategories)
	}
	if len(cats[1].SubCategories) != 2 || cats[1].SubCategories[0].Code != "padded" || cats[1].SubCategories[1].Code != "jackets" {
		t.Fatalf("clothing subcategories: %+v", cats[1].SubCategories)
	}
	cats[1].Sizes[0] = "mutated"
	if SeedCategories[1].Sizes[0] != "XXS" {
		t.Fatal("Categories must return a copy")
	}
	if got := Categories(Taxonomy{}); len(got[0].SubCategories) != 0 || got[0].SubCategories == nil {
		t.Fatalf("empty store: want [] subcategories, got %#v", got[0].SubCategories)
	}
}
