package domain

import (
	"strings"
	"testing"
)

func TestNormalizeSizeChart(t *testing.T) {
	got, err := NormalizeSizeChart("clothing", []SizeChartRow{
		{Size: " m ", ChestCm: 112.2, LengthCm: 70.3},
		{Size: "L", ChestCm: 118},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Size != "M" || got[0].ChestCm != 112 || got[0].LengthCm != 70.5 {
		t.Fatalf("row 0 = %+v, want size M rounded to 0.5 cm", got[0])
	}

	if rows, err := NormalizeSizeChart("clothing", nil); err != nil || rows != nil {
		t.Fatalf("empty chart: rows=%v err=%v", rows, err)
	}

	for name, tc := range map[string]struct {
		category string
		rows     []SizeChartRow
		want     string
	}{
		"blank size":       {"clothing", []SizeChartRow{{ChestCm: 100}}, "size is required"},
		"duplicate size":   {"clothing", []SizeChartRow{{Size: "M", ChestCm: 1}, {Size: "m", ChestCm: 2}}, "appears twice"},
		"bag size":         {"clothing", []SizeChartRow{{Size: "OS", ChestCm: 100}}, "not a clothing size"},
		"no measurement":   {"clothing", []SizeChartRow{{Size: "M"}}, "at least one measurement"},
		"negative":         {"clothing", []SizeChartRow{{Size: "M", ChestCm: -1}}, "between 0 and 300"},
		"millimetres typo": {"clothing", []SizeChartRow{{Size: "M", LengthCm: 700}}, "between 0 and 300"},
	} {
		if _, err := NormalizeSizeChart(tc.category, tc.rows); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}

	// Bags list no sizes, so any size code may carry a chart.
	if _, err := NormalizeSizeChart("bags", []SizeChartRow{{Size: "OS", LengthCm: 30}}); err != nil {
		t.Fatalf("bag chart: %v", err)
	}
}

func TestCheckVariantSize(t *testing.T) {
	if err := CheckVariantSize("clothing", "m"); err != nil {
		t.Fatalf("M jacket: %v", err)
	}
	if err := CheckVariantSize("clothing", "OS"); err == nil || !strings.Contains(err.Error(), "not a clothing size") {
		t.Fatalf("OS jacket: err = %v", err)
	}
	if err := CheckVariantSize("clothing", ""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("sizeless jacket: err = %v", err)
	}
	for _, category := range []string{"bags", "", "handbag"} {
		if err := CheckVariantSize(category, "OS"); err != nil {
			t.Fatalf("%q: bags and legacy categories take any size: %v", category, err)
		}
	}
}
