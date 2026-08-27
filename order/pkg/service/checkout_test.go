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

type fakeCouponClient struct {
	code     string
	discount float64
	err      error
}

func (f *fakeCouponClient) Redeem(ctx context.Context, code string) (*ports.Coupon, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ports.Coupon{
		Code:             f.code,
		DiscountFraction: f.discount,
	}, nil
}

func TestCheckoutSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	svc := service.NewWithCheckout(repo, stock, &fakeCouponClient{
		code:     "SUMMER30",
		discount: 0.30,
	}, 0).WithProduct(&fakeProduct{defaultCents: 5000})

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
		SKU: "bag-1", Quantity: 2, UnitPriceCents: 1, // client price ignored
	})
	if err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}
	if session.SubtotalCents != 10000 || session.TotalCents != 10000 {
		t.Fatalf("session totals = %d/%d, want 10000/10000", session.SubtotalCents, session.TotalCents)
	}

	session, err = svc.ApplyCheckoutCoupon(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("ApplyCheckoutCoupon returned error: %v", err)
	}
	if session.DiscountCents != 3000 || session.TotalCents != 7000 {
		t.Fatalf("discounted totals = %d/%d, want 3000/7000", session.DiscountCents, session.TotalCents)
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
	if result.Order.TotalCents != 7000 {
		t.Fatalf("order total = %d, want 7000", result.Order.TotalCents)
	}
	if result.Order.CouponCode != "SUMMER30" {
		t.Fatalf("order coupon = %q, want SUMMER30", result.Order.CouponCode)
	}
	if result.Order.RecipientName != "Test User" || result.Order.RecipientPhone != "01012345678" {
		t.Fatalf("order fulfillment: %+v", result.Order)
	}
	if stock.reservationID != "res-checkout" {
		t.Fatalf("stock reservation = %q, want res-checkout", stock.reservationID)
	}

	_, err = svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-2", Quantity: 1, UnitPriceCents: 1000,
	})
	if !errors.Is(err, domain.ErrSessionNotOpen) {
		t.Fatalf("UpsertCheckoutItem on completed session error = %v, want ErrSessionNotOpen", err)
	}
}

func TestCompleteCheckoutPersistsPCCC(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{reservationID: "res-pccc"}, nil, 0).WithProduct(&fakeProduct{defaultCents: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 1,
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
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{defaultCents: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 1,
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
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{defaultCents: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 1,
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
	ctx := context.Background()
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

func TestApplyCouponWithoutClientReturnsUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0).WithProduct(&fakeProduct{defaultCents: 1000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	_, err = svc.ApplyCheckoutCoupon(ctx, session.ID, "SUMMER30")
	if !errors.Is(err, ports.ErrCouponUnavailable) {
		t.Fatalf("ApplyCheckoutCoupon error = %v, want ErrCouponUnavailable", err)
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

func TestCompleteCheckoutRecomputesCouponDiscountAfterRepricing(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	product := &mutableProduct{price: 10000}
	svc := service.NewWithCheckout(repo, stock, &fakeCouponClient{
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
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 1,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}
	session, err = svc.ApplyCheckoutCoupon(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("ApplyCheckoutCoupon returned error: %v", err)
	}
	if session.DiscountCents != 3000 || session.TotalCents != 7000 {
		t.Fatalf("session discounted totals = %d/%d, want 3000/7000", session.DiscountCents, session.TotalCents)
	}

	product.price = 3000

	result, err := svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	if err != nil {
		t.Fatalf("CompleteCheckout returned error: %v", err)
	}
	if result.Order.SubtotalCents != 3000 {
		t.Fatalf("order subtotal = %d, want 3000", result.Order.SubtotalCents)
	}
	if result.Order.DiscountCents != 900 {
		t.Fatalf("order discount = %d, want 900 (30%% of repriced subtotal)", result.Order.DiscountCents)
	}
	if result.Order.TotalCents != 2100 {
		t.Fatalf("order total = %d, want 2100", result.Order.TotalCents)
	}
}

type mutableProduct struct {
	price int64
}

func (m *mutableProduct) GetVariant(_ context.Context, sku string) (*ports.VariantInfo, error) {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	return &ports.VariantInfo{SkuID: "ID-" + sku, SKU: sku, UnitPriceCents: m.price}, nil
}

func (m *mutableProduct) GetVariantBySkuID(_ context.Context, skuID string) (*ports.VariantInfo, error) {
	skuID = strings.TrimSpace(skuID)
	return &ports.VariantInfo{SkuID: skuID, SKU: strings.ToUpper(skuID), UnitPriceCents: m.price}, nil
}

func TestCompleteCheckoutRejectsSecondComplete(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	svc := service.NewWithCheckout(repo, stock, nil, 0).WithProduct(&fakeProduct{defaultCents: 5000})

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 5000,
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
	ctx := context.Background()
	product := &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"BAG-OK": {SkuID: "ID-BAG-OK", SKU: "BAG-OK", UnitPriceCents: 5000},
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
	ctx := context.Background()
	product := &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"BAG-1":    {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceCents: 5000},
			"ID-BAG-1": {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceCents: 5000},
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
	ctx := context.Background()
	product := &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"BAG-1":    {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceCents: 5000},
			"ID-BAG-1": {SkuID: "ID-BAG-1", SKU: "BAG-1", UnitPriceCents: 5000},
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
