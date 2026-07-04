package handler

// API route paths (v1).
const (
	RouteHealth        = "/api/v1/products/health"
	RouteProducts      = "/api/v1/products"
	RoutePublicProduct = "/api/v1/products/{id}"
	RouteProductByID   = "/api/v1/products/{id}"
	RouteProductImages = "/api/v1/products/{id}/images"
	RouteRedeemCoupon  = "/api/v1/coupons/redeem"
	RouteCoupons       = "/api/v1/coupons"
	RouteCouponByCode  = "/api/v1/coupons/{code}"
)
