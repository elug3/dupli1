package domain_test

import (
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func validShippingAddress() domain.ShippingAddress {
	return domain.ShippingAddress{
		PostalCode:   "06194",
		AddressLine1: "테헤란로 78길 14-12",
		AddressLine2: "9층",
		City:         "강남구",
		Province:     "서울특별시",
	}
}

func TestValidateFulfillmentSnapshot(t *testing.T) {
	snap, err := domain.ValidateFulfillmentSnapshot(
		"윤라희", "010-4112-5167",
		validShippingAddress(),
		"addr_000001",
	)
	if err != nil {
		t.Fatal(err)
	}
	if snap.RecipientPhone != "01041125167" || snap.SourceAddressID != "addr_000001" {
		t.Fatalf("snapshot: %+v", snap)
	}
	if snap.ShippingAddress.AddressLine2 != "9층" {
		t.Fatalf("address line2 = %q, want 9층", snap.ShippingAddress.AddressLine2)
	}
}

func TestValidateFulfillmentSnapshot_RejectsInvalidInput(t *testing.T) {
	addr := validShippingAddress()
	tests := []struct {
		name  string
		apply func() (string, string, domain.ShippingAddress, string)
	}{
		{
			name: "empty recipient name",
			apply: func() (string, string, domain.ShippingAddress, string) {
				return "", "01041125167", addr, ""
			},
		},
		{
			name: "recipient name too long",
			apply: func() (string, string, domain.ShippingAddress, string) {
				return strings.Repeat("가", 51), "01041125167", addr, ""
			},
		},
		{
			name: "invalid phone",
			apply: func() (string, string, domain.ShippingAddress, string) {
				return "윤라희", "12345", addr, ""
			},
		},
		{
			name: "invalid postal code",
			apply: func() (string, string, domain.ShippingAddress, string) {
				bad := addr
				bad.PostalCode = "0619"
				return "윤라희", "01041125167", bad, ""
			},
		},
		{
			name: "empty address line1",
			apply: func() (string, string, domain.ShippingAddress, string) {
				bad := addr
				bad.AddressLine1 = ""
				return "윤라희", "01041125167", bad, ""
			},
		},
		{
			name: "address line2 too long",
			apply: func() (string, string, domain.ShippingAddress, string) {
				bad := addr
				bad.AddressLine2 = strings.Repeat("층", 201)
				return "윤라희", "01041125167", bad, ""
			},
		},
		{
			name: "empty city",
			apply: func() (string, string, domain.ShippingAddress, string) {
				bad := addr
				bad.City = ""
				return "윤라희", "01041125167", bad, ""
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, phone, address, sourceID := tc.apply()
			_, err := domain.ValidateFulfillmentSnapshot(name, phone, address, sourceID)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if err != domain.ErrInvalidFulfillment {
				t.Fatalf("error = %v, want ErrInvalidFulfillment", err)
			}
		})
	}
}

func TestValidateFulfillmentSnapshot_AllowsEmptyOptionalLine2(t *testing.T) {
	addr := validShippingAddress()
	addr.AddressLine2 = ""
	snap, err := domain.ValidateFulfillmentSnapshot("윤라희", "01041125167", addr, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ShippingAddress.AddressLine2 != "" {
		t.Fatalf("address line2 = %q, want empty", snap.ShippingAddress.AddressLine2)
	}
}

func TestOrderApplyFulfillment(t *testing.T) {
	snap, err := domain.ValidateFulfillmentSnapshot(
		"윤라희", "01041125167", validShippingAddress(), "addr_000001",
	)
	if err != nil {
		t.Fatal(err)
	}

	order := &domain.Order{ID: "ord-1"}
	if err := order.ApplyFulfillment(snap); err != nil {
		t.Fatal(err)
	}
	if order.RecipientName != "윤라희" || order.RecipientPhone != "01041125167" {
		t.Fatalf("order fulfillment: %+v", order)
	}
	if order.SourceAddressID != "addr_000001" {
		t.Fatalf("source address id = %q, want addr_000001", order.SourceAddressID)
	}
}

func TestOrderApplyFulfillment_RejectsNil(t *testing.T) {
	order := &domain.Order{ID: "ord-1"}
	if err := order.ApplyFulfillment(nil); err != domain.ErrInvalidFulfillment {
		t.Fatalf("ApplyFulfillment(nil) error = %v, want ErrInvalidFulfillment", err)
	}
	var nilOrder *domain.Order
	if err := nilOrder.ApplyFulfillment(snapForApplyTest(t)); err != domain.ErrInvalidFulfillment {
		t.Fatalf("nil order ApplyFulfillment error = %v, want ErrInvalidFulfillment", err)
	}
}

func snapForApplyTest(t *testing.T) *domain.FulfillmentSnapshot {
	t.Helper()
	snap, err := domain.ValidateFulfillmentSnapshot("윤라희", "01041125167", validShippingAddress(), "")
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
