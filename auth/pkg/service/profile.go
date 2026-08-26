package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
)

// ProfilePatch is a merge-patch payload for customer profile fields.
type ProfilePatch struct {
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
}

// AddressInput is the create/update body for a saved address.
type AddressInput struct {
	Label          string `json:"label"`
	RecipientName  string `json:"recipient_name"`
	RecipientPhone string `json:"recipient_phone"`
	PostalCode     string `json:"postal_code"`
	AddressLine1   string `json:"address_line1"`
	AddressLine2   string `json:"address_line2"`
	City           string `json:"city"`
	Province       string `json:"province"`
	PCCC           string `json:"pccc"`
	IsDefault      *bool  `json:"is_default"`
}

func (s *Service) requireProfileRepo() error {
	if s.profileRepo == nil {
		return errors.New("profile repository not configured")
	}
	return nil
}

func (s *Service) GetProfileView(ctx context.Context, userID string) (*domain.ProfileView, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	profile, err := s.profileRepo.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	addresses, err := s.profileRepo.ListAddresses(ctx, userID)
	if err != nil {
		return nil, err
	}
	if addresses == nil {
		addresses = []*domain.Address{}
	}
	view := &domain.ProfileView{
		UserID:    userID,
		Addresses: addresses,
	}
	if profile != nil {
		view.DisplayName = profile.DisplayName
		view.Phone = profile.Phone
	}
	for _, a := range addresses {
		if a.IsDefault {
			view.DefaultAddressID = a.ID
			break
		}
	}
	return view, nil
}

func (s *Service) PatchProfile(ctx context.Context, userID string, patch ProfilePatch) (*domain.ProfileView, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	if patch.DisplayName == nil && patch.Phone == nil {
		return s.GetProfileView(ctx, userID)
	}

	now := time.Now().UTC()
	profile, err := s.profileRepo.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		profile = &domain.Profile{UserID: userID, CreatedAt: now}
	}

	if patch.DisplayName != nil {
		name, err := domain.NormalizePersonName(*patch.DisplayName)
		if err != nil {
			return nil, domain.ErrInvalidProfile
		}
		profile.DisplayName = name
	}
	if patch.Phone != nil {
		phone, err := domain.NormalizeKRPhone(*patch.Phone)
		if err != nil {
			return nil, domain.ErrInvalidProfile
		}
		profile.Phone = phone
	}
	profile.UpdatedAt = now
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	if err := s.profileRepo.UpsertProfile(ctx, profile); err != nil {
		return nil, err
	}
	return s.GetProfileView(ctx, userID)
}

func (s *Service) ListAddresses(ctx context.Context, userID string) ([]*domain.Address, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	addresses, err := s.profileRepo.ListAddresses(ctx, userID)
	if err != nil {
		return nil, err
	}
	if addresses == nil {
		return []*domain.Address{}, nil
	}
	return addresses, nil
}

func (s *Service) GetAddress(ctx context.Context, userID, addressID string) (*domain.Address, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	a, err := s.profileRepo.GetAddress(ctx, userID, addressID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ports.ErrAddressNotFound
	}
	return a, nil
}

func (s *Service) CreateAddress(ctx context.Context, userID string, input AddressInput) (*domain.Address, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	count, err := s.profileRepo.CountAddresses(ctx, userID)
	if err != nil {
		return nil, err
	}
	if count >= domain.MaxAddressesPerUser {
		return nil, autherrors.ErrAddressLimitReached
	}

	validated, err := domain.ValidateAddressInput(
		input.RecipientName, input.RecipientPhone, input.PostalCode,
		input.AddressLine1, input.AddressLine2, input.City, input.Province, input.PCCC,
	)
	if err != nil {
		return nil, err
	}

	id, err := s.profileRepo.NextAddressID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	isDefault := false
	if input.IsDefault != nil {
		isDefault = *input.IsDefault
	} else if count == 0 {
		isDefault = true
	}
	if isDefault {
		if err := s.profileRepo.ClearDefaultAddresses(ctx, userID); err != nil {
			return nil, err
		}
	}

	validated.ID = id
	validated.UserID = userID
	validated.Label = strings.TrimSpace(input.Label)
	validated.IsDefault = isDefault
	validated.CreatedAt = now
	validated.UpdatedAt = now
	if err := s.profileRepo.SaveAddress(ctx, validated); err != nil {
		return nil, err
	}
	return validated, nil
}

func (s *Service) PatchAddress(ctx context.Context, userID, addressID string, input AddressInput) (*domain.Address, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	existing, err := s.profileRepo.GetAddress(ctx, userID, addressID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ports.ErrAddressNotFound
	}

	recipientName := existing.RecipientName
	if input.RecipientName != "" {
		recipientName = input.RecipientName
	}
	recipientPhone := existing.RecipientPhone
	if input.RecipientPhone != "" {
		recipientPhone = input.RecipientPhone
	}
	postalCode := existing.PostalCode
	if input.PostalCode != "" {
		postalCode = input.PostalCode
	}
	line1 := existing.AddressLine1
	if input.AddressLine1 != "" {
		line1 = input.AddressLine1
	}
	line2 := existing.AddressLine2
	if input.AddressLine2 != "" {
		line2 = input.AddressLine2
	}
	city := existing.City
	if input.City != "" {
		city = input.City
	}
	province := existing.Province
	if input.Province != "" {
		province = input.Province
	}
	pccc := existing.PCCC
	if input.PCCC != "" {
		pccc = input.PCCC
	}

	validated, err := domain.ValidateAddressInput(
		recipientName, recipientPhone, postalCode, line1, line2, city, province, pccc,
	)
	if err != nil {
		return nil, err
	}

	if input.Label != "" || existing.Label != "" {
		label := existing.Label
		if input.Label != "" {
			label = strings.TrimSpace(input.Label)
		}
		validated.Label = label
	}

	if input.IsDefault != nil && *input.IsDefault {
		if err := s.profileRepo.ClearDefaultAddresses(ctx, userID); err != nil {
			return nil, err
		}
		validated.IsDefault = true
	} else {
		validated.IsDefault = existing.IsDefault
	}

	validated.ID = existing.ID
	validated.UserID = userID
	validated.CreatedAt = existing.CreatedAt
	validated.UpdatedAt = time.Now().UTC()
	if err := s.profileRepo.SaveAddress(ctx, validated); err != nil {
		return nil, err
	}
	return validated, nil
}

func (s *Service) DeleteAddress(ctx context.Context, userID, addressID string) error {
	if err := s.requireProfileRepo(); err != nil {
		return err
	}
	return s.profileRepo.DeleteAddress(ctx, userID, addressID)
}

func (s *Service) SetDefaultAddress(ctx context.Context, userID, addressID string) (*domain.Address, error) {
	if err := s.requireProfileRepo(); err != nil {
		return nil, err
	}
	existing, err := s.profileRepo.GetAddress(ctx, userID, addressID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ports.ErrAddressNotFound
	}
	if err := s.profileRepo.ClearDefaultAddresses(ctx, userID); err != nil {
		return nil, err
	}
	existing.IsDefault = true
	existing.UpdatedAt = time.Now().UTC()
	if err := s.profileRepo.SaveAddress(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}
