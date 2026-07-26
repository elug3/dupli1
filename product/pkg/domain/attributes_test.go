package domain

import (
	"fmt"
	"testing"
)

func TestNormalizeAttributes(t *testing.T) {
	out, err := NormalizeAttributes(map[string]string{
		" condition ": " excellent ",
		"":            "skip",
		"care":        "wipe dry",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["condition"] != "excellent" || out["care"] != "wipe dry" {
		t.Fatalf("got %#v", out)
	}
	if _, ok := out[""]; ok {
		t.Fatal("empty key should be dropped")
	}
}

func TestNormalizeAttributes_Nil(t *testing.T) {
	out, err := NormalizeAttributes(nil)
	if err != nil || out != nil {
		t.Fatalf("want nil,nil got %#v %v", out, err)
	}
}

func TestNormalizeAttributes_TooMany(t *testing.T) {
	attrs := make(map[string]string, maxAttributeEntries+1)
	for i := 0; i < maxAttributeEntries+1; i++ {
		attrs[fmt.Sprintf("k%d", i)] = "x"
	}
	if _, err := NormalizeAttributes(attrs); err == nil {
		t.Fatal("want error for too many entries")
	}
}

func TestProductMergeUpdate_Attributes(t *testing.T) {
	existing := Product{
		ID: "BOT-001", Name: "Cassette",
		Attributes: map[string]string{"condition": "good"},
	}
	merged := existing.MergeUpdate(Product{Style: "evening"})
	if merged.Attributes["condition"] != "good" {
		t.Fatalf("attributes wiped: %#v", merged.Attributes)
	}
	merged = existing.MergeUpdate(Product{Attributes: map[string]string{"condition": "excellent", "care": "dry"}})
	if merged.Attributes["condition"] != "excellent" || merged.Attributes["care"] != "dry" {
		t.Fatalf("attributes not replaced: %#v", merged.Attributes)
	}
	merged = existing.MergeUpdate(Product{Attributes: map[string]string{}})
	if len(merged.Attributes) != 0 {
		t.Fatalf("want clear via empty map, got %#v", merged.Attributes)
	}
}
