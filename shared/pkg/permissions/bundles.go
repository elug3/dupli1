package permissions

import "fmt"

// Bundle names for preset permission sets.
const (
	BundleCatalogEditor      = "catalog_editor"
	BundleCatalogAdmin       = "catalog_admin"
	BundleFulfillment        = "fulfillment"
	BundleUserAdmin          = "user_admin"
	BundleCustomerRegistrar  = "customer_registrar"
)

var bundles = map[string][]string{
	BundleCatalogEditor: {
		ProductCreate,
		ProductUpdate,
		ProductRead,
		ProductVariantCreate,
		ProductVariantUpdate,
		ProductImageUpload,
		ProductMasterRead,
		ProductMasterWrite,
	},
	BundleCatalogAdmin: {
		ProductAll,
		CouponAll,
	},
	BundleFulfillment: {
		OrderShip,
		OrderStatusUpdate,
		InventoryStockWrite,
		InventoryReservationManage,
		CartRead,
		PaymentBypass,
	},
	BundleUserAdmin: {
		UserCreate,
		UserRead,
		UserPasswordUpdate,
		UserStatusUpdate,
	},
	BundleCustomerRegistrar: {
		UserCreate,
	},
}

// BundleNames returns sorted bundle identifiers.
func BundleNames() []string {
	names := make([]string, 0, len(bundles))
	for name := range bundles {
		names = append(names, name)
	}
	// simple insertion sort for stable tiny list
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// ExpandBundle returns the permissions for a named bundle.
func ExpandBundle(name string) ([]string, error) {
	perms, ok := bundles[name]
	if !ok {
		return nil, fmt.Errorf("unknown bundle: %q", name)
	}
	out := make([]string, len(perms))
	copy(out, perms)
	return out, nil
}

// ExpandBundles unions multiple named bundles.
func ExpandBundles(names ...string) ([]string, error) {
	var out []string
	for _, name := range names {
		perms, err := ExpandBundle(name)
		if err != nil {
			return nil, err
		}
		out = append(out, perms...)
	}
	return Dedupe(out), nil
}
