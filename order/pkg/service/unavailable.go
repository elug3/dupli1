package service

import (
	"strings"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

// UnavailableVariantsError is returned when one or more lines fail variant
// resolution. Error() preserves the historical "variant not found" string;
// Items carries every failed line for the storefront.
type UnavailableVariantsError struct {
	Items []domain.UnavailableItem
}

func (e *UnavailableVariantsError) Error() string {
	return ports.ErrVariantNotFound.Error()
}

func (e *UnavailableVariantsError) Unwrap() error {
	return ports.ErrVariantNotFound
}

func unavailableFromOrderItem(item domain.OrderItem) domain.UnavailableItem {
	return domain.UnavailableItem{
		SkuID:  strings.TrimSpace(item.SkuID),
		SKU:    strings.TrimSpace(item.SKU),
		Reason: domain.ReasonVariantNotFound,
	}
}
