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
	// PromotionAll supersedes CouponAll; both are accepted during the rename
	// window (docs/product-promotion-rename.md).
	PromotionAll = "promotion.*"
	CouponAll    = "coupon.*"
	UserAll      = "user.*"
)

// User administration permissions (auth service).
const (
	UserCreate            = "user.create"
	UserRead              = "user.read"
	UserPermissionsUpdate = "user.permissions.update"
	UserPasswordUpdate    = "user.password.update"
	UserStatusUpdate      = "user.status.update"
	UserDelete            = "user.delete"
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
	ProductMasterRead     = "product.master.read"
	ProductMasterWrite    = "product.master.write"
)

// Promotional code permissions (product service).
const (
	PromotionRead   = "promotion.read"
	PromotionCreate = "promotion.create"
	PromotionUpdate = "promotion.update"
	PromotionDelete = "promotion.delete"
	// PromotionRedeem moves the usage ledger (reserve, consume, release). It is
	// service-to-service: order holds it, managers do not need it, and the
	// public redeem/evaluate endpoints require no permission at all.
	PromotionRedeem = "promotion.redeem"
	// PromotionIssue grants and revokes a single-user entitlement. Reading
	// one's own wallet needs no permission — that is ABAC on the token subject.
	PromotionIssue = "promotion.issue"
)

// Deprecated: the coupon.* set is superseded by the promotion.* set above.
// Both are accepted on every promotion route for one release so access tokens
// minted before the rename keep authorizing; drop these once that window
// closes. See docs/product-promotion-rename.md.
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
	PaymentCreate  = "payment.create"
	PaymentReadAll = "payment.read.all"
	PaymentBypass  = "payment.bypass"
	PaymentCancel  = "payment.cancel"
)

// Notification permissions (notification service).
const (
	NotificationTelegramRead   = "notification.telegram.read"
	NotificationTelegramManage = "notification.telegram.manage"
)

// All lists every concrete (non-wildcard) permission in the catalog.
var Catalog = []string{
	UserCreate,
	UserRead,
	UserPermissionsUpdate,
	UserPasswordUpdate,
	UserStatusUpdate,
	UserDelete,
	ProductCreate,
	ProductUpdate,
	ProductDelete,
	ProductRead,
	ProductVariantCreate,
	ProductVariantUpdate,
	ProductVariantDelete,
	ProductImageUpload,
	ProductMasterRead,
	ProductMasterWrite,
	PromotionRead,
	PromotionCreate,
	PromotionUpdate,
	PromotionDelete,
	PromotionRedeem,
	PromotionIssue,
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
	PaymentBypass,
	PaymentCancel,
	NotificationTelegramRead,
	NotificationTelegramManage,
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
