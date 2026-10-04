package domain

import (
	"fmt"
	"math"
	"strings"
)

// SizeChartRow is one size's garment measurements in centimetres, shown as
// the storefront size guide. Size is a size master code (e.g. "M"). A zero
// measurement means "not given" and is left out of the guide.
type SizeChartRow struct {
	Size       string  `json:"size"`
	ChestCm    float64 `json:"chestCm,omitempty"`
	LengthCm   float64 `json:"lengthCm,omitempty"`
	ShoulderCm float64 `json:"shoulderCm,omitempty"`
	SleeveCm   float64 `json:"sleeveCm,omitempty"`
}

const (
	maxSizeChartRows = 20
	// maxSizeChartCm bounds a single measurement; anything longer is a typo
	// (mm entered as cm, an extra digit).
	maxSizeChartCm = 300
)

// NormalizeSizeChart validates a product's size chart for its category:
// sizes are upper-cased, unique and, when the category lists its sizes, one
// of them; each row gives at least one measurement, every measurement is
// between 0 and 300 cm and is rounded to the nearest 0.5 cm. An empty chart
// returns nil.
func NormalizeSizeChart(category string, rows []SizeChartRow) ([]SizeChartRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) > maxSizeChartRows {
		return nil, fmt.Errorf("size chart: at most %d rows", maxSizeChartRows)
	}
	cat, known := LookupCategory(category)
	if !known {
		cat, _ = LookupCategory(DefaultCategory)
	}
	seen := make(map[string]bool, len(rows))
	out := make([]SizeChartRow, 0, len(rows))
	for i, r := range rows {
		r.Size = strings.ToUpper(strings.TrimSpace(r.Size))
		if r.Size == "" {
			return nil, fmt.Errorf("size chart row %d: size is required", i+1)
		}
		if seen[r.Size] {
			return nil, fmt.Errorf("size chart: size %s appears twice", r.Size)
		}
		seen[r.Size] = true
		if !cat.AllowsSize(r.Size) {
			return nil, fmt.Errorf("size chart: size %s is not a %s size", r.Size, cat.Code)
		}
		given := false
		for _, m := range []struct {
			name string
			v    *float64
		}{
			{"chestCm", &r.ChestCm},
			{"lengthCm", &r.LengthCm},
			{"shoulderCm", &r.ShoulderCm},
			{"sleeveCm", &r.SleeveCm},
		} {
			if math.IsNaN(*m.v) || *m.v < 0 || *m.v > maxSizeChartCm {
				return nil, fmt.Errorf("size chart %s: %s must be between 0 and %d cm", r.Size, m.name, maxSizeChartCm)
			}
			*m.v = math.Round(*m.v*2) / 2
			if *m.v > 0 {
				given = true
			}
		}
		if !given {
			return nil, fmt.Errorf("size chart %s: give at least one measurement", r.Size)
		}
		out = append(out, r)
	}
	return out, nil
}
