package authjwt

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

func signRS256AccessToken(t *testing.T, key *rsa.PrivateKey, kid, userID string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"type":  "access",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return signed
}

func publicJWKS(key *rsa.PrivateKey, kid string) []byte {
	pub := &key.PublicKey
	doc := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"kid": kid,
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	b, _ := json.Marshal(doc)
	return b
}

func TestJWKSValidatorExpandsAdminRole(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	const kid = "test-kid"
	jwks := publicJWKS(key, kid)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	defer srv.Close()

	validator := NewJWKSValidator(srv.URL)
	token := signRS256AccessToken(t, key, kid, "admin-1", []string{"admin"})

	claims, err := validator.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if !claims.HasPermission(permissions.ProductCreate) {
		t.Fatalf("admin should grant product.create, got %v", claims.Permissions)
	}
	if !claims.HasPermission(permissions.CouponRead) {
		t.Fatalf("admin should grant coupon.read, got %v", claims.Permissions)
	}
}
