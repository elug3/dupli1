package domain

import "fmt"

// Benefit is what a promotional code gives. It is stored as JSONB beside the
// conditions so the discount shape can grow without another column.

// BenefitTarget is what the discount comes off.
type BenefitTarget string

const (
	// BenefitTargetGoods discounts eligible line goods. The only target
	// implemented today.
	BenefitTargetGoods BenefitTarget = "goods"
	// The rest are Phase 4. They are named here so stored documents and the
	// wire shape do not have to change when they land, but Validate rejects
	// them for now rather than accepting a definition that would quietly
	// discount nothing.
	BenefitTargetShipping         BenefitTarget = "shipping"
	BenefitTargetGoodsAndShipping BenefitTarget = "goods_and_shipping"
	BenefitTargetNone             BenefitTarget = "none"
)

// DiscountType is how the amount is worked out.
type DiscountType string

const (
	DiscountTypePercent DiscountType = "percent"
	DiscountTypeFixed   DiscountType = "fixed"
	DiscountTypeNone    DiscountType = "none"
)

// ApplyTo is the base the discount is computed against.
type ApplyTo string

const (
	// ApplyToEntireSubtotal charges the discount against the whole cart, which
	// is what every pre-Phase-2 code did.
	ApplyToEntireSubtotal ApplyTo = "entire_subtotal"
	// ApplyToEligibleLines charges it against only the lines that matched the
	// conditions, leaving the rest at full price.
	ApplyToEligibleLines ApplyTo = "eligible_lines"
	ApplyToShippingFee   ApplyTo = "shipping_fee"
)

type Benefit struct {
	Target           BenefitTarget `json:"target"`
	DiscountType     DiscountType  `json:"discount_type"`
	DiscountFraction float64       `json:"discount_fraction,omitempty"`
	DiscountFixedWon int64         `json:"discount_fixed_won,omitempty"`
	// MaxDiscountWon caps a percentage on a large cart. nil means uncapped.
	MaxDiscountWon *int64  `json:"max_discount_won,omitempty"`
	ApplyTo        ApplyTo `json:"apply_to,omitempty"`
}

// IsZero reports a benefit that was never set, which is how rows written
// before Phase 2 look.
func (b Benefit) IsZero() bool {
	return b.Target == "" && b.DiscountType == "" &&
		b.DiscountFraction == 0 && b.DiscountFixedWon == 0
}

// Validate rejects a benefit a manager should not be able to save.
//
// This runs on write precisely because the pre-Phase-2 service did not: a
// definition with a nonsense discount saved cleanly and then failed at
// somebody's checkout, where nobody who could fix it would see it.
func (b Benefit) Validate() error {
	switch b.Target {
	case BenefitTargetGoods:
	case BenefitTargetShipping, BenefitTargetGoodsAndShipping, BenefitTargetNone:
		return fmt.Errorf("benefit target %q is not implemented yet; only %q is supported", b.Target, BenefitTargetGoods)
	default:
		return fmt.Errorf("benefit target %q is not one of goods, shipping, goods_and_shipping, none", b.Target)
	}

	switch b.DiscountType {
	case DiscountTypePercent:
		if b.DiscountFraction <= 0 || b.DiscountFraction >= 1 {
			return fmt.Errorf("discount_fraction must be greater than 0 and less than 1, got %v", b.DiscountFraction)
		}
		if b.DiscountFixedWon != 0 {
			return fmt.Errorf("discount_fixed_won must be 0 for a percent benefit")
		}
	case DiscountTypeFixed:
		if b.DiscountFixedWon <= 0 {
			return fmt.Errorf("discount_fixed_won must be greater than 0, got %d", b.DiscountFixedWon)
		}
		if b.DiscountFraction != 0 {
			return fmt.Errorf("discount_fraction must be 0 for a fixed benefit")
		}
	case DiscountTypeNone:
		return fmt.Errorf("discount_type %q is not implemented yet", b.DiscountType)
	default:
		return fmt.Errorf("discount_type %q is not one of percent, fixed, none", b.DiscountType)
	}

	if b.MaxDiscountWon != nil && *b.MaxDiscountWon <= 0 {
		return fmt.Errorf("max_discount_won must be greater than 0 when set, got %d", *b.MaxDiscountWon)
	}
	switch b.ApplyTo {
	case "", ApplyToEntireSubtotal, ApplyToEligibleLines:
	case ApplyToShippingFee:
		return fmt.Errorf("apply_to %q needs a shipping benefit target, which is not implemented yet", b.ApplyTo)
	default:
		return fmt.Errorf("apply_to %q is not one of entire_subtotal, eligible_lines", b.ApplyTo)
	}
	return nil
}

// base picks the money the discount is computed against.
func (b Benefit) base(ctx EvaluationContext, eligible []EvaluationLine) int64 {
	if b.ApplyTo == ApplyToEligibleLines {
		var total int64
		for _, line := range eligible {
			total += line.ExtendedWon()
		}
		return total
	}
	return ctx.SubtotalWon()
}

// discountFor computes the won amount, never exceeding the base it is drawn
// from. Clamping here is what keeps a 5,000원 code on a 3,000원 cart from
// discounting more than the cart is worth, and what keeps an order total from
// dropping below its shipping fee.
func (b Benefit) discountFor(base int64) int64 {
	if base <= 0 {
		return 0
	}
	var discount int64
	switch b.DiscountType {
	case DiscountTypePercent:
		discount = int64(float64(base) * b.DiscountFraction)
	case DiscountTypeFixed:
		discount = b.DiscountFixedWon
	default:
		return 0
	}
	if b.MaxDiscountWon != nil && discount > *b.MaxDiscountWon {
		discount = *b.MaxDiscountWon
	}
	if discount > base {
		discount = base
	}
	if discount < 0 {
		return 0
	}
	return discount
}
