package authjwt

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

func TestHMACValidatorReadsPermissionsClaim(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         "user-1",
		"type":        "access",
		"permissions": permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}),
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
	if !claims.HasPermission(permissions.InventoryStockWrite) {
		t.Fatal("expected inventory.stock.write")
	}
}
