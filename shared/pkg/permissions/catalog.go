// Package permissions implements Dupli1 fine-grained authorization:
// permission constants, wildcard evaluation, legacy role migration, and bundles.
//
// See docs/permissions.md for the authoritative specification.
package permissions

// Wildcard permission tokens.
const (
	All        = "*"
	AdminAll   = "admin.*"
	ProductAll = "product.*"
	CouponAll  = "coupon.*"
	UserAll    = "user.*"
)

// User administration permissions (auth service).
const (
	UserCreate            = "user.create"
	UserRead              = "user.read"
	UserPermissionsUpdate = "user.permissions.update"
	UserPasswordUpdate    = "user.password.update"
	UserStatusUpdate      = "user.status.update"
)

// Product catalog permissions (product service).
const (
	ProductCreate         = "product.create"
	ProductUpdate         = "product.update"
	ProductDelete         = "product.delete"
	ProductRead           = "product.read"
	ProductVariantCreate  = "product.variant.create"
	ProductVariantUpdate  = "product.variant.update"
	ProductVariantDelete  = "product.variant.delete"
	ProductImageUpload    = "product.image.upload"
)

// Coupon permissions (product service).
const (
	CouponRead   = "coupon.read"
	CouponCreate = "coupon.create"
	CouponUpdate = "coupon.update"
	CouponDelete = "coupon.delete"
)

// Inventory permissions (inventory service).
const (
	InventoryStockRead        = "inventory.stock.read"
	InventoryStockWrite       = "inventory.stock.write"
	InventoryReservationManage = "inventory.reservation.manage"
)

// Order permissions (order service).
const (
	OrderCreate       = "order.create"
	OrderReadAll      = "order.read.all"
	OrderShip         = "order.ship"
	OrderStatusUpdate = "order.status.update"
)

// Cart permissions (cart service).
const (
	CartRead = "cart.read"
)

// Payment permissions (payment service).
const (
	PaymentCreate   = "payment.create"
	PaymentReadAll  = "payment.read.all"
)

// All lists every concrete (non-wildcard) permission in the catalog.
var Catalog = []string{
	UserCreate,
	UserRead,
	UserPermissionsUpdate,
	UserPasswordUpdate,
	UserStatusUpdate,
	ProductCreate,
	ProductUpdate,
	ProductDelete,
	ProductRead,
	ProductVariantCreate,
	ProductVariantUpdate,
	ProductVariantDelete,
	ProductImageUpload,
	CouponRead,
	CouponCreate,
	CouponUpdate,
	CouponDelete,
	InventoryStockRead,
	InventoryStockWrite,
	InventoryReservationManage,
	OrderCreate,
	OrderReadAll,
	OrderShip,
	OrderStatusUpdate,
	CartRead,
	PaymentCreate,
	PaymentReadAll,
}

// known is the set of concrete permissions for O(1) lookup.
var known map[string]struct{}

func init() {
	known = make(map[string]struct{}, len(Catalog))
	for _, p := range Catalog {
		known[p] = struct{}{}
	}
}

// IsKnown reports whether perm is a concrete catalog permission.
func IsKnown(perm string) bool {
	_, ok := known[perm]
	return ok
}
