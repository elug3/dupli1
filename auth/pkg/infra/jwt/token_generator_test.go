package jwt_test

import (
	"context"
	"testing"

	jwtinfra "github.com/elug3/dupli1/auth/pkg/infra/jwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestRoundtrip_UserIDAndPermissionsPreserved(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", 3600)
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-1", []string{permissions.UserCreate})
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
	if len(claims.Permissions) != 1 || claims.Permissions[0] != permissions.UserCreate {
		t.Fatalf("Permissions = %v, want [%s]", claims.Permissions, permissions.UserCreate)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != permissions.RoleCustomerRegistrar {
		t.Fatalf("Roles = %v, want [%s]", claims.Roles, permissions.RoleCustomerRegistrar)
	}
}

func TestGenerate_IncludesLegacyRolesForDualRead(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", 3600)
	ctx := context.Background()

	perms := permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})
	token, err := gen.Generate(ctx, "user-2", perms)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := gen.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !permissions.Has(claims.Permissions, permissions.AdminAll) {
		t.Fatalf("Permissions = %v, want admin wildcard", claims.Permissions)
	}
	if len(claims.Roles) == 0 {
		t.Fatal("expected legacy roles claim for dual-read")
	}
}

func TestGenerate_EmptyPermissionsInfersCustomerRole(t *testing.T) {
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
	if len(claims.Permissions) != 0 {
		t.Fatalf("Permissions = %v, want empty", claims.Permissions)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != permissions.RoleCustomer {
		t.Fatalf("Roles = %v, want [customer]", claims.Roles)
	}
}

func TestRefreshToken_OmitsPermissionsAndRoles(t *testing.T) {
	gen := jwtinfra.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-1", []string{permissions.All})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := gen.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(claims.Permissions) != 0 {
		t.Fatalf("refresh Permissions = %v, want empty", claims.Permissions)
	}
	if len(claims.Roles) != 0 {
		t.Fatalf("refresh Roles = %v, want empty", claims.Roles)
	}
}

func TestValidate_WrongSecretReturnsError(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("secret-A", 3600)
	ctx := context.Background()

	token, _ := gen.Generate(ctx, "user-1", []string{permissions.UserCreate})

	other := jwtinfra.NewTokenGenerator("secret-B", 3600)
	if _, err := other.Validate(ctx, token); err == nil {
		t.Fatal("expected error with wrong secret, got nil")
	}
}

func TestValidate_ExpiredTokenReturnsError(t *testing.T) {
	gen := jwtinfra.NewTokenGenerator("test-secret", -1)
	ctx := context.Background()

	token, err := gen.Generate(ctx, "user-1", []string{permissions.UserCreate})
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

	refreshToken, err := refresh.Generate(ctx, "user-1", nil)
	if err != nil {
		t.Fatalf("Generate refresh: %v", err)
	}
	if _, err := access.Validate(ctx, refreshToken); err == nil {
		t.Fatal("expected access validator to reject refresh token, got nil")
	}

	accessToken, err := access.Generate(ctx, "user-1", []string{permissions.UserCreate})
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

	token, _ := gen.Generate(ctx, "user-1", []string{permissions.UserCreate})

	if _, err := gen.Validate(ctx, token+"tampered"); err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
}
