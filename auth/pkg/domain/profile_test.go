package domain_test

import (
	"strings"
	"testing"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

func TestNormalizeKRPhone(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"010-4112-5167", "01041125167", true},
		{"01041125167", "01041125167", true},
		{"12345", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, err := domain.NormalizeKRPhone(tc.in)
		if tc.ok && err != nil {
			t.Fatalf("NormalizeKRPhone(%q): %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("NormalizeKRPhone(%q): want error", tc.in)
		}
		if got != tc.want {
			t.Fatalf("NormalizeKRPhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizePostalCode(t *testing.T) {
	got, err := domain.NormalizePostalCode("06194")
	if err != nil || got != "06194" {
		t.Fatalf("valid postal: got %q, %v", got, err)
	}
	if _, err := domain.NormalizePostalCode("0619"); err == nil {
		t.Fatal("expected invalid postal")
	}
}

func TestValidateAddressInput(t *testing.T) {
	addr, err := domain.ValidateAddressInput(
		"윤라희", "010-4112-5167", "06194",
		"테헤란로 78길 14-12", "9층", "강남구", "서울특별시",
	)
	if err != nil {
		t.Fatal(err)
	}
	if addr.RecipientPhone != "01041125167" || addr.AddressLine2 != "9층" {
		t.Fatalf("unexpected address: %+v", addr)
	}
}

func TestNormalizePersonName(t *testing.T) {
	got, err := domain.NormalizePersonName("  윤라희  ")
	if err != nil || got != "윤라희" {
		t.Fatalf("valid name: got %q, %v", got, err)
	}
	if _, err := domain.NormalizePersonName(""); err == nil {
		t.Fatal("empty name should fail")
	}
	longName := strings.Repeat("가", 51)
	if _, err := domain.NormalizePersonName(longName); err == nil {
		t.Fatal("name over 50 runes should fail")
	}
}

func TestNormalizeAddressLine(t *testing.T) {
	got, err := domain.NormalizeAddressLine("  테헤란로  ", 200)
	if err != nil || got != "테헤란로" {
		t.Fatalf("valid line: got %q, %v", got, err)
	}
	if _, err := domain.NormalizeAddressLine("", 200); err == nil {
		t.Fatal("empty required line should fail")
	}
	longLine := strings.Repeat("a", 201)
	if _, err := domain.NormalizeAddressLine(longLine, 200); err == nil {
		t.Fatal("line over max runes should fail")
	}
}

func TestNormalizeOptionalLine(t *testing.T) {
	got, err := domain.NormalizeOptionalLine("  9층  ", 200)
	if err != nil || got != "9층" {
		t.Fatalf("valid optional line: got %q, %v", got, err)
	}
	got, err = domain.NormalizeOptionalLine("   ", 200)
	if err != nil || got != "" {
		t.Fatalf("blank optional line: got %q, %v", got, err)
	}
	longLine := strings.Repeat("b", 201)
	if _, err := domain.NormalizeOptionalLine(longLine, 200); err == nil {
		t.Fatal("optional line over max runes should fail")
	}
}

func TestValidateAddressInput_RejectsInvalidFields(t *testing.T) {
	valid := []string{"윤라희", "01041125167", "06194", "테헤란로", "", "강남구", "서울특별시"}
	cases := []struct {
		name string
		idx  int
		val  string
	}{
		{"invalid phone", 1, "12345"},
		{"invalid postal", 2, "0619"},
		{"empty line1", 3, ""},
		{"empty city", 5, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string(nil), valid...)
			args[tc.idx] = tc.val
			if _, err := domain.ValidateAddressInput(args[0], args[1], args[2], args[3], args[4], args[5], args[6]); err == nil {
				t.Fatalf("ValidateAddressInput(%q) should fail", tc.name)
			}
		})
	}
}
