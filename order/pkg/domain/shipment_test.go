package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func TestNormalizeShipmentTracking(t *testing.T) {
	t.Parallel()

	got, err := domain.NormalizeShipmentTracking("CJ", " 1234567890 ", "")
	if err != nil {
		t.Fatalf("NormalizeShipmentTracking: %v", err)
	}
	if got.Carrier != domain.CarrierCJ || got.TrackingNumber != "1234567890" || got.CarrierNote != "" {
		t.Fatalf("got = %+v", got)
	}

	got, err = domain.NormalizeShipmentTracking("other", "TRK-1", "DHL Express")
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	if got.CarrierNote != "DHL Express" {
		t.Fatalf("carrier_note = %q", got.CarrierNote)
	}

	_, err = domain.NormalizeShipmentTracking("", "1", "")
	if !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("empty carrier err = %v", err)
	}
	_, err = domain.NormalizeShipmentTracking("fedex", "1", "")
	if !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("unknown carrier err = %v", err)
	}
	_, err = domain.NormalizeShipmentTracking("cj", "", "")
	if !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("empty tracking err = %v", err)
	}
	_, err = domain.NormalizeShipmentTracking("other", "1", "")
	if !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("other without note err = %v", err)
	}
	// Non-other carriers drop any note.
	got, err = domain.NormalizeShipmentTracking("hanjin", "ABC", "ignored")
	if err != nil {
		t.Fatalf("hanjin: %v", err)
	}
	if got.CarrierNote != "" {
		t.Fatalf("expected note cleared, got %q", got.CarrierNote)
	}

	_, err = domain.NormalizeShipmentTracking("cj", strings.Repeat("x", 65), "")
	if !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("tracking too long err = %v", err)
	}
	_, err = domain.NormalizeShipmentTracking("other", "TRK-1", strings.Repeat("n", 121))
	if !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("carrier_note too long err = %v", err)
	}
}
