package permissions

import (
	"slices"
	"testing"
)

func TestCatalog_uniqueAndKnown(t *testing.T) {
	seen := make(map[string]struct{})
	for _, p := range Catalog {
		if !IsKnown(p) {
			t.Fatalf("%q in Catalog but not known", p)
		}
		if _, dup := seen[p]; dup {
			t.Fatalf("duplicate in Catalog: %q", p)
		}
		seen[p] = struct{}{}
	}
}

func TestValid(t *testing.T) {
	valid := []string{All, AdminAll, ProductAll, ProductCreate, "inventory.stock.write"}
	for _, v := range valid {
		if !Valid(v) {
			t.Fatalf("%q should be valid", v)
		}
	}
	invalid := []string{"", "Product.Create", "bad wildcard.", ".*", "UPPER.case"}
	for _, v := range invalid {
		if Valid(v) {
			t.Fatalf("%q should be invalid", v)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Validate([]string{ProductCreate, CouponAll}); err != nil {
		t.Fatal(err)
	}
	if err := Validate([]string{"not.a.real.permission"}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDedupe(t *testing.T) {
	got := Dedupe([]string{ProductCreate, ProductCreate, ProductUpdate})
	want := []string{ProductCreate, ProductUpdate}
	if !slices.Equal(got, want) {
		t.Fatalf("Dedupe = %v, want %v", got, want)
	}
}

func TestUnion(t *testing.T) {
	got := Union([]string{ProductCreate}, []string{ProductUpdate, ProductCreate})
	want := []string{ProductCreate, ProductUpdate}
	if !slices.Equal(got, want) {
		t.Fatalf("Union = %v, want %v", got, want)
	}
}

func TestUnion_empty(t *testing.T) {
	got := Union()
	if len(got) != 0 {
		t.Fatalf("Union() with no args = %v, want empty slice", got)
	}
}
