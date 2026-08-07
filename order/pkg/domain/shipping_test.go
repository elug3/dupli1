package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func TestValidateFulfillmentSnapshot(t *testing.T) {
	snap, err := domain.ValidateFulfillmentSnapshot(
		"윤라희", "010-4112-5167",
		domain.ShippingAddress{
			PostalCode:   "06194",
			AddressLine1: "테헤란로 78길 14-12",
			AddressLine2: "9층",
			City:         "강남구",
			Province:     "서울특별시",
		},
		"addr_000001",
	)
	if err != nil {
		t.Fatal(err)
	}
	if snap.RecipientPhone != "01041125167" || snap.SourceAddressID != "addr_000001" {
		t.Fatalf("snapshot: %+v", snap)
	}
}
