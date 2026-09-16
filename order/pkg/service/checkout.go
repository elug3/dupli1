package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

type CreateCheckoutSessionInput struct {
	CustomerID string
}

type CompleteCheckoutResult struct {
	Session *domain.CheckoutSession `json:"session"`
	Order   *domain.Order           `json:"order"`
}

func (s *Service) CreateCheckoutSession(ctx context.Context, input CreateCheckoutSessionInput) (*domain.CheckoutSession, error) {
	sessionID, err := s.repo.NextCheckoutSessionID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := domain.NewCheckoutSession(sessionID, input.CustomerID, s.now(), s.checkoutTTL, s.shippingFeeKRW)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveCheckoutSession(ctx, session); err != nil {
		return nil, err
	}
	return cloneCheckoutSession(session), nil
}

func (s *Service) GetCheckoutSession(ctx context.Context, id string) (*domain.CheckoutSession, error) {
	session, err := s.repo.GetCheckoutSession(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := session.EnsureOpen(s.now()); err != nil && !errors.Is(err, domain.ErrSessionNotOpen) {
		_ = s.repo.SaveCheckoutSession(ctx, session)
		return nil, err
	}
	return s.annotateCheckoutSession(ctx, cloneCheckoutSession(session)), nil
}

func (s *Service) SetCheckoutItems(ctx context.Context, sessionID string, items []domain.OrderItem) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	priced, err := s.priceItems(ctx, items)
	if err != nil {
		return nil, err
	}
	if err := session.SetItems(priced, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) UpsertCheckoutItem(ctx context.Context, sessionID string, item domain.OrderItem) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	priced, err := s.priceItems(ctx, []domain.OrderItem{item})
	if err != nil {
		return nil, err
	}
	if err := session.UpsertItem(priced[0], s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) RemoveCheckoutItem(ctx context.Context, sessionID, sku string) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := session.RemoveItem(sku, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) RemoveCheckoutItemBySkuID(ctx context.Context, sessionID, skuID string) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := session.RemoveItemBySkuID(skuID, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

// ApplyCheckoutPromotion asks product whether a code applies to this cart and
// records the discount it earns.
//
// The cart goes with the request because eligibility and amount both depend on
// it — minimum spend, which lines qualify, a cap. Product does the arithmetic;
// order stores the answer and re-asks at complete.
func (s *Service) ApplyCheckoutPromotion(ctx context.Context, sessionID, code string) (*domain.CheckoutSession, error) {
	if s.promotionClient == nil {
		return nil, ports.ErrPromotionUnavailable
	}

	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(session.Items) == 0 {
		return nil, domain.ErrEmptyCheckout
	}

	// Price the lines first: the evaluator must judge the cart against
	// server-resolved prices, never numbers a client supplied.
	priced, err := s.priceItems(ctx, session.Items)
	if err != nil {
		return nil, err
	}

	verdict, err := s.promotionClient.Evaluate(ctx, code, promotionContextFor(session, priced))
	if err != nil {
		return nil, err
	}
	if !verdict.OK {
		return nil, promotionRejection(verdict)
	}
	if err := session.ApplyPromotion(code, verdict.DiscountWon, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

// ClearCheckoutPromotion removes an applied code. Until Phase 2 the domain
// could do this but nothing was routed to it, so a customer who applied a code
// had no way to take it off again.
func (s *Service) ClearCheckoutPromotion(ctx context.Context, sessionID string) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := session.ClearPromotion(s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

// promotionContextFor builds the evaluator's view of a session from its priced
// lines.
func promotionContextFor(session *domain.CheckoutSession, priced []domain.OrderItem) ports.PromotionContext {
	lines := make([]ports.PromotionLine, 0, len(priced))
	for _, item := range priced {
		lines = append(lines, ports.PromotionLine{
			SkuID:        item.SkuID,
			SKU:          item.SKU,
			Quantity:     item.Quantity,
			UnitPriceWon: item.UnitPriceWon,
		})
	}
	return ports.PromotionContext{
		CustomerID:     session.CustomerID,
		ShippingFeeWon: session.ShippingFeeWon,
		Lines:          lines,
	}
}

// promotionRejection turns a verdict into an error carrying its reason, so the
// handler can surface a machine-readable code rather than a bare 400.
func promotionRejection(verdict *ports.PromotionEvaluation) error {
	reason := verdict.Reason
	if reason == "" {
		reason = "not_eligible"
	}
	if verdict.SubReason != "" {
		reason += ":" + verdict.SubReason
	}
	if verdict.Reason == "invalid_code" {
		return fmt.Errorf("%w: %s", ports.ErrPromotionInvalid, reason)
	}
	return fmt.Errorf("%w: %s", ports.ErrPromotionNotEligible, reason)
}

func (s *Service) CompleteCheckout(ctx context.Context, sessionID string, input CompleteCheckoutInput) (*CompleteCheckoutResult, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(session.Items) == 0 {
		return nil, domain.ErrEmptyCheckout
	}

	snapshot, err := applyCompleteCheckoutInput(input)
	if err != nil {
		return nil, err
	}

	pricedItems, err := s.priceItems(ctx, session.Items)
	if err != nil {
		return nil, err
	}

	// Re-evaluate against the final priced cart and reserve the use in the
	// same call, so a cart edited after the code was applied cannot carry a
	// discount it no longer earns, and the customer's one use is claimed
	// exactly once. The reservation is keyed by order id and is idempotent, so
	// a retried complete does not burn a second use.
	// Re-evaluate against the final priced cart, so a cart edited after the
	// code was applied cannot carry a discount it no longer earns. The use is
	// claimed further down, once the order exists and has an id to key it to.
	discountKRW := int64(0)
	promotionCode := session.PromotionCode
	promoCtx := promotionContextFor(session, pricedItems)
	if promotionCode != "" {
		if s.promotionClient == nil {
			return nil, ports.ErrPromotionUnavailable
		}
		verdict, err := s.promotionClient.Evaluate(ctx, promotionCode, promoCtx)
		if err != nil {
			return nil, err
		}
		if !verdict.OK {
			return nil, promotionRejection(verdict)
		}
		discountKRW = verdict.DiscountWon
	}

	shippingFee := session.ShippingFeeWon
	order, err := s.CreateOrder(ctx, CreateOrderInput{
		CustomerID:      session.CustomerID,
		Items:           pricedItems,
		PromotionCode:      promotionCode,
		DiscountWon:     discountKRW,
		RecipientName:   snapshot.RecipientName,
		RecipientPhone:  snapshot.RecipientPhone,
		ShippingAddress: snapshot.ShippingAddress,
		SourceAddressID: snapshot.SourceAddressID,
		ShippingFeeWon:  &shippingFee,
	})
	if err != nil {
		return nil, err
	}

	// Claim the customer's use now that the order has an id to key it to. The
	// reservation re-evaluates server-side and is idempotent per order, so a
	// retried complete cannot burn a second use. A refusal here — the code was
	// spent between evaluating and now — rolls the order back rather than
	// shipping an unearned discount.
	if promotionCode != "" {
		verdict, reserveErr := s.promotionClient.Reserve(ctx, promotionCode, order.ID, promoCtx)
		if reserveErr != nil {
			_, _ = s.CancelOrder(ctx, order.ID)
			return nil, reserveErr
		}
		if !verdict.OK {
			_, _ = s.CancelOrder(ctx, order.ID)
			return nil, promotionRejection(verdict)
		}
	}

	now := s.now()
	claimed, err := s.repo.CompleteCheckoutSessionIfOpen(ctx, session.ID, order.ID, now)
	if err != nil {
		_, _ = s.CancelOrder(ctx, order.ID)
		return nil, err
	}
	if !claimed {
		_, _ = s.CancelOrder(ctx, order.ID)
		return nil, domain.ErrSessionNotOpen
	}
	session.Status = domain.CheckoutStatusCompleted
	session.OrderID = order.ID
	session.UpdatedAt = now

	return &CompleteCheckoutResult{
		Session: cloneCheckoutSession(session),
		Order:   order,
	}, nil
}

func (s *Service) getOpenCheckoutSession(ctx context.Context, sessionID string) (*domain.CheckoutSession, error) {
	session, err := s.repo.GetCheckoutSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if err := session.EnsureOpen(s.now()); err != nil {
		_ = s.repo.SaveCheckoutSession(ctx, session)
		return nil, err
	}
	return session, nil
}

func (s *Service) saveCheckoutSession(ctx context.Context, session *domain.CheckoutSession) (*domain.CheckoutSession, error) {
	if err := s.repo.SaveCheckoutSession(ctx, session); err != nil {
		return nil, err
	}
	return s.annotateCheckoutSession(ctx, cloneCheckoutSession(session)), nil
}

// annotateCheckoutSession re-checks each stored line against the product catalog
// so the storefront can surface unavailable_items before complete.
func (s *Service) annotateCheckoutSession(ctx context.Context, session *domain.CheckoutSession) *domain.CheckoutSession {
	if session == nil || len(session.Items) == 0 || s.product == nil {
		return session
	}
	unavailable := make([]domain.UnavailableItem, 0)
	for i, item := range session.Items {
		_, err := s.resolveVariant(ctx, item)
		if errors.Is(err, ports.ErrVariantNotFound) {
			available := false
			session.Items[i].Available = &available
			unavailable = append(unavailable, unavailableFromOrderItem(item))
		}
	}
	if len(unavailable) > 0 {
		session.UnavailableItems = unavailable
	} else {
		session.UnavailableItems = nil
	}
	return session
}

func cloneCheckoutSession(session *domain.CheckoutSession) *domain.CheckoutSession {
	if session == nil {
		return nil
	}
	copied := *session
	copied.Items = cloneOrderItems(session.Items)
	if session.UnavailableItems != nil {
		copied.UnavailableItems = append([]domain.UnavailableItem(nil), session.UnavailableItems...)
	}
	return &copied
}

func cloneOrderItems(items []domain.OrderItem) []domain.OrderItem {
	copied := make([]domain.OrderItem, len(items))
	copy(copied, items)
	return copied
}
