package domain

import "testing"

func TestProductChanges_SizeChartMeasurement(t *testing.T) {
	before := Product{SizeChart: []SizeChartRow{{Size: "M", ChestCm: 52, LengthCm: 70}}}
	after := Product{SizeChart: []SizeChartRow{{Size: "M", ChestCm: 52.5, LengthCm: 70}}}
	got := ProductChanges(before, after)
	if len(got) != 1 || got[0].Field != "sizeChart" || got[0].Before != "M 52/70/-/-" || got[0].After != "M 52.5/70/-/-" {
		t.Fatalf("changes = %+v", got)
	}
}

func TestProductChanges_RemovedAttributeAndOfficialPrice(t *testing.T) {
	before := Product{OfficialPrice: 3000, Attributes: map[string]string{"care": "dry"}}
	after := Product{}
	got := ProductChanges(before, after)
	if len(got) != 2 {
		t.Fatalf("changes = %+v", got)
	}
	if got[0] != (FieldChange{Field: "officialPrice", Before: "3000", After: ""}) {
		t.Fatalf("official price change = %+v", got[0])
	}
	if got[1] != (FieldChange{Field: "attributes.care", Before: "dry", After: ""}) {
		t.Fatalf("attribute change = %+v", got[1])
	}
}
