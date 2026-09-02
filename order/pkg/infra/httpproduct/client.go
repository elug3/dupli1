package httpproduct

import (
	"context"
	"errors"
	"net/http"

	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/productclient"
)

// Client resolves product variants for order pricing/display. Delegates
// the actual HTTP fetch to shared/pkg/productclient and maps its superset
// Variant into order's own ports.VariantInfo (which carries ProductName,
// not Color).
type Client struct {
	inner *productclient.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{inner: productclient.NewClient(baseURL, httpClient)}
}

func (c *Client) GetVariant(ctx context.Context, sku string) (*ports.VariantInfo, error) {
	return toVariantInfo(c.inner.GetVariant(ctx, sku))
}

func (c *Client) GetVariantBySkuID(ctx context.Context, skuID string) (*ports.VariantInfo, error) {
	return toVariantInfo(c.inner.GetVariantBySkuID(ctx, skuID))
}

func toVariantInfo(v *productclient.Variant, err error) (*ports.VariantInfo, error) {
	if err != nil {
		if errors.Is(err, productclient.ErrVariantNotFound) {
			return nil, ports.ErrVariantNotFound
		}
		return nil, err
	}
	return &ports.VariantInfo{
		SkuID:          v.SkuID,
		SKU:            v.SKU,
		ProductID:      v.ProductID,
		UnitPriceCents: v.UnitPriceCents,
		ProductName:    v.ProductName,
		ImageURL:       v.ImageURL,
	}, nil
}
