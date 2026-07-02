package authjwt

import (
	"testing"
	"time"

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

func TestHMACValidatorExtractsRoles(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-2",
		"type":  "access",
		"roles": []string{"order_manager", "customer"},
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
	if !claims.HasRole("order_manager") {
		t.Fatal("expected order_manager role")
	}
	if claims.HasRole("admin") {
		t.Fatal("did not expect admin role")
	}
}

func TestNewAccessTokenValidatorPrefersJWKS(t *testing.T) {
	v, err := NewAccessTokenValidator("http://auth/jwks.json", "secret")
	if err != nil {
		t.Fatalf("NewAccessTokenValidator: %v", err)
	}
	if _, ok := v.(*JWKSValidator); !ok {
		t.Fatalf("expected JWKSValidator, got %T", v)
	}
}

func TestNewAccessTokenValidatorFallsBackToHMAC(t *testing.T) {
	v, err := NewAccessTokenValidator("", "secret")
	if err != nil {
		t.Fatalf("NewAccessTokenValidator: %v", err)
	}
	if _, ok := v.(*HMACValidator); !ok {
		t.Fatalf("expected HMACValidator, got %T", v)
	}
}

func TestNewAccessTokenValidatorRequiresConfig(t *testing.T) {
	if _, err := NewAccessTokenValidator("", ""); err == nil {
		t.Fatal("expected error when neither JWKS URL nor secret is set")
	}
}
