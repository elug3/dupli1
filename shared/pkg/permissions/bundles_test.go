package permissions

import (
	"slices"
	"testing"
)

func TestExpandBundle_catalogEditor(t *testing.T) {
	got, err := ExpandBundle(BundleCatalogEditor)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{ProductCreate, ProductUpdate, ProductRead, ProductVariantCreate, ProductVariantUpdate, ProductImageUpload} {
		if !slices.Contains(got, p) {
			t.Fatalf("catalog_editor missing %s", p)
		}
	}
}

func TestExpandBundle_catalogAdminWildcards(t *testing.T) {
	got, err := ExpandBundle(BundleCatalogAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{ProductAll, CouponAll}) {
		t.Fatalf("catalog_admin = %v", got)
	}
	if !Has(got, ProductCreate) {
		t.Fatal("catalog_admin wildcards should grant product.create")
	}
}

func TestExpandBundle_unknown(t *testing.T) {
	_, err := ExpandBundle("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExpandBundles_union(t *testing.T) {
	got, err := ExpandBundles(BundleCustomerRegistrar, BundleUserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, UserCreate) {
		t.Fatal("missing user.create")
	}
	if !slices.Contains(got, UserRead) {
		t.Fatal("missing user.read")
	}
}

func TestBundleNames_sorted(t *testing.T) {
	names := BundleNames()
	if !slices.IsSorted(names) {
		t.Fatalf("BundleNames not sorted: %v", names)
	}
}
