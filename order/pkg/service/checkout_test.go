package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

// fakePromotionClient stands in for product. It applies `discount` as a
// fraction of the cart it is handed, which is what the real evaluator does for
// a percentage benefit — the point being that the amount is computed from the
// priced lines, not taken from the caller.
type fakePromotionClient struct {
	code     string
	discount float64
	err      error

	reserved map[string]string // orderID -> code
	released map[string]int
	consumed map[string]int

	// refuse, when set, is returned instead of a successful evaluation.
	refuse *ports.PromotionEvaluation
}

func (f *fakePromotionClient) evaluate(promoCtx ports.PromotionContext) *ports.PromotionEvaluation {
	if f.refuse != nil {
		return f.refuse
	}
	var subtotal int64
	for _, line := range promoCtx.Lines {
		subtotal += int64(line.Quantity) * line.UnitPriceWon
	}
	return &ports.PromotionEvaluation{
		OK:                  true,
		Code:                f.code,
		DiscountWon:         int64(float64(subtotal) * f.discount),
		EligibleSubtotalWon: subtotal,
	}
}

func (f *fakePromotionClient) Evaluate(_ context.Context, _ string, promoCtx ports.PromotionContext) (*ports.PromotionEvaluation, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.evaluate(promoCtx), nil
}

func (f *fakePromotionClient) Reserve(_ context.Context, code, orderID string, promoCtx ports.PromotionContext) (*ports.PromotionEvaluation, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.reserved == nil {
		f.reserved = map[string]string{}
	}
	f.reserved[orderID] = code
	return f.evaluate(promoCtx), nil
}

func (f *fakePromotionClient) Consume(_ context.Context, orderID string) error {
	if f.consumed == nil {
		f.consumed = map[string]int{}
	}
	f.consumed[orderID]++
	return nil
}

func (f *fakePromotionClient) Release(_ context.Context, orderID string) error {
	if f.released == nil {
		f.released = map[string]int{}
	}
	f.released[orderID]++
	return nil
}

func TestCheckoutSessionLifecycle(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	svc := service.NewWithCheckout(repo, stock, &fakePromotionClient{
		code:     "SUMMER30",
		discount: 0.30,
	}, 0).WithProduct(&fakeProduct{defaultKRW: 5000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if session.Status != domain.CheckoutStatusOpen {
		t.Fatalf("session status = %q, want open", session.Status)
	}

	session, err = svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 2, UnitPriceWon: 1, // client price ignored
	})
	if err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}
	if session.SubtotalWon != 10000 || session.TotalWon != 10000 {
		t.Fatalf("session totals = %d/%d, want 10000/10000", session.SubtotalWon, session.TotalWon)
	}

	session, err = svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("ApplyCheckoutPromotion returned error: %v", err)
	}
	if session.DiscountWon != 3000 || session.TotalWon != 7000 {
		t.Fatalf("discounted totals = %d/%d, want 3000/7000", session.DiscountWon, session.TotalWon)
	}

	result, err := svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	if err != nil {
		t.Fatalf("CompleteCheckout returned error: %v", err)
	}
	if result.Session.Status != domain.CheckoutStatusCompleted {
		t.Fatalf("session status = %q, want completed", result.Session.Status)
	}
	if result.Order.Status != domain.StatusPending {
		t.Fatalf("order status = %q, want pending", result.Order.Status)
	}
	if result.Order.TotalWon != 7000 {
		t.Fatalf("order total = %d, want 7000", result.Order.TotalWon)
	}
	if result.Order.PromotionCode != "SUMMER30" {
		t.Fatalf("order promotion = %q, want SUMMER30", result.Order.PromotionCode)
	}
	if result.Order.RecipientName != "Test User" || result.Order.RecipientPhone != "01012345678" {
		t.Fatalf("order fulfillment: %+v", result.Order)
	}
	if stock.reservationID != "res-checkout" {
		t.Fatalf("stock reservation = %q, want res-checkout", stock.reservationID)
	}

	_, err = svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-2", Quantity: 1, UnitPriceWon: 1000,
	})
	if !errors.Is(err, domain.ErrSessionNotOpen) {
		t.Fatalf("UpsertCheckoutItem on completed session error = %v, want ErrSessionNotOpen", err)
	}
}

