package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxAddressesPerUser = 10

var (
	ErrInvalidProfile = errors.New("invalid profile")
	ErrInvalidAddress = errors.New("invalid address")

	krPhoneDigits = regexp.MustCompile(`^01[0-9]{8,9}$`)
	postalCodeRE  = regexp.MustCompile(`^\d{5}$`)
)

// Profile holds reusable customer defaults (1:1 with User).
type Profile struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name,omitempty"`
	Phone       string    `json:"phone,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// Address is a saved shipping address for a user.
type Address struct {
	ID             string    `json:"id"`
	UserID         string    `json:"-"`
	Label          string    `json:"label,omitempty"`
	RecipientName  string    `json:"recipient_name"`
	RecipientPhone string    `json:"recipient_phone"`
	PostalCode     string    `json:"postal_code"`
	AddressLine1   string    `json:"address_line1"`
	AddressLine2   string    `json:"address_line2,omitempty"`
	City           string    `json:"city"`
	Province       string    `json:"province"`
	IsDefault      bool      `json:"is_default"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ProfileView is returned by GET /me/profile.
type ProfileView struct {
	UserID           string     `json:"user_id"`
	DisplayName      string     `json:"display_name,omitempty"`
	Phone            string     `json:"phone,omitempty"`
	DefaultAddressID string     `json:"default_address_id,omitempty"`
	Addresses        []*Address `json:"addresses"`
}

// NormalizePersonName trims and validates a display or recipient name.
func NormalizePersonName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 50 {
		return "", ErrInvalidProfile
	}
	return name, nil
}

// NormalizeKRPhone strips formatting and validates a Korean mobile number.
func NormalizeKRPhone(phone string) (string, error) {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	normalized := digits.String()
	if !krPhoneDigits.MatchString(normalized) {
		return "", ErrInvalidProfile
	}
	return normalized, nil
}

// NormalizePostalCode validates a 5-digit Korean postal code.
func NormalizePostalCode(code string) (string, error) {
	code = strings.TrimSpace(code)
	if !postalCodeRE.MatchString(code) {
		return "", ErrInvalidAddress
	}
	return code, nil
}

// NormalizeAddressLine trims and validates a required address line.
func NormalizeAddressLine(line string, maxLen int) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" || utf8.RuneCountInString(line) > maxLen {
		return "", ErrInvalidAddress
	}
	return line, nil
}

// NormalizeOptionalLine trims an optional address field.
func NormalizeOptionalLine(line string, maxLen int) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if utf8.RuneCountInString(line) > maxLen {
		return "", ErrInvalidAddress
	}
	return line, nil
}

// ValidateAddressInput validates required address fields for create/update.
func ValidateAddressInput(recipientName, recipientPhone, postalCode, line1, line2, city, province string) (*Address, error) {
	name, err := NormalizePersonName(recipientName)
	if err != nil {
		return nil, ErrInvalidAddress
	}
	phone, err := NormalizeKRPhone(recipientPhone)
	if err != nil {
		return nil, ErrInvalidAddress
	}
	postal, err := NormalizePostalCode(postalCode)
	if err != nil {
		return nil, err
	}
	addr1, err := NormalizeAddressLine(line1, 200)
	if err != nil {
		return nil, err
	}
	addr2, err := NormalizeOptionalLine(line2, 200)
	if err != nil {
		return nil, err
	}
	cityNorm, err := NormalizeAddressLine(city, 100)
	if err != nil {
		return nil, err
	}
	provinceNorm, err := NormalizeAddressLine(province, 100)
	if err != nil {
		return nil, err
	}
	return &Address{
		RecipientName:  name,
		RecipientPhone: phone,
		PostalCode:     postal,
		AddressLine1:   addr1,
		AddressLine2:   addr2,
		City:           cityNorm,
		Province:       provinceNorm,
	}, nil
}
