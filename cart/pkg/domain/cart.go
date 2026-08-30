package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidCart     = errors.New("invalid cart")
	ErrInvalidCartItem = errors.New("invalid cart item")
)

// ReasonVariantNotFound is returned when a cart line cannot be resolved to an
// active, sellable product variant.
const ReasonVariantNotFound = "variant_not_found"

// ReasonInsufficientStock is returned when requested quantity exceeds available.
const ReasonInsufficientStock = "insufficient_stock"

type StoredItem struct {
	SkuID    string
	SKU      string
	Quantity int
}

// UnavailableItem identifies a cart/checkout line that cannot be purchased.
type UnavailableItem struct {
	SkuID  string `json:"sku_id,omitempty"`
	SKU    string `json:"sku,omitempty"`
	Reason string `json:"reason"`
}

type CartItem struct {
	SkuID          string `json:"sku_id,omitempty"`
	SKU            string `json:"sku"`
	ProductID      string `json:"product_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"` // whole KRW won (enriched from product)
	Color          string `json:"color,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	AvailableQty   int    `json:"available_qty,omitempty"`
	// Available is false when enrichment failed (variant not sellable).
	// Omitted when true/unknown so existing clients stay compatible.
	Available *bool `json:"available,omitempty"`
}

type Cart struct {
	CustomerID       string            `json:"customer_id"`
	Items            []CartItem        `json:"items"`
	UnavailableItems []UnavailableItem `json:"unavailable_items,omitempty"`
	SubtotalCents    int64             `json:"subtotal_cents"` // whole KRW won; excludes unavailable lines
	UpdatedAt        time.Time         `json:"updated_at"`
}

func NormalizeSKU(sku string) string {
	return strings.ToUpper(strings.TrimSpace(sku))
}

func ValidateStoredItem(item StoredItem) error {
	if (NormalizeSKU(item.SKU) == "" && strings.TrimSpace(item.SkuID) == "") || item.Quantity <= 0 {
		return ErrInvalidCartItem
	}
	return nil
}

func ValidateStoredItems(items []StoredItem) error {
	for _, item := range items {
		if err := ValidateStoredItem(item); err != nil {
			return err
		}
	}
	return nil
}
