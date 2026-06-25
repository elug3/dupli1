package ports

import (
	"context"
	"errors"
)

var (
	ErrCouponInvalid     = errors.New("invalid coupon code")
	ErrCouponUnavailable = errors.New("coupon service unavailable")
)

type Coupon struct {
	Code           string
	DiscountFraction float64
}

type CouponClient interface {
	Redeem(ctx context.Context, code string) (*Coupon, error)
}
