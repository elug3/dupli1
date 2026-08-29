package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Carrier codes for Korean domestic parcel carriers used at ship time.
const (
	CarrierCJ     = "cj"
	CarrierHanjin = "hanjin"
	CarrierLotte  = "lotte"
	CarrierLogen  = "logen"
	CarrierEPost  = "epost"
	CarrierOther  = "other"
)

// ValidCarriers is the fixed set accepted by POST /orders/{id}/ship.
var ValidCarriers = map[string]struct{}{
	CarrierCJ:     {},
	CarrierHanjin: {},
	CarrierLotte:  {},
	CarrierLogen:  {},
	CarrierEPost:  {},
	CarrierOther:  {},
}

// ShipmentTracking is required when moving an order to in_transit.
type ShipmentTracking struct {
	Carrier        string
	TrackingNumber string
	// CarrierNote is required when Carrier is "other" (free-text carrier name).
	CarrierNote string
}

// NormalizeShipmentTracking validates and normalizes ship tracking fields.
// Tracking is required on every ship.
func NormalizeShipmentTracking(carrier, trackingNumber, carrierNote string) (ShipmentTracking, error) {
	carrier = strings.ToLower(strings.TrimSpace(carrier))
	trackingNumber = strings.TrimSpace(trackingNumber)
	carrierNote = strings.TrimSpace(carrierNote)

	if carrier == "" {
		return ShipmentTracking{}, fmt.Errorf("%w: carrier is required", ErrInvalidShipment)
	}
	if _, ok := ValidCarriers[carrier]; !ok {
		return ShipmentTracking{}, fmt.Errorf("%w: unknown carrier %q", ErrInvalidShipment, carrier)
	}
	if trackingNumber == "" {
		return ShipmentTracking{}, fmt.Errorf("%w: tracking_number is required", ErrInvalidShipment)
	}
	if utf8.RuneCountInString(trackingNumber) > 64 {
		return ShipmentTracking{}, fmt.Errorf("%w: tracking_number too long", ErrInvalidShipment)
	}
	if carrier == CarrierOther {
		if carrierNote == "" {
			return ShipmentTracking{}, fmt.Errorf("%w: carrier_note is required when carrier is other", ErrInvalidShipment)
		}
		if utf8.RuneCountInString(carrierNote) > 120 {
			return ShipmentTracking{}, fmt.Errorf("%w: carrier_note too long", ErrInvalidShipment)
		}
	} else {
		carrierNote = ""
	}

	return ShipmentTracking{
		Carrier:        carrier,
		TrackingNumber: trackingNumber,
		CarrierNote:    carrierNote,
	}, nil
}
