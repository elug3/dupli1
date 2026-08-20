package service

import (
	"github.com/elug3/dupli1/cart/pkg/domain"
	"github.com/elug3/dupli1/cart/pkg/ports"
)

// UnavailableVariantsError is returned when one or more cart lines fail variant
// resolution. Error() preserves the historical "variant not found" string;
// UnavailableItems carries every failed line for the storefront.
type UnavailableVariantsError struct {
	Items []domain.UnavailableItem
}

func (e *UnavailableVariantsError) Error() string {
	return ports.ErrVariantNotFound.Error()
}

func (e *UnavailableVariantsError) Unwrap() error {
	return ports.ErrVariantNotFound
}

func unavailableFromStored(item domain.StoredItem) domain.UnavailableItem {
	return domain.UnavailableItem{
		SkuID:  item.SkuID,
		SKU:    item.SKU,
		Reason: domain.ReasonVariantNotFound,
	}
}
