package domain_test

import (
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
