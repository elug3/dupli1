package domain

import (
	"math"
	"sort"
	"strings"
)

const (
	recommendWeightBrand    = 5.0
	recommendWeightMaterial = 3.0
	recommendWeightTag      = 2.0
	recommendWeightPrice    = 2.0
	recommendMaxTagOverlap  = 3
	recommendPriceBand      = 0.30 // ±30% of seed PriceFrom
)

// ScoreRecommendation returns the deterministic related-product score for a candidate.
// Weights match docs/product-views-recommendations-plan.md.
func ScoreRecommendation(seed, candidate Product) float64 {
	var score float64

	if brandMatch(seed, candidate) {
		score += recommendWeightBrand
	}
	if seed.Material != "" && seed.Material == candidate.Material {
		score += recommendWeightMaterial
	}
	overlap := tagOverlap(seed.Tags, candidate.Tags)
	if overlap > recommendMaxTagOverlap {
		overlap = recommendMaxTagOverlap
	}
	score += recommendWeightTag * float64(overlap)
	if inPriceBand(seed.PriceFrom, candidate.PriceFrom) {
		score += recommendWeightPrice
	}
	score += math.Log10(float64(candidate.ViewCount) + 1)
	return score
}

// RankRecommendations filters, scores, and returns the top limit candidates for seed.
// Excludes the seed id and non-active products. Same-category only when seed has a category.
func RankRecommendations(seed Product, candidates []Product, limit int) []Product {
	if limit <= 0 {
		return nil
	}
	type scored struct {
		p     Product
		score float64
	}
	ranked := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if c.ID == "" || c.ID == seed.ID {
			continue
		}
		if c.Status != "" && c.Status != "active" {
			continue
		}
		if seed.Category != "" && c.Category != seed.Category {
			continue
		}
		ranked = append(ranked, scored{p: c, score: ScoreRecommendation(seed, c)})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].p.ViewCount != ranked[j].p.ViewCount {
			return ranked[i].p.ViewCount > ranked[j].p.ViewCount
		}
		return ranked[i].p.ID < ranked[j].p.ID
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]Product, len(ranked))
	for i, r := range ranked {
		out[i] = r.p
	}
	return out
}

func brandMatch(seed, candidate Product) bool {
	if seed.BrandCode != "" && candidate.BrandCode != "" {
		return seed.BrandCode == candidate.BrandCode
	}
	if seed.Brand == "" || candidate.Brand == "" {
		return false
	}
	return strings.EqualFold(seed.Brand, candidate.Brand)
}

func tagOverlap(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, t := range a {
		t = strings.TrimSpace(t)
		if t != "" {
			set[t] = struct{}{}
		}
	}
	n := 0
	for _, t := range b {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := set[t]; ok {
			n++
		}
	}
	return n
}

func inPriceBand(seedPrice, candidatePrice float64) bool {
	if seedPrice <= 0 || candidatePrice <= 0 {
		return false
	}
	lo := seedPrice * (1 - recommendPriceBand)
	hi := seedPrice * (1 + recommendPriceBand)
	return candidatePrice >= lo && candidatePrice <= hi
}
