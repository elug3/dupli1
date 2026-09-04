package httpproduct

import (
	"context"
	"errors"
	"net/http"

	"github.com/elug3/dupli1/cart/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/productclient"
)

// Client resolves product variants for cart display. Delegates the actual
// HTTP fetch to shared/pkg/productclient and maps its superset Variant into
// cart's own ports.VariantInfo (which carries Color, not ProductName).
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
		Color:          v.Color,
		UnitPriceCents: v.UnitPriceCents,
		ImageURL:       v.ImageURL,
	}, nil
}
