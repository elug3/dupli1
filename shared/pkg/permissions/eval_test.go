package permissions

import "testing"

func TestHas_exactMatch(t *testing.T) {
	held := []string{ProductCreate, ProductUpdate}
	if !Has(held, ProductCreate) {
		t.Fatal("expected exact match")
	}
	if Has(held, ProductDelete) {
		t.Fatal("did not expect product.delete")
	}
}

func TestHas_starWildcard(t *testing.T) {
	if !Has([]string{All}, ProductCreate) {
		t.Fatal("* should grant any permission")
	}
	if !Has([]string{All}, UserCreate) {
		t.Fatal("* should grant user.create")
	}
}

func TestHas_resourceWildcard(t *testing.T) {
	held := []string{ProductAll}
	cases := []struct {
		perm string
		want bool
	}{
		{ProductCreate, true},
		{ProductVariantCreate, true},
		{ProductImageUpload, true},
		{CouponCreate, false},
		{UserCreate, false},
	}
	for _, tc := range cases {
		if got := Has(held, tc.perm); got != tc.want {
			t.Fatalf("product.* vs %s = %v, want %v", tc.perm, got, tc.want)
		}
	}
}

func TestHas_adminWildcard(t *testing.T) {
	held := []string{AdminAll}
	if !Has(held, UserCreate) {
		t.Fatal("admin.* should grant user.create")
	}
	if !Has(held, UserPermissionsUpdate) {
		t.Fatal("admin.* should grant user.permissions.update")
	}
	if Has(held, ProductCreate) {
		t.Fatal("admin.* should not grant product.create")
	}
}

func TestHas_evaluationOrder_exactBeforeWildcard(t *testing.T) {
	// Exact product.create without product.* should not grant product.update
	held := []string{ProductCreate}
	if Has(held, ProductUpdate) {
		t.Fatal("product.create alone should not grant product.update")
	}
}

func TestHasAny(t *testing.T) {
	held := []string{CouponRead}
	if !HasAny(held, ProductCreate, CouponRead) {
		t.Fatal("expected HasAny to match coupon.read")
	}
	if HasAny(held, ProductCreate, ProductUpdate) {
		t.Fatal("unexpected match")
	}
}

func TestHasAll(t *testing.T) {
	held := []string{ProductCreate, ProductUpdate}
	if !HasAll(held, ProductCreate, ProductUpdate) {
		t.Fatal("expected HasAll")
	}
	if HasAll(held, ProductCreate, ProductDelete) {
		t.Fatal("did not expect HasAll with missing perm")
	}
}

func TestCanRegisterAnyAccountType(t *testing.T) {
	if CanRegisterAnyAccountType([]string{UserCreate}) {
		t.Fatal("user.create alone cannot register any account type")
	}
	if !CanRegisterAnyAccountType([]string{UserCreate, UserPermissionsUpdate}) {
		t.Fatal("user.permissions.update should allow any account type")
	}
	if !CanRegisterAnyAccountType([]string{AdminAll}) {
		t.Fatal("admin.* should allow any account type")
	}
}

func TestBypassesOrderABAC(t *testing.T) {
	if BypassesOrderABAC(nil) {
		t.Fatal("empty should not bypass ABAC")
	}
	if !BypassesOrderABAC([]string{OrderReadAll}) {
		t.Fatal("order.read.all should bypass ABAC")
	}
	if BypassesOrderABAC([]string{OrderShip}) {
		t.Fatal("order.ship alone should not bypass read ABAC")
	}
}

func TestBypassesPaymentABAC(t *testing.T) {
	if !BypassesPaymentABAC([]string{PaymentReadAll}) {
		t.Fatal("payment.read.all should bypass ABAC")
	}
}

func TestCanBypassPayment(t *testing.T) {
	if CanBypassPayment(nil) {
		t.Fatal("empty should not allow method bypass")
	}
	if CanBypassPayment([]string{PaymentCreate}) {
		t.Fatal("payment.create alone should not allow method bypass")
	}
	if !CanBypassPayment([]string{PaymentBypass}) {
		t.Fatal("payment.bypass should allow method bypass")
	}
	if !CanBypassPayment([]string{All}) {
		t.Fatal("* should allow method bypass")
	}
	if !CanBypassPayment([]string{AdminAll}) {
		t.Fatal("admin.* should allow method bypass")
	}
}
