package domain

import "testing"

func TestScoreRecommendationWeights(t *testing.T) {
	seed := Product{
		ID: "A", BrandCode: "BOT", Brand: "Bottega", Material: "leather",
		Tags: []string{"tote", "daily"}, PriceFrom: 100, Category: "bags",
	}
	sameBrand := Product{ID: "B", BrandCode: "BOT", Brand: "Bottega", Status: "active", Category: "bags", PriceFrom: 100}
	sameBrandMaterial := Product{
		ID: "C", BrandCode: "BOT", Material: "leather", Status: "active", Category: "bags", PriceFrom: 100,
	}
	if ScoreRecommendation(seed, sameBrandMaterial) <= ScoreRecommendation(seed, sameBrand) {
		t.Fatal("same brand+material should score higher than brand alone")
	}
}

func TestRankRecommendationsOrderAndFilters(t *testing.T) {
	seed := Product{
		ID: "SEED", BrandCode: "BOT", Material: "leather", Category: "bags",
		Tags: []string{"tote"}, PriceFrom: 200, Status: "active",
	}
	candidates := []Product{
		{ID: "SEED", BrandCode: "BOT", Category: "bags", Status: "active"}, // self
		{ID: "DRAFT", BrandCode: "BOT", Category: "bags", Status: "draft"},
		{ID: "SHOES", BrandCode: "BOT", Category: "shoes", Status: "active"},
		{ID: "LOW", Brand: "Other", Category: "bags", Status: "active", ViewCount: 1000},
		{ID: "HIGH", BrandCode: "BOT", Material: "leather", Tags: []string{"tote"}, Category: "bags", Status: "active", PriceFrom: 200, ViewCount: 1},
		{ID: "MID", BrandCode: "BOT", Category: "bags", Status: "active", PriceFrom: 200, ViewCount: 50},
	}
	got := RankRecommendations(seed, candidates, 3)
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	if got[0].ID != "HIGH" {
		t.Fatalf("want HIGH first, got %s", got[0].ID)
	}
	if got[1].ID != "MID" {
		t.Fatalf("want MID second, got %s", got[1].ID)
	}
	for _, p := range got {
		if p.ID == "SEED" || p.ID == "DRAFT" || p.ID == "SHOES" {
			t.Fatalf("unexpected id %s in results", p.ID)
		}
	}
}

func TestRankRecommendationsEmpty(t *testing.T) {
	seed := Product{ID: "ONLY", Category: "bags", Status: "active"}
	got := RankRecommendations(seed, []Product{seed}, 8)
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}