func TestCompleteCheckoutPersistsPCCC(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{reservationID: "res-pccc"}, nil, 0).WithProduct(&fakeProduct{defaultKRW: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceWon: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	input := testCompleteCheckoutInput()
	input.ShippingAddress.PCCC = "p123456789012"

	result, err := svc.CompleteCheckout(ctx, session.ID, input)
	if err != nil {
		t.Fatalf("CompleteCheckout returned error: %v", err)
	}
	if result.Order.ShippingAddress.PCCC != "P123456789012" {
		t.Fatalf("order shipping pccc = %q, want normalized P123456789012", result.Order.ShippingAddress.PCCC)
	}
}

func TestCompleteCheckoutRejectsMalformedPCCC(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{defaultKRW: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceWon: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	input := testCompleteCheckoutInput()
	input.ShippingAddress.PCCC = "P12345"

	_, err = svc.CompleteCheckout(ctx, session.ID, input)
	if !errors.Is(err, domain.ErrInvalidFulfillment) {
		t.Fatalf("CompleteCheckout error = %v, want ErrInvalidFulfillment", err)
	}
}

func TestCompleteCheckoutRejectsInvalidFulfillment(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{defaultKRW: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceWon: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	_, err = svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName:  "",
		RecipientPhone: "01012345678",
		ShippingAddress: domain.ShippingAddress{
			PostalCode:   "06194",
			AddressLine1: "테헤란로 78길 14-12",
			City:         "강남구",
			Province:     "서울특별시",
		},
	})
	if !errors.Is(err, domain.ErrInvalidFulfillment) {
		t.Fatalf("CompleteCheckout error = %v, want ErrInvalidFulfillment", err)
	}
}

func TestCompleteCheckoutRequiresItems(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}

	_, err = svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{})
	if !errors.Is(err, domain.ErrEmptyCheckout) {
		t.Fatalf("CompleteCheckout error = %v, want ErrEmptyCheckout", err)
	}
}

func TestApplyPromotionWithoutClientReturnsUnavailable(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{defaultKRW: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceWon: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	_, err = svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30")
	if !errors.Is(err, ports.ErrPromotionUnavailable) {
		t.Fatalf("ApplyCheckoutPromotion error = %v, want ErrPromotionUnavailable", err)
	}
}

func testCompleteCheckoutInput() service.CompleteCheckoutInput {
	return service.CompleteCheckoutInput{
		RecipientName:  "Test User",
		RecipientPhone: "01012345678",
		ShippingAddress: domain.ShippingAddress{
			PostalCode:   "06194",
			AddressLine1: "테헤란로 78길 14-12",
			City:         "강남구",
			Province:     "서울특별시",
		},
	}
}

func TestCompleteCheckoutRecomputesPromotionDiscountAfterRepricing(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	product := &mutableProduct{price: 10000}
	svc := service.NewWithCheckout(repo, stock, &fakePromotionClient{
		code:     "SUMMER30",
		discount: 0.30,
	}, 0).WithProduct(product)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceWon: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}
	session, err = svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("ApplyCheckoutPromotion returned error: %v", err)
	}
	if session.DiscountWon != 3000 || session.TotalWon != 7000 {
		t.Fatalf("session discounted totals = %d/%d, want 3000/7000", session.DiscountWon, session.TotalWon)
	}

	product.price = 3000

	result, err := svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	if err != nil {
		t.Fatalf("CompleteCheckout returned error: %v", err)
	}
	if result.Order.SubtotalWon != 3000 {
		t.Fatalf("order subtotal = %d, want 3000", result.Order.SubtotalWon)
	}
	if result.Order.DiscountWon != 900 {
		t.Fatalf("order discount = %d, want 900 (30%% of repriced subtotal)", result.Order.DiscountWon)
	}
	if result.Order.TotalWon != 2100 {
		t.Fatalf("order total = %d, want 2100", result.Order.TotalWon)
	}
}

type mutableProduct struct {
	price int64
}

func (m *mutableProduct) GetVariant(_ context.Context, sku string) (*ports.VariantInfo, error) {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	return &ports.VariantInfo{SkuID: "ID-" + sku, SKU: sku, UnitPriceWon: m.price}, nil
}

func (m *mutableProduct) GetVariantBySkuID(_ context.Context, skuID string) (*ports.VariantInfo, error) {
	skuID = strings.TrimSpace(skuID)
	return &ports.VariantInfo{SkuID: skuID, SKU: strings.ToUpper(skuID), UnitPriceWon: m.price}, nil
}

