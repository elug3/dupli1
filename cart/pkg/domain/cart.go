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

type StoredItem struct {
	SkuID    string
	SKU      string
	Quantity int
}

type CartItem struct {
	SkuID          string `json:"sku_id,omitempty"`
	SKU            string `json:"sku"`
	ProductID      string `json:"product_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Color          string `json:"color,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	AvailableQty   int    `json:"available_qty,omitempty"`
}

type Cart struct {
	CustomerID    string     `json:"customer_id"`
	Items         []CartItem `json:"items"`
	SubtotalCents int64      `json:"subtotal_cents"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
