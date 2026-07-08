package authjwt

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

func TestHMACValidatorRequiresAccessType(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-1",
		"type": "access",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	v := NewHMACValidator("test-secret")
	claims, err := v.ValidateAccessToken(signed)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", claims.UserID)
	}
}

func TestHMACValidatorRejectsRefreshType(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-1",
		"type": "refresh",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	v := NewHMACValidator("test-secret")
	if _, err := v.ValidateAccessToken(signed); err == nil {
		t.Fatal("expected refresh token to be rejected")
	}
}

func TestHMACValidatorReadsPermissionsClaim(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         "user-2",
		"type":        "access",
		"permissions": []string{permissions.ProductAll, permissions.CouponAll},
		"exp":         time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	v := NewHMACValidator("test-secret")
	claims, err := v.ValidateAccessToken(signed)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if !claims.HasPermission(permissions.ProductCreate) {
		t.Fatal("expected product.create from product.* wildcard")
	}
	if claims.HasPermission(permissions.OrderShip) {
		t.Fatal("did not expect order.ship")
	}
}

func TestHMACValidatorIgnoresLegacyRolesClaim(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         "user-3",
		"type":        "access",
		"permissions": []string{permissions.CouponRead},
		"roles":       []string{"product_manager"},
		"exp":         time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	v := NewHMACValidator("test-secret")
	claims, err := v.ValidateAccessToken(signed)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if !claims.HasPermission(permissions.CouponRead) {
		t.Fatal("expected coupon.read from permissions claim")
	}
	if claims.HasPermission(permissions.ProductCreate) {
		t.Fatal("legacy roles claim must not grant permissions")
	}
}

func TestExtractStringSliceHandlesStringSliceClaim(t *testing.T) {
	claims := jwt.MapClaims{
		"permissions": []string{"coupon.read"},
	}
	perms := extractStringSlice(claims, "permissions")
	if len(perms) != 1 || perms[0] != "coupon.read" {
		t.Fatalf("permissions = %v, want [coupon.read]", perms)
	}
}