func TestCompleteCheckoutRejectsSecondComplete(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	svc := service.NewWithCheckout(repo, stock, nil, 0).WithProduct(&fakeProduct{defaultKRW: 5000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceWon: 5000,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	first, err := svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	if err != nil {
		t.Fatalf("first CompleteCheckout returned error: %v", err)
	}

	_, err = svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	if !errors.Is(err, domain.ErrSessionNotOpen) {
		t.Fatalf("second CompleteCheckout error = %v, want ErrSessionNotOpen", err)
	}

	orders, err := repo.ListByCustomer(ctx, "customer-1")
	if err != nil {
		t.Fatalf("ListByCustomer returned error: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("customer order count = %d, want 1", len(orders))
	}
	if orders[0].ID != first.Order.ID {
		t.Fatalf("order id = %q, want %q", orders[0].ID, first.Order.ID)
	}
}

func TestSetCheckoutItems_CollectsAllUnavailable(t *testing.T) {
	ctx := t.Context()
	product := &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"BAG-OK": {SkuID: "ID-BAG-OK", SKU: "BAG-OK", UnitPriceWon: 5000},
		},
		strictMissing: true,
	}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{}, nil, 0).WithProduct(product)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}

	_, err = svc.SetCheckoutItems(ctx, session.ID, []domain.OrderItem{
		{SkuID: "BAD-1", Quantity: 1},
		{SKU: "BAG-OK", Quantity: 1},
		{SkuID: "BAD-2", SKU: "BAD-SKU-2", Quantity: 2},
	})
	var unavailable *service.UnavailableVariantsError
	if !errors.As(err, &unavailable) {
		t.Fatalf("want UnavailableVariantsError, got %v", err)
	}
	if len(unavailable.Items) != 2 {
		t.Fatalf("want 2 unavailable items, got %+v", unavailable.Items)
	}
	if unavailable.Items[0].SkuID != "BAD-1" || unavailable.Items[1].SkuID != "BAD-2" {
		t.Fatalf("unexpected unavailable: %+v", unavailable.Items)
	}
	if unavailable.Error() != "variant not found" {
		t.Fatalf("error = %q", unavailable.Error())
	}
}

func TestGetCheckoutSession_ReportsUnavailableItems(t *testing.T) {
	ctx := t.Context()
	product := &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"BAG-1":    {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceWon: 5000},
			"ID-BAG-1": {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceWon: 5000},
		},
		strictMissing: true,
	}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{}, nil, 0).WithProduct(product)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	session, err = svc.SetCheckoutItems(ctx, session.ID, []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1},
	})
	if err != nil {
		t.Fatalf("SetCheckoutItems: %v", err)
	}

	delete(product.byKey, "BAG-1")
	delete(product.byKey, "ID-BAG-1")

	got, err := svc.GetCheckoutSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetCheckoutSession: %v", err)
	}
	if len(got.UnavailableItems) != 1 {
		t.Fatalf("want 1 unavailable item, got %+v", got.UnavailableItems)
	}
	if got.UnavailableItems[0].SkuID != "ID-BAG-1" || got.UnavailableItems[0].Reason != domain.ReasonVariantNotFound {
		t.Fatalf("unexpected unavailable: %+v", got.UnavailableItems[0])
	}
	if got.Items[0].Available == nil || *got.Items[0].Available {
		t.Fatalf("want available=false, got %+v", got.Items[0])
	}
}

func TestCompleteCheckout_UnavailableVariants(t *testing.T) {
	ctx := t.Context()
	product := &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"BAG-1":    {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceWon: 5000},
			"ID-BAG-1": {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceWon: 5000},
		},
		strictMissing: true,
	}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{}, nil, 0).WithProduct(product)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if _, err := svc.SetCheckoutItems(ctx, session.ID, []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1},
	}); err != nil {
		t.Fatalf("SetCheckoutItems: %v", err)
	}

	delete(product.byKey, "BAG-1")
	delete(product.byKey, "ID-BAG-1")

	_, err = svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	var unavailable *service.UnavailableVariantsError
	if !errors.As(err, &unavailable) || len(unavailable.Items) != 1 {
		t.Fatalf("want UnavailableVariantsError with 1 item, got %v", err)
	}
	if unavailable.Items[0].SkuID != "ID-BAG-1" {
		t.Fatalf("unexpected unavailable: %+v", unavailable.Items[0])
	}
}

// ── Promotional code lifecycle (Phase 2) ─────────────────────────────────────

// newPromoSvc wires a checkout service with a promotion client that records
// what order asked of it.
func newPromoSvc(t *testing.T) (*service.Service, *fakePromotionClient) {
	t.Helper()
	promo := &fakePromotionClient{code: "SUMMER30", discount: 0.30}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{reservationID: "res-1"}, promo, 0).
		WithProduct(&fakeProduct{defaultKRW: 10000})
	return svc, promo
}

func openSessionWithItem(t *testing.T, svc *service.Service) *domain.CheckoutSession {
	t.Helper()
	ctx := t.Context()
	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{CustomerID: "customer-1"})
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SkuID: "sku-1", SKU: "BAG-1", Quantity: 1, UnitPriceWon: 10000,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem: %v", err)
	}
	return session
}

