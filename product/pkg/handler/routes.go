package handler

import "net/http"

// API route paths (v1).
//
// Convention: /api/v1/{service_name}/... where service_name is the owning
// microservice (products for this package). Legacy top-level prefixes
// (variants, coupons, catalog, inventory) remain registered as aliases —
// see Legacy* constants and docs/TODO.md.
//
// Note: Go's ServeMux rejects sibling patterns like
// /products/variants/{sku} and /products/{id}/recommendations (overlapping
// matches). Canonical wildcards therefore use an extra literal segment
// (by-sku, by-code, items).
const (
	RouteHealth                 = "/api/v1/products/health"
	RouteSettings               = "/api/v1/products/settings"
	RouteProducts               = "/api/v1/products"
	RoutePublicProduct          = "/api/v1/products/{id}"
	RouteProductByID            = "/api/v1/products/{id}"
	RouteProductImages          = "/api/v1/products/{id}/images"
	RouteVariants               = "/api/v1/products/{id}/variants"
	RouteVariantBySKU           = "/api/v1/products/{id}/variants/{sku}"
	RouteVariantImages          = "/api/v1/products/{id}/variants/{sku}/images"
	RouteProductRecommendations = "/api/v1/products/{id}/recommendations"
	RouteProductWishlist        = "/api/v1/products/{id}/wishlist"
	RouteWishlist               = "/api/v1/products/wishlist"

	// Public variant lookups (service-prefixed).
	RoutePublicVariants       = "/api/v1/products/variants"
	RoutePublicVariant        = "/api/v1/products/variants/by-sku/{sku}"
	RoutePublicVariantBySkuID = "/api/v1/products/variants/by-sku-id/{skuId}"

	// Coupons under products.
	RouteRedeemCoupon = "/api/v1/products/coupons/redeem"
	RouteCoupons      = "/api/v1/products/coupons"
	RouteCouponByCode = "/api/v1/products/coupons/by-code/{code}"

	// Inventory under products (merged former inventory service).
	RouteInventoryHealth             = "/api/v1/products/inventory/health"
	RouteInventorySettings           = "/api/v1/products/inventory/settings"
	RouteInventoryItem               = "/api/v1/products/inventory/items/{sku}"
	RouteInventoryAdjust             = "/api/v1/products/inventory/items/{sku}/adjust"
	RouteInventoryItemBySkuID        = "/api/v1/products/inventory/items/by-sku-id/{skuId}"
	RouteInventoryAdjustBySkuID      = "/api/v1/products/inventory/items/by-sku-id/{skuId}/adjust"
	RouteInventoryReservations       = "/api/v1/products/inventory/reservations"
	RouteInventoryReservationCommit  = "/api/v1/products/inventory/reservations/{id}/commit"
	RouteInventoryReservationRelease = "/api/v1/products/inventory/reservations/{id}/release"

	// Catalog master data under products.
	RouteCatalogBrands        = "/api/v1/products/catalog/brands"
	RouteCatalogBrandByCode   = "/api/v1/products/catalog/brands/{code}"
	RouteCatalogStyles        = "/api/v1/products/catalog/brands/{code}/styles"
	RouteCatalogStyleByCode   = "/api/v1/products/catalog/brands/{code}/styles/{styleCode}"
	RouteCatalogColors        = "/api/v1/products/catalog/colors"
	RouteCatalogColorByCode   = "/api/v1/products/catalog/colors/{code}"
	RouteCatalogSizes         = "/api/v1/products/catalog/sizes"
	RouteCatalogSizeByCode    = "/api/v1/products/catalog/sizes/{code}"
	RouteCatalogEditions      = "/api/v1/products/catalog/editions"
	RouteCatalogEditionByCode = "/api/v1/products/catalog/editions/{code}"

	// Legacy aliases — same handlers; remove after clients migrate.
	LegacyRoutePublicVariant               = "/api/v1/variants/{sku}"
	LegacyRoutePublicVariantBySkuID        = "/api/v1/variants/by-sku-id/{skuId}"
	LegacyRouteRedeemCoupon                = "/api/v1/coupons/redeem"
	LegacyRouteCoupons                     = "/api/v1/coupons"
	LegacyRouteCouponByCode                = "/api/v1/coupons/{code}"
	LegacyRouteInventoryHealth             = "/api/v1/inventory/health"
	LegacyRouteInventorySettings           = "/api/v1/inventory/settings"
	LegacyRouteInventoryItem               = "/api/v1/inventory/{sku}"
	LegacyRouteInventoryAdjust             = "/api/v1/inventory/{sku}/adjust"
	LegacyRouteInventoryItemBySkuID        = "/api/v1/inventory/by-sku-id/{skuId}"
	LegacyRouteInventoryAdjustBySkuID      = "/api/v1/inventory/by-sku-id/{skuId}/adjust"
	LegacyRouteInventoryReservations       = "/api/v1/inventory/reservations"
	LegacyRouteInventoryReservationCommit  = "/api/v1/inventory/reservations/{id}/commit"
	LegacyRouteInventoryReservationRelease = "/api/v1/inventory/reservations/{id}/release"
	LegacyRouteCatalogBrands               = "/api/v1/catalog/brands"
	LegacyRouteCatalogBrandByCode          = "/api/v1/catalog/brands/{code}"
	LegacyRouteCatalogStyles               = "/api/v1/catalog/brands/{code}/styles"
	LegacyRouteCatalogStyleByCode          = "/api/v1/catalog/brands/{code}/styles/{styleCode}"
	LegacyRouteCatalogColors               = "/api/v1/catalog/colors"
	LegacyRouteCatalogColorByCode          = "/api/v1/catalog/colors/{code}"
	LegacyRouteCatalogSizes                = "/api/v1/catalog/sizes"
	LegacyRouteCatalogSizeByCode           = "/api/v1/catalog/sizes/{code}"
	LegacyRouteCatalogEditions             = "/api/v1/catalog/editions"
	LegacyRouteCatalogEditionByCode        = "/api/v1/catalog/editions/{code}"
)

// Mount registers method+path and optional legacy aliases onto the same handler.
func Mount(mux *http.ServeMux, method, path string, h http.Handler, legacy ...string) {
	mux.Handle(method+" "+path, h)
	for _, p := range legacy {
		mux.Handle(method+" "+p, h)
	}
}

// MountFunc is Mount for HandlerFunc registrations.
func MountFunc(mux *http.ServeMux, method, path string, h func(http.ResponseWriter, *http.Request), legacy ...string) {
	mux.HandleFunc(method+" "+path, h)
	for _, p := range legacy {
		mux.HandleFunc(method+" "+p, h)
	}
}
