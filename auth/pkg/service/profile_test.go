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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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

func TestSetDefaultAddress(t *testing.T) {
	svc, _ := newProfileService(t)
	ctx := context.Background()
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
