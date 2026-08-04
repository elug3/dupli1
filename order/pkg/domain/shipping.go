package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ErrInvalidFulfillment = errors.New("invalid fulfillment")

var (
	krPhoneDigits = regexp.MustCompile(`^01[0-9]{8,9}$`)
	postalCodeRE  = regexp.MustCompile(`^\d{5}$`)
)

// ShippingAddress is the immutable shipping location snapshot on an order.
type ShippingAddress struct {
	PostalCode   string `json:"postal_code"`
	AddressLine1 string `json:"address_line1"`
	AddressLine2 string `json:"address_line2,omitempty"`
	City         string `json:"city"`
	Province     string `json:"province"`
}

// FulfillmentSnapshot captures recipient + shipping data at checkout complete.
type FulfillmentSnapshot struct {
	RecipientName   string
	RecipientPhone  string
	ShippingAddress ShippingAddress
	SourceAddressID string
}

// ValidateFulfillmentSnapshot normalizes and validates checkout fulfillment input.
func ValidateFulfillmentSnapshot(recipientName, recipientPhone string, addr ShippingAddress, sourceAddressID string) (*FulfillmentSnapshot, error) {
	name, err := normalizePersonName(recipientName)
	if err != nil {
		return nil, ErrInvalidFulfillment
	}
	phone, err := normalizeKRPhone(recipientPhone)
	if err != nil {
		return nil, ErrInvalidFulfillment
	}
	normalizedAddr, err := normalizeShippingAddress(addr)
	if err != nil {
		return nil, err
	}
	return &FulfillmentSnapshot{
		RecipientName:   name,
		RecipientPhone:  phone,
		ShippingAddress: normalizedAddr,
		SourceAddressID: strings.TrimSpace(sourceAddressID),
	}, nil
}

func normalizePersonName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 50 {
		return "", ErrInvalidFulfillment
	}
	return name, nil
}

func normalizeKRPhone(phone string) (string, error) {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	normalized := digits.String()
	if !krPhoneDigits.MatchString(normalized) {
		return "", ErrInvalidFulfillment
	}
	return normalized, nil
}

func normalizeShippingAddress(addr ShippingAddress) (ShippingAddress, error) {
	postal, err := normalizePostalCode(addr.PostalCode)
	if err != nil {
		return ShippingAddress{}, err
	}
	line1, err := normalizeAddressLine(addr.AddressLine1, 200)
	if err != nil {
		return ShippingAddress{}, err
	}
	line2, err := normalizeOptionalLine(addr.AddressLine2, 200)
	if err != nil {
		return ShippingAddress{}, err
	}
	city, err := normalizeAddressLine(addr.City, 100)
	if err != nil {
		return ShippingAddress{}, err
	}
	province, err := normalizeAddressLine(addr.Province, 100)
	if err != nil {
		return ShippingAddress{}, err
	}
	return ShippingAddress{
		PostalCode:   postal,
		AddressLine1: line1,
		AddressLine2: line2,
		City:         city,
		Province:     province,
	}, nil
}

func normalizePostalCode(code string) (string, error) {
	code = strings.TrimSpace(code)
	if !postalCodeRE.MatchString(code) {
		return "", ErrInvalidFulfillment
	}
	return code, nil
}

func normalizeAddressLine(line string, maxLen int) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" || utf8.RuneCountInString(line) > maxLen {
		return "", ErrInvalidFulfillment
	}
	return line, nil
}

func normalizeOptionalLine(line string, maxLen int) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if utf8.RuneCountInString(line) > maxLen {
		return "", ErrInvalidFulfillment
	}
	return line, nil
}

// ApplyFulfillment sets validated fulfillment fields on the order.
func (o *Order) ApplyFulfillment(snapshot *FulfillmentSnapshot) error {
	if o == nil || snapshot == nil {
		return ErrInvalidFulfillment
	}
	o.RecipientName = snapshot.RecipientName
	o.RecipientPhone = snapshot.RecipientPhone
	o.ShippingAddress = snapshot.ShippingAddress
	o.SourceAddressID = snapshot.SourceAddressID
	return nil
}
