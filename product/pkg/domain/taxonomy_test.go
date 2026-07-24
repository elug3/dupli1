package domain

import "testing"

func TestNormalizeProductTaxonomy(t *testing.T) {
	p := Product{
		SubCategory: "Handbags",
		Style:       "EVENING",
		Target:      "mem", // typo → men
	}
	if err := NormalizeProductTaxonomy(&p); err != nil {
		t.Fatal(err)
	}
	if p.SubCategory != "handbags" || p.Style != "evening" || p.Target != "men" {
		t.Fatalf("got sub=%q style=%q target=%q", p.SubCategory, p.Style, p.Target)
	}

	bad := Product{SubCategory: "backpack"}
	if err := NormalizeProductTaxonomy(&bad); err == nil {
		t.Fatal("expected invalid subcategory error")
	}
	if err := NormalizeProductTaxonomy(&Product{Style: "formal"}); err == nil {
		t.Fatal("expected invalid style error")
	}
	if err := NormalizeProductTaxonomy(&Product{Target: "unisex"}); err == nil {
		t.Fatal("expected invalid target error")
	}
	empty := Product{}
	if err := NormalizeProductTaxonomy(&empty); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultMasterCatalog(t *testing.T) {
	c := DefaultMasterCatalog()
	if len(c.SubCategories) != 5 || len(c.Styles) != 5 || len(c.Targets) != 3 {
		t.Fatalf("unexpected lengths: %+v", c)
	}
	if c.SubCategories[0].Code != "handbags" || c.Styles[0].Code != "casual" || c.Targets[0].Code != "men" {
		t.Fatalf("unexpected first entries: %+v", c)
	}
}
