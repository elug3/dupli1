package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/elug3/schick/auth/pkg/autherrors"
	"github.com/elug3/schick/auth/pkg/ports"
	"github.com/golang-jwt/jwt/v5"
)

// JWK represents a single JSON Web Key (RFC 7517).
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// RSATokenGenerator implements ports.TokenGenerator using RS256.
type RSATokenGenerator struct {
	privateKey     *rsa.PrivateKey
	keyID          string
	expiryDuration time.Duration
}

// NewRSATokenGenerator creates a token generator from an existing RSA key.
func NewRSATokenGenerator(key *rsa.PrivateKey, keyID string, expirySeconds int64) *RSATokenGenerator {
	if keyID == "" {
		keyID = "default"
	}
	return &RSATokenGenerator{
		privateKey:     key,
		keyID:          keyID,
		expiryDuration: time.Duration(expirySeconds) * time.Second,
	}
}

// NewRSATokenGeneratorFromPEM parses a PEM-encoded RSA private key and creates a token generator.
// Supports PKCS#1 ("RSA PRIVATE KEY") and PKCS#8 ("PRIVATE KEY") formats.
func NewRSATokenGeneratorFromPEM(pemBytes []byte, keyID string, expirySeconds int64) (*RSATokenGenerator, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key")
	}

	var privateKey *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		var err error
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS1 RSA private key: %w", err)
		}
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PEM contains %T, not an RSA private key", key)
		}
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %q", block.Type)
	}

	return NewRSATokenGenerator(privateKey, keyID, expirySeconds), nil
}

// GenerateRSAKey creates a new RSA private key with the given bit size.
func GenerateRSAKey(bits int) (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, bits)
}

// Generate issues a signed RS256 JWT with sub, roles, exp, iat, and kid header.
func (g *RSATokenGenerator) Generate(ctx context.Context, userID string, roles []string) (string, error) {
	if roles == nil {
		roles = []string{}
	}
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"exp":   time.Now().Add(g.expiryDuration).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = g.keyID

	tokenString, err := token.SignedString(g.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return tokenString, nil
}

// Validate verifies an RS256 JWT and returns the claims.
func (g *RSATokenGenerator) Validate(ctx context.Context, tokenString string) (ports.Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &g.privateKey.PublicKey, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return ports.Claims{}, autherrors.ErrTokenExpired
		}
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	if !token.Valid {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	userID, err := extractSubject(mapClaims)
	if err != nil {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	return ports.Claims{UserID: userID, Roles: extractRoles(mapClaims)}, nil
}

// PublicJWKS returns the JWKS document for the public key.
func (g *RSATokenGenerator) PublicJWKS() JWKS {
	pub := &g.privateKey.PublicKey
	return JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Use: "sig",
				Kid: g.keyID,
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
}
