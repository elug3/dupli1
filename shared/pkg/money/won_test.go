package money_test

import (
	"encoding/json"
	"testing"

	"github.com/elug3/dupli1/shared/pkg/money"
)

func TestWonUnmarshalWholeNumbers(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want money.Won
	}{
		{"1000", 1000},
		{"1000.0", 1000},
		{"1e3", 1000},
		{"1.2E6", 1200000},
		{"3200000.000", 3200000},
		{"0", 0},
		{"-5", -5},
		{"-0.0", 0},
		{"9223372036854775807", 9223372036854775807},
	} {
		var body struct {
			Price money.Won `json:"price"`
		}
		if err := json.Unmarshal([]byte(`{"price":`+tc.in+`}`), &body); err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if body.Price != tc.want {
			t.Errorf("%s = %d, want %d", tc.in, body.Price, tc.want)
		}
	}
}

func TestWonUnmarshalRefusesFractionsAndRange(t *testing.T) {
	for _, in := range []string{"10.5", "0.1", "-1.25", "1e-3", "9223372036854775808", "1e300", "9007199254740993.0"} {
		var body struct {
			Price money.Won `json:"price"`
		}
		err := json.Unmarshal([]byte(`{"price":`+in+`}`), &body)
		ute, ok := money.IsWonNumberError(err)
		if !ok {
			t.Errorf("%s: want a Won number error, got %v", in, err)
			continue
		}
		if ute.Field != "price" {
			t.Errorf("%s: Field = %q, want price", in, ute.Field)
		}
	}
}

func TestWonUnmarshalNonNumbers(t *testing.T) {
	for _, in := range []string{`"1000"`, "true", "[1]", `{"a":1}`} {
		var body struct {
			Price money.Won `json:"price"`
		}
		err := json.Unmarshal([]byte(`{"price":`+in+`}`), &body)
		if err == nil {
			t.Errorf("%s: accepted", in)
			continue
		}
		if _, ok := money.IsWonNumberError(err); ok {
			t.Errorf("%s: a non-number must not read as a refused number", in)
		}
	}
}

func TestWonUnmarshalNullAndPointers(t *testing.T) {
	body := struct {
		Price    money.Won  `json:"price"`
		Override *money.Won `json:"override"`
	}{Price: 7}
	if err := json.Unmarshal([]byte(`{"price":null,"override":null}`), &body); err != nil {
		t.Fatal(err)
	}
	if body.Price != 7 || body.Override != nil {
		t.Fatalf("null: price=%d override=%v, want 7 and nil", body.Price, body.Override)
	}
	if err := json.Unmarshal([]byte(`{"override":5.0}`), &body); err != nil {
		t.Fatal(err)
	}
	if body.Override == nil || *body.Override != 5 {
		t.Fatalf("override = %v, want 5", body.Override)
	}
}

func TestWonUnmarshalNestedFieldPath(t *testing.T) {
	var body struct {
		Variants []struct {
			Override *money.Won `json:"priceOverride"`
		} `json:"variants"`
	}
	err := json.Unmarshal([]byte(`{"variants":[{"priceOverride":1},{"priceOverride":1.5}]}`), &body)
	ute, ok := money.IsWonNumberError(err)
	if !ok || ute.Field != "variants.priceOverride" {
		t.Fatalf("err = %v, want a Won number error on variants.priceOverride", err)
	}
}

func TestWonMarshalsAsInteger(t *testing.T) {
	override := money.Won(450000)
	got, err := json.Marshal(struct {
		Price    money.Won  `json:"price"`
		Official money.Won  `json:"officialPrice,omitempty"`
		Override *money.Won `json:"priceOverride,omitempty"`
	}{Price: 3200000, Override: &override})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"price":3200000,"priceOverride":450000}` {
		t.Fatalf("got %s", got)
	}
}
