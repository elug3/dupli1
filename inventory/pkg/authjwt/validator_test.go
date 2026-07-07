package authjwt

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

func TestHMACValidatorExpandsLegacyRoles(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-2",
		"type":  "access",
		"roles": []string{"order_manager"},
		"exp":   time.Now().Add(time.Hour).Unix(),
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
	if !claims.HasPermission(permissions.InventoryStockWrite) {
		t.Fatal("expected inventory.stock.write from order_manager role")
	}
	if claims.HasPermission(permissions.ProductCreate) {
		t.Fatal("did not expect product.create")
	}
}
