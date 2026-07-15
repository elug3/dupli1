package handler

// API route paths (v1).
const (
	RouteHealth               = "/api/v1/products/health"
	RouteProducts             = "/api/v1/products"
	RoutePublicProduct        = "/api/v1/products/{id}"
	RouteProductByID          = "/api/v1/products/{id}"
	RouteProductImages        = "/api/v1/products/{id}/images"
	RouteVariants             = "/api/v1/products/{id}/variants"
	RouteVariantBySKU         = "/api/v1/products/{id}/variants/{sku}"
	RouteVariantImages        = "/api/v1/products/{id}/variants/{sku}/images"
	RoutePublicVariant        = "/api/v1/variants/{sku}"
	RoutePublicVariantBySkuID = "/api/v1/variants/by-sku-id/{skuId}"
	RouteRedeemCoupon         = "/api/v1/coupons/redeem"
	RouteCoupons              = "/api/v1/coupons"
	RouteCouponByCode         = "/api/v1/coupons/{code}"

	// Inventory (merged in from the standalone inventory service).
	RouteInventoryHealth             = "/api/v1/inventory/health"
	RouteInventoryItem               = "/api/v1/inventory/{sku}"
	RouteInventoryAdjust             = "/api/v1/inventory/{sku}/adjust"
	RouteInventoryItemBySkuID        = "/api/v1/inventory/by-sku-id/{skuId}"
	RouteInventoryAdjustBySkuID      = "/api/v1/inventory/by-sku-id/{skuId}/adjust"
	RouteInventoryReservations       = "/api/v1/inventory/reservations"
	RouteInventoryReservationCommit  = "/api/v1/inventory/reservations/{id}/commit"
	RouteInventoryReservationRelease = "/api/v1/inventory/reservations/{id}/release"

	// Catalog master data (code → name dictionaries).
	RouteCatalogBrands       = "/api/v1/catalog/brands"
	RouteCatalogBrandByCode  = "/api/v1/catalog/brands/{code}"
	RouteCatalogStyles       = "/api/v1/catalog/brands/{code}/styles"
	RouteCatalogStyleByCode  = "/api/v1/catalog/brands/{code}/styles/{styleCode}"
	RouteCatalogColors       = "/api/v1/catalog/colors"
	RouteCatalogColorByCode  = "/api/v1/catalog/colors/{code}"
	RouteCatalogSizes        = "/api/v1/catalog/sizes"
	RouteCatalogSizeByCode   = "/api/v1/catalog/sizes/{code}"
	RouteCatalogEditions     = "/api/v1/catalog/editions"
	RouteCatalogEditionByCode = "/api/v1/catalog/editions/{code}"
)
