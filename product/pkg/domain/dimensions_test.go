package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

func TestNormalizeDimensions(t *testing.T) {
	t.Run("nil and empty clear", func(t *testing.T) {
		out, err := domain.NormalizeDimensions(nil)
		if err != nil || out != nil {
			t.Fatalf("nil: got %+v err=%v", out, err)
		}
		out, err = domain.NormalizeDimensions(&domain.Dimensions{})
		if err != nil || out != nil {
			t.Fatalf("empty: got %+v err=%v", out, err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		out, err := domain.NormalizeDimensions(&domain.Dimensions{WidthMm: 340, HeightMm: 220, DepthMm: 80})
		if err != nil {
			t.Fatal(err)
		}
		if out.WidthMm != 340 || out.HeightMm != 220 || out.DepthMm != 80 {
			t.Fatalf("got %+v", out)
		}
	})

	t.Run("partial axes ok", func(t *testing.T) {
		out, err := domain.NormalizeDimensions(&domain.Dimensions{WidthMm: 100, HeightMm: 50})
		if err != nil || out.DepthMm != 0 || out.WidthMm != 100 {
			t.Fatalf("got %+v err=%v", out, err)
		}
	})

	t.Run("negative rejected", func(t *testing.T) {
		if _, err := domain.NormalizeDimensions(&domain.Dimensions{WidthMm: -1}); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("over max rejected", func(t *testing.T) {
		if _, err := domain.NormalizeDimensions(&domain.Dimensions{WidthMm: domain.MaxDimensionMm + 1}); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestVariantMergeUpdate_Dimensions(t *testing.T) {
	existing := domain.Variant{
		SKU: "BOT_CAS001_GRN_M", Color: "Green", Size: "M", SizeCode: "M",
		Dimensions: &domain.Dimensions{WidthMm: 340, HeightMm: 220, DepthMm: 80},
		Status:     "active",
	}

	// Omitted dimensions keep existing.
	merged := existing.MergeUpdate(domain.Variant{Color: "Black"})
	if merged.Dimensions == nil || merged.Dimensions.WidthMm != 340 {
		t.Fatalf("omitted dimensions cleared: %+v", merged.Dimensions)
	}
	if merged.Color != "Black" {
		t.Fatalf("color = %q", merged.Color)
	}

	// Explicit replace.
	merged = existing.MergeUpdate(domain.Variant{
		Dimensions: &domain.Dimensions{WidthMm: 400, HeightMm: 250, DepthMm: 90},
	})
	if merged.Dimensions.WidthMm != 400 || merged.Dimensions.DepthMm != 90 {
		t.Fatalf("replace failed: %+v", merged.Dimensions)
	}

	// Empty object clears (NormalizeDimensions applied by service).
	merged = existing.MergeUpdate(domain.Variant{Dimensions: &domain.Dimensions{}})
	if merged.Dimensions == nil || !merged.Dimensions.Empty() {
		t.Fatalf("want empty dimensions for clear signal, got %+v", merged.Dimensions)
	}
}
