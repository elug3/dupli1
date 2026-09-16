package ports

import (
	"context"
	"errors"
)

var (
	ErrPromotionInvalid     = errors.New("invalid promotion code")
	ErrPromotionUnavailable = errors.New("promotion service unavailable")
)

type Promotion struct {
	Code           string
	DiscountFraction float64
}

type PromotionClient interface {
	Redeem(ctx context.Context, code string) (*Promotion, error)
}
