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
	if err := v.ValidateAccessToken(signed); err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
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
	if err := v.ValidateAccessToken(signed); err == nil {
		t.Fatal("expected refresh token to be rejected")
	}
}
