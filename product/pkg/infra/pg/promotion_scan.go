package pg

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// promotionColumns is the one place the promotion column list is written, so a
// new field cannot be added to some queries and forgotten in others.
const promotionColumns = `code, scope, discount, description, expires, active, ` +
	`conditions, benefit, expires_at, max_redemptions, max_per_customer, terms, ` +
	`redemption_count, updated_at`

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanPromotion(row scanner) (*domain.Promotion, error) {
	var (
		p          domain.Promotion
		scope      string
		conditions []byte
		benefit    []byte
		expiresAt  *time.Time
		maxRedeem  *int
		updatedAt  time.Time
	)
	if err := row.Scan(
		&p.Code, &scope, &p.Discount, &p.Description, &p.Expires, &p.Active,
		&conditions, &benefit, &expiresAt, &maxRedeem, &p.MaxPerCustomer, &p.Terms,
		&p.RedemptionCount, &updatedAt,
	); err != nil {
		return nil, err
	}
	p.Scope = domain.Scope(scope)
	p.ExpiresAt = expiresAt
	p.MaxRedemptions = maxRedeem
	p.UpdatedAt = updatedAt

	// A document that will not parse must not be treated as "no rules": that
	// would turn a restricted code into an unrestricted one. Fail the read.
	if err := unmarshalDoc(conditions, &p.Conditions, p.Code, "conditions"); err != nil {
		return nil, err
	}
	if err := unmarshalDoc(benefit, &p.Benefit, p.Code, "benefit"); err != nil {
		return nil, err
	}
	return &p, nil
}

func unmarshalDoc(raw []byte, into any, code, what string) error {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("promotion %s: decode %s: %w", code, what, err)
	}
	return nil
}

func marshalPromotionDocs(p domain.Promotion) (conditions, benefit []byte, err error) {
	conditions, err = json.Marshal(p.Conditions)
	if err != nil {
		return nil, nil, fmt.Errorf("encode conditions: %w", err)
	}
	benefit, err = json.Marshal(p.Benefit)
	if err != nil {
		return nil, nil, fmt.Errorf("encode benefit: %w", err)
	}
	return conditions, benefit, nil
}

// applyPromotionPatch folds a partial update onto the current definition.
// Omitted fields keep their value; the Clear* flags are how a nullable field
// is actually unset, which a bare pointer cannot express.
func applyPromotionPatch(p *domain.Promotion, patch ports.PromotionPatch) {
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.Active != nil {
		p.Active = *patch.Active
	}
	if patch.Terms != nil {
		p.Terms = *patch.Terms
	}
	if patch.Scope != nil {
		p.Scope = *patch.Scope
	}
	if patch.Conditions != nil {
		p.Conditions = *patch.Conditions
	}
	if patch.Benefit != nil {
		p.Benefit = *patch.Benefit
	}
	if patch.MaxPerCustomer != nil {
		p.MaxPerCustomer = *patch.MaxPerCustomer
	}
	if patch.ClearExpiresAt {
		p.ExpiresAt = nil
	} else if patch.ExpiresAt != nil {
		p.ExpiresAt = patch.ExpiresAt
	}
	if patch.ClearMaxRedemptions {
		p.MaxRedemptions = nil
	} else if patch.MaxRedemptions != nil {
		p.MaxRedemptions = patch.MaxRedemptions
	}
	if patch.Discount != nil {
		p.Discount = *patch.Discount
	}
	if patch.Expires != nil {
		p.Expires = *patch.Expires
	}
}