// Completing a checkout claims the customer's use against the order's own id,
// so the ledger can key it and a retry cannot burn a second use.
func TestCompleteCheckoutReservesThePromotionAgainstTheOrder(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	session := openSessionWithItem(t, svc)

	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("ApplyCheckoutPromotion: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName: "Kim", RecipientPhone: "010-0000-0000",
		ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
	})
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	if got := promo.reserved[result.Order.ID]; got != "SUMMER30" {
		t.Fatalf("reserved[%s] = %q, want SUMMER30", result.Order.ID, got)
	}
}

// Paying an order spends the use.
func TestMarkOrderPaidConsumesThePromotion(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	session := openSessionWithItem(t, svc)
	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName: "Kim", RecipientPhone: "010-0000-0000",
		ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, result.Order.ID, "pay-1", result.Order.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	if promo.consumed[result.Order.ID] != 1 {
		t.Fatalf("consumed = %d, want 1", promo.consumed[result.Order.ID])
	}
}

// A pending cancel releases the promotion; a late payment must consume it again
// so the customer cannot checkout twice with the same single-use code.
func TestMarkOrderPaidConsumesPromotionAfterPendingCancel(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	session := openSessionWithItem(t, svc)
	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName: "Kim", RecipientPhone: "010-0000-0000",
		ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.CancelOrder(ctx, result.Order.ID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if promo.released[result.Order.ID] != 1 {
		t.Fatalf("released = %d, want 1", promo.released[result.Order.ID])
	}
	if _, err := svc.MarkOrderPaid(ctx, result.Order.ID, "pay-late", result.Order.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	if promo.consumed[result.Order.ID] != 1 {
		t.Fatalf("consumed = %d, want 1 after late payment", promo.consumed[result.Order.ID])
	}
}

// Cancelling before shipment hands the use back, exactly as it hands stock
// back. This is the rule the plan pins: nothing is really spent until the
// goods have gone.
func TestCancelBeforeShipmentReleasesThePromotion(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	session := openSessionWithItem(t, svc)
	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName: "Kim", RecipientPhone: "010-0000-0000",
		ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.CancelOrder(ctx, result.Order.ID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if promo.released[result.Order.ID] != 1 {
		t.Fatalf("released = %d, want 1 for a pre-shipment cancel", promo.released[result.Order.ID])
	}
}

// Applying then removing a code leaves the session at full price, and does not
// leave a stale code behind for complete to re-apply.
func TestClearCheckoutPromotionRemovesTheDiscount(t *testing.T) {
	ctx := t.Context()
	svc, _ := newPromoSvc(t)
	session := openSessionWithItem(t, svc)

	applied, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.DiscountWon != 3000 {
		t.Fatalf("discount = %d, want 3000", applied.DiscountWon)
	}

	cleared, err := svc.ClearCheckoutPromotion(ctx, session.ID)
	if err != nil {
		t.Fatalf("ClearCheckoutPromotion: %v", err)
	}
	if cleared.PromotionCode != "" {
		t.Fatalf("promotion_code = %q, want empty", cleared.PromotionCode)
	}
	if cleared.DiscountWon != 0 {
		t.Fatalf("discount = %d, want 0 after removing the code", cleared.DiscountWon)
	}
	if cleared.TotalWon != cleared.SubtotalWon+cleared.ShippingFeeWon {
		t.Fatalf("total = %d, want subtotal+shipping = %d", cleared.TotalWon, cleared.SubtotalWon+cleared.ShippingFeeWon)
	}
}

// A code refused for this cart must not be applied, and the refusal must carry
// its reason rather than surfacing as a generic failure.
func TestApplyRefusedPromotionSurfacesTheReason(t *testing.T) {
	ctx := t.Context()
	promo := &fakePromotionClient{code: "SPEND100K", refuse: &ports.PromotionEvaluation{
		Reason: "not_eligible", SubReason: "min_spend",
	}}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{reservationID: "res-1"}, promo, 0).
		WithProduct(&fakeProduct{defaultKRW: 10000})
	session := openSessionWithItem(t, svc)

	_, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SPEND100K")
	if err == nil {
		t.Fatal("an ineligible cart must not get the discount")
	}
	if !errors.Is(err, ports.ErrPromotionNotEligible) {
		t.Fatalf("err = %v, want ErrPromotionNotEligible", err)
	}
	if !strings.Contains(err.Error(), "min_spend") {
		t.Fatalf("err = %v, want the sub-reason to travel with it", err)
	}
}
