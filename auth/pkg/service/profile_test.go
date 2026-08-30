package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/infra/memory"
	jwtgen "github.com/elug3/dupli1/auth/pkg/infra/jwt"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/auth/pkg/service"
	"github.com/google/uuid"
)

type stubUserRepo struct{}

func (stubUserRepo) FindByEmail(context.Context, string) (*domain.User, error) { return nil, nil }
func (stubUserRepo) FindByID(context.Context, string) (*domain.User, error)    { return nil, nil }
func (stubUserRepo) ListAll(context.Context) ([]*domain.User, error)          { return nil, nil }
func (stubUserRepo) Save(context.Context, *domain.User) error                 { return nil }
func (stubUserRepo) Delete(context.Context, string) error                     { return nil }

func newProfileService(t *testing.T) (*service.Service, ports.ProfileRepository) {
	t.Helper()
	profileRepo := memory.NewProfileRepository()
	svc := service.NewService(
		stubUserRepo{},
		jwtgen.NewTokenGenerator("secret", 900),
		service.WithProfileRepository(profileRepo),
	)
	return svc, profileRepo
}

func TestPatchProfile_CreateAndUpdate(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	name := "윤라희"
	phone := "010-4112-5167"
	view, err := svc.PatchProfile(ctx, userID, service.ProfilePatch{
		DisplayName: &name,
		Phone:       &phone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.DisplayName != "윤라희" || view.Phone != "01041125167" {
		t.Fatalf("profile: %+v", view)
	}

	phone2 := "01099998888"
	view, err = svc.PatchProfile(ctx, userID, service.ProfilePatch{Phone: &phone2})
	if err != nil {
		t.Fatal(err)
	}
	if view.Phone != "01099998888" || view.DisplayName != "윤라희" {
		t.Fatalf("merge patch failed: %+v", view)
	}
}

func TestCreateAddress_DefaultFirst(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	addr, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName:  "윤라희",
		RecipientPhone: "01041125167",
		PostalCode:     "06194",
		AddressLine1:   "테헤란로 78길 14-12",
		City:           "강남구",
		Province:       "서울특별시",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !addr.IsDefault {
		t.Fatal("first address should be default")
	}

	view, err := svc.GetProfileView(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if view.DefaultAddressID != addr.ID || len(view.Addresses) != 1 {
		t.Fatalf("view: %+v", view)
	}
}

func TestCreateAddress_Limit(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	input := service.AddressInput{
		RecipientName:  "A",
		RecipientPhone: "01011112222",
		PostalCode:     "06194",
		AddressLine1:   "Line 1",
		City:           "강남구",
		Province:       "서울",
	}
	for i := 0; i < domain.MaxAddressesPerUser; i++ {
		if _, err := svc.CreateAddress(ctx, userID, input); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := svc.CreateAddress(ctx, userID, input); err != autherrors.ErrAddressLimitReached {
		t.Fatalf("want limit error, got %v", err)
	}
}

func TestPatchAddress_PartialUpdate(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	created, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "A", RecipientPhone: "01011112222", PostalCode: "06194",
		AddressLine1: "One", City: "강남구", Province: "서울",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.PatchAddress(ctx, userID, created.ID, service.AddressInput{
		RecipientName: "Updated Name",
		AddressLine2:  "Suite 9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.RecipientName != "Updated Name" {
		t.Fatalf("name = %q, want Updated Name", updated.RecipientName)
	}
	if updated.RecipientPhone != "01011112222" || updated.AddressLine2 != "Suite 9" {
		t.Fatalf("unchanged fields mutated: %+v", updated)
	}
}

func TestCreateAndPatchAddress_PCCC(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	created, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "A", RecipientPhone: "01011112222", PostalCode: "06194",
		AddressLine1: "One", City: "강남구", Province: "서울", PCCC: "p123456789012",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.PCCC != "P123456789012" {
		t.Fatalf("pccc = %q, want normalized P123456789012", created.PCCC)
	}

	// Patching an unrelated field should preserve the existing PCCC.
	updated, err := svc.PatchAddress(ctx, userID, created.ID, service.AddressInput{
		RecipientName: "Updated Name",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.PCCC != "P123456789012" {
		t.Fatalf("pccc after unrelated patch = %q, want unchanged", updated.PCCC)
	}

	if _, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "B", RecipientPhone: "01011112222", PostalCode: "06194",
		AddressLine1: "Two", City: "강남구", Province: "서울", PCCC: "bad-code",
	}); err == nil {
		t.Fatal("expected error for malformed pccc")
	}
}

func TestPatchAddress_NotFound(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	_, err := svc.PatchAddress(ctx, userID, "addr_missing", service.AddressInput{
		RecipientName: "Nobody",
	})
	if err != ports.ErrAddressNotFound {
		t.Fatalf("want ErrAddressNotFound, got %v", err)
	}
}

func TestDeleteAddress(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	addr, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "A", RecipientPhone: "01011112222", PostalCode: "06194",
		AddressLine1: "One", City: "강남구", Province: "서울",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAddress(ctx, userID, addr.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetAddress(ctx, userID, addr.ID); err != ports.ErrAddressNotFound {
		t.Fatalf("deleted address should be missing, got %v", err)
	}
	if err := svc.DeleteAddress(ctx, userID, "addr_missing"); err != ports.ErrAddressNotFound {
		t.Fatalf("second delete want ErrAddressNotFound, got %v", err)
	}
}

func TestPatchProfile_InvalidPhone(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	bad := "not-a-phone"
	_, err := svc.PatchProfile(ctx, userID, service.ProfilePatch{Phone: &bad})
	if err != domain.ErrInvalidProfile {
		t.Fatalf("want ErrInvalidProfile, got %v", err)
	}
}

func TestSetDefaultAddress(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := t.Context()
	userID := uuid.New().String()

	a1, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "A", RecipientPhone: "01011112222", PostalCode: "06194",
		AddressLine1: "One", City: "강남구", Province: "서울",
	})
	if err != nil {
		t.Fatal(err)
	}
	falseVal := false
	a2, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "B", RecipientPhone: "01033334444", PostalCode: "06194",
		AddressLine1: "Two", City: "강남구", Province: "서울",
		IsDefault: &falseVal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !a1.IsDefault {
		t.Fatal("a1 should stay default")
	}

	updated, err := svc.SetDefaultAddress(ctx, userID, a2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.IsDefault {
		t.Fatal("a2 should be default")
	}
	check, err := svc.GetAddress(ctx, userID, a1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.IsDefault {
		t.Fatal("a1 should no longer be default")
	}
}
