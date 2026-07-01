package jwt_test

import (
	"context"
	"testing"

	jwtinfra "github.com/elug3/dupli1/auth/pkg/infra/jwt"
)

func TestRoundtrip_UserIDAndRolesPreserved(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", 3600)
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-1", []string{"customer"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := gen.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", claims.UserID)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "customer" {
		t.Fatalf("Roles = %v, want [customer]", claims.Roles)
	}
}

func TestGenerate_MultipleRolesPreserved(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", 3600)
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-2", []string{"order_manager", "admin"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := gen.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(claims.Roles) != 2 {
		t.Fatalf("len(Roles) = %d, want 2", len(claims.Roles))
	}
}

func TestGenerate_EmptyRolesPreserved(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", 3600)
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-3", []string{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := gen.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(claims.Roles) != 0 {
		t.Fatalf("Roles = %v, want empty", claims.Roles)
	}
}

func TestValidate_WrongSecretReturnsError(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("secret-A", 3600)
	ctx := context.Background()

	token, _ := gen.Generate(ctx, "user-1", []string{"customer"})

	other := jwtinfra.NewTokenGenerator("secret-B", 3600)
	if _, err := other.Validate(ctx, token); err == nil {
		t.Fatal("expected error with wrong secret, got nil")
	}
}

func TestValidate_ExpiredTokenReturnsError(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", -1) // -1s → already expired
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-1", []string{"customer"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := gen.Validate(ctx, token); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidate_RejectsWrongTokenType(t *testing.T) {
	access := jwtinfra.NewTokenGeneratorWithType("test-secret", 3600, "access")
	refresh := jwtinfra.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	ctx := context.Background()

	refreshToken, err := refresh.Generate(ctx, "user-1", []string{"customer"})
	if err != nil {
		t.Fatalf("Generate refresh: %v", err)
	}
	if _, err := access.Validate(ctx, refreshToken); err == nil {
		t.Fatal("expected access validator to reject refresh token, got nil")
	}

	accessToken, err := access.Generate(ctx, "user-1", []string{"customer"})
	if err != nil {
		t.Fatalf("Generate access: %v", err)
	}
	if _, err := refresh.Validate(ctx, accessToken); err == nil {
		t.Fatal("expected refresh validator to reject access token, got nil")
	}
}

func TestValidate_TamperedTokenReturnsError(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", 3600)
	ctx := context.Background()

	token, _ := gen.Generate(ctx, "user-1", []string{"customer"})

	if _, err := gen.Validate(ctx, token+"tampered"); err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
}
