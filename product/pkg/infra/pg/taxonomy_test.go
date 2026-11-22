package pg

import (
	"errors"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// A database the old code ran on has the bag terms (written on every start)
// in a subcategory table with no category column. The migrate that adds the
// column copies the clothing subcategories the old code served from Go once,
// so clothing products stay editable; after that, a term staff delete stays
// deleted.
func TestMigrateTaxonomy_UpgradesTheCodeSeededSchemaOnce(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "taxonomy_upgrade_test")
	ctx := t.Context()

	for _, q := range []string{
		`CREATE TABLE bag_subcategories (code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO bag_subcategories (code, name, sort_order) VALUES ('handbags', 'Handbags', 0), ('tote', 'Tote', 1)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	catalog := NewCatalogStore(pool)
	subs, err := catalog.ListTaxonomy(ctx, domain.TaxonomySubCategory)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, sc := range subs {
		got[sc.Code] = sc.Category
	}
	want := map[string]string{"handbags": "bags", "tote": "bags", "padded": "clothing", "jackets": "clothing"}
	if len(got) != len(want) {
		t.Fatalf("subcategories after upgrade: %+v", subs)
	}
	for code, cat := range want {
		if got[code] != cat {
			t.Errorf("%s: category %q, want %q", code, got[code], cat)
		}
	}

	if err := catalog.DeleteTaxonomyTerm(ctx, domain.TaxonomySubCategory, "padded"); err != nil {
		t.Fatal(err)
	}
	if err := store.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if subs, _ := catalog.ListTaxonomy(ctx, domain.TaxonomySubCategory); len(subs) != 3 {
		t.Fatalf("deleted clothing subcategory came back on restart: %+v", subs)
	}
}

func TestCatalogStore_TaxonomyTerms(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "taxonomy_terms_test")
	ctx := t.Context()
	products := &ProductSearchStore{pool: pool}
	if err := products.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	loadReferenceData(t, pool)
	stock, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := products.SeedMockEcoBag(ctx, stock); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE products SET bag_style = 'casual', target = 'women', sub_category = 'tote'`); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalogStore(pool)

	created, err := catalog.CreateTaxonomyTerm(ctx, domain.TaxonomyBagStyle, domain.CatalogTerm{Code: "resort", Name: "Resort", Category: "ignored", SortOrder: 1})
	if err != nil || created.Category != "" {
		t.Fatalf("create style: %+v, %v", created, err)
	}
	if _, err := catalog.CreateTaxonomyTerm(ctx, domain.TaxonomyBagStyle, domain.CatalogTerm{Code: "resort", Name: "Again"}); !errors.Is(err, domain.ErrMasterExists) {
		t.Fatalf("duplicate: want ErrMasterExists, got %v", err)
	}
	if _, err := catalog.CreateTaxonomyTerm(ctx, domain.TaxonomySubCategory, domain.CatalogTerm{Code: "padded", Name: "Padded", Category: "bags"}); !errors.Is(err, domain.ErrMasterExists) {
		t.Fatalf("subcategory code from another category: want ErrMasterExists, got %v", err)
	}
	styles, err := catalog.ListTaxonomy(ctx, domain.TaxonomyBagStyle)
	if err != nil {
		t.Fatal(err)
	}
	// sort_order 1 ties with evening; ties break by code.
	if styles[0].Code != "casual" || styles[1].Code != "evening" || styles[2].Code != "resort" {
		t.Fatalf("styles order: %+v", styles)
	}

	name := "Resort & Travel"
	updated, err := catalog.UpdateTaxonomyTerm(ctx, domain.TaxonomyBagStyle, "resort", domain.CatalogTermPatch{Name: &name})
	if err != nil || updated.Name != name || updated.SortOrder != 1 {
		t.Fatalf("rename: %+v, %v", updated, err)
	}
	order := 7
	updated, err = catalog.UpdateTaxonomyTerm(ctx, domain.TaxonomySubCategory, "padded", domain.CatalogTermPatch{SortOrder: &order})
	if err != nil || updated.SortOrder != 7 || updated.Category != "clothing" || updated.Name != "Padded Jackets" {
		t.Fatalf("reorder subcategory: %+v, %v", updated, err)
	}
	if _, err := catalog.UpdateTaxonomyTerm(ctx, domain.TaxonomyTarget, "nobody", domain.CatalogTermPatch{Name: &name}); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("update unknown: want ErrMasterNotFound, got %v", err)
	}

	for kind, code := range map[domain.TaxonomyKind]string{
		domain.TaxonomyBagStyle:    "casual",
		domain.TaxonomyTarget:      "women",
		domain.TaxonomySubCategory: "tote",
	} {
		if err := catalog.DeleteTaxonomyTerm(ctx, kind, code); !errors.Is(err, domain.ErrMasterInUse) {
			t.Errorf("delete %s %s carried by a product: want ErrMasterInUse, got %v", kind, code, err)
		}
	}
	if err := catalog.DeleteTaxonomyTerm(ctx, domain.TaxonomyBagStyle, "resort"); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
	if err := catalog.DeleteTaxonomyTerm(ctx, domain.TaxonomyBagStyle, "resort"); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("delete again: want ErrMasterNotFound, got %v", err)
	}
}
