package handler

// API route paths (v1).
const (
	RouteHealth         = "/api/v1/products/health"
	RouteSearchBags     = "/api/v1/products/bags"
	RoutePublicProduct  = "/api/v1/products/{id}"
	RouteRedeemCoupon   = "/api/v1/coupons/redeem"
	RouteProducts       = "/api/v1/products"
	RouteManageProduct  = "/api/v1/products/{id}/manage"
	RouteProductByID    = "/api/v1/products/{id}"
	RouteProductImage   = "/api/v1/products/{id}/image"
	RouteCoupons        = "/api/v1/coupons"
	RouteCouponByCode   = "/api/v1/coupons/{code}"
)
