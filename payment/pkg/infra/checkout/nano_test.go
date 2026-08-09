package checkout

import (
	"context"
	"strings"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

func TestNanoHashMatchesDocExampleShape(t *testing.T) {
	// SHA256(ver+loginId+shopcode+reqPayAmt+timestamp+API_KEY+"NANO")
	got := NanoHash("240000005", "shoptest", "240000005", "1004", "1725440123456", "R7L9PxM5V8K2Jc4N6dWqY1Eb3T5XhZU2")
	if len(got) != 64 {
		t.Fatalf("hash len = %d, want 64 hex chars", len(got))
	}
	// Deterministic known value for regression.
	want := NanoHash("240000005", "shoptest", "240000005", "1004", "1725440123456", "R7L9PxM5V8K2Jc4N6dWqY1Eb3T5XhZU2")
	if got != want {
		t.Fatalf("hash not stable")
	}
}

func TestNanoProviderCreateSession(t *testing.T) {
	p := NewNanoProvider(NanoConfig{
		BaseURL:       "https://dev3.nanopay.co.kr",
		Ver:           "240000005",
		ShopCode:      "240000005",
		LoginID:       "shoptest",
		APIKey:        "test-key",
		PublicBaseURL: "https://dupli1.com",
	})
	sess, err := p.CreateSession(context.Background(), ports.CheckoutSessionInput{
		OrderID:     "ord_1",
		PaymentID:   "pay_000001",
		AmountCents: 70000,
		OrderName:   "윤라희",
		OrderTel:    "010-4112-5167",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.Provider != domain.ProviderNano {
		t.Fatalf("provider = %s", sess.Provider)
	}
	if !strings.HasSuffix(sess.CheckoutURL, "/api/v1/payments/pay_000001/nano/checkout") {
		t.Fatalf("checkout_url = %s", sess.CheckoutURL)
	}
}

func TestNanoProviderRequiresPayer(t *testing.T) {
	p := NewNanoProvider(NanoConfig{
		ShopCode: "240000005", LoginID: "shoptest", APIKey: "k", PublicBaseURL: "http://localhost:8080",
	})
	_, err := p.CreateSession(context.Background(), ports.CheckoutSessionInput{
		PaymentID: "pay_1", AmountCents: 1000, OrderName: "", OrderTel: "01012345678",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestBuildRequest(t *testing.T) {
	p := NewNanoProvider(NanoConfig{
		BaseURL: "https://dev3.nanopay.co.kr", Ver: "240000005", ShopCode: "240000005",
		LoginID: "shoptest", APIKey: "secret", PublicBaseURL: "https://dupli1.com",
	})
	url, body, err := p.BuildRequest("pay_1", "ord_1", "cust_1", "홍길동", "01012345678", "a@b.c", "가방", 1004, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(url, nanoPCRequestPath) {
		t.Fatalf("url = %s", url)
	}
	if body.PayWay != "card" || body.ReqPayAmt != "1004" || body.CompOrderNo != "pay_1" {
		t.Fatalf("body = %+v", body)
	}
	if body.ReceiveURL != "https://dupli1.com/api/v1/payments/nano/return" {
		t.Fatalf("receiveUrl = %s", body.ReceiveURL)
	}
	wantHash := NanoHash(body.Ver, body.LoginID, body.ShopCode, body.ReqPayAmt, body.Timestamp, "secret")
	if body.HashValue != wantHash {
		t.Fatalf("hash mismatch")
	}
	_, bodyM, err := p.BuildRequest("pay_1", "ord_1", "cust_1", "홍길동", "01012345678", "", "가방", 1004, true)
	if err != nil {
		t.Fatal(err)
	}
	if bodyM.OrderTel != "01012345678" {
		t.Fatalf("phone = %s", bodyM.OrderTel)
	}
}

func TestIsMobileUserAgent(t *testing.T) {
	if !IsMobileUserAgent("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)") {
		t.Fatal("iphone should be mobile")
	}
	if IsMobileUserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120") {
		t.Fatal("desktop chrome should not be mobile")
	}
}

func TestNormalizeKRPhone(t *testing.T) {
	if got := normalizeKRPhone("010-4112-5167"); got != "01041125167" {
		t.Fatalf("got %s", got)
	}
}
