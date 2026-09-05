package checkout

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

func cancelTestProvider(baseURL string) *NanoProvider {
	return NewNanoProvider(NanoConfig{
		BaseURL:       baseURL,
		Ver:           "240000005",
		ShopCode:      "240000005",
		LoginID:       "shoptest",
		APIKey:        "test-key",
		PublicBaseURL: "https://dupli1.com",
	})
}

func TestBuildCancelRequest(t *testing.T) {
	p := cancelTestProvider("https://dev3.nanopay.co.kr")

	url, body, err := p.BuildCancelRequest("2409030071109", "pay_000001", 70000)
	if err != nil {
		t.Fatalf("BuildCancelRequest: %v", err)
	}
	if want := "https://dev3.nanopay.co.kr/api/payment/cancel.io"; url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}
	if body.TranNo != "2409030071109" {
		t.Fatalf("tranNo = %q", body.TranNo)
	}
	if body.CancelAmt != "70000" {
		t.Fatalf("cancelAmt = %q, want the amount to cancel as a string", body.CancelAmt)
	}
	if body.CompOrderNo != "pay_000001" {
		t.Fatalf("compOrderNo = %q, want the dupli1 payment id", body.CompOrderNo)
	}
	if body.ShopCode != "240000005" || body.LoginID != "shoptest" {
		t.Fatalf("merchant fields = %q/%q", body.ShopCode, body.LoginID)
	}
}

// The cancel endpoint authenticates with the API_KEY header, not the body
// hashValue the cert request uses. Serialising a hashValue/timestamp here would
// be wrong per 수기결제 v2.5 §3.
func TestBuildCancelRequest_CarriesNoHashOrTimestamp(t *testing.T) {
	p := cancelTestProvider("https://dev3.nanopay.co.kr")
	_, body, err := p.BuildCancelRequest("2409030071109", "pay_000001", 70000)
	if err != nil {
		t.Fatalf("BuildCancelRequest: %v", err)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{"hashValue", "timestamp", "encData"} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("cancel body must not carry %s: %s", field, raw)
		}
	}
}

func TestBuildCancelRequest_RequiresTranNo(t *testing.T) {
	p := cancelTestProvider("https://dev3.nanopay.co.kr")
	_, _, err := p.BuildCancelRequest("  ", "pay_000001", 70000)
	if !errors.Is(err, ports.ErrCancelUnsupported) {
		t.Fatalf("err = %v, want ErrCancelUnsupported", err)
	}
}

func TestBuildCancelRequest_RejectsNonPositiveAmount(t *testing.T) {
	p := cancelTestProvider("https://dev3.nanopay.co.kr")
	if _, _, err := p.BuildCancelRequest("2409030071109", "pay_000001", 0); !errors.Is(err, domain.ErrCancelAmountInvalid) {
		t.Fatalf("err = %v, want ErrCancelAmountInvalid", err)
	}
}

func TestCancelPayment_UnconfiguredProvider(t *testing.T) {
	p := NewNanoProvider(NanoConfig{})
	_, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409030071109", PaymentID: "pay_000001", AmountCents: 70000,
	})
	if !errors.Is(err, ports.ErrCancelUnsupported) {
		t.Fatalf("err = %v, want ErrCancelUnsupported", err)
	}
}

func TestCancelPayment_Success(t *testing.T) {
	var gotAPIKey, gotPath, gotContentType string
	var gotBody NanoCancelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("API_KEY")
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		// Response shape from 수기결제 v2.5 §3.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resultCode": "0000",
			"resultMsg": "정상",
			"shopcode": "240000005",
			"cancelDate": "20260905",
			"cancelTime": "155242",
			"cancelAmt": "70000",
			"apprTranNo": "2409030071109",
			"remainAmt": "0",
			"compOrderNo": "pay_000001"
		}`))
	}))
	defer srv.Close()

	p := cancelTestProvider(srv.URL)
	res, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409030071109", PaymentID: "pay_000001", AmountCents: 70000,
	})
	if err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if gotPath != "/api/payment/cancel.io" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("API_KEY header = %q, want the merchant key", gotAPIKey)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if gotBody.CancelAmt != "70000" || gotBody.TranNo != "2409030071109" {
		t.Fatalf("body = %+v", gotBody)
	}
	if res.CanceledAmountCents != 70000 {
		t.Fatalf("canceled = %d, want 70000", res.CanceledAmountCents)
	}
	if !res.RemainingKnown || res.RemainingCents != 0 {
		t.Fatalf("remaining = %d (known=%t), want 0 known", res.RemainingCents, res.RemainingKnown)
	}
	if res.ProviderRef != "2409030071109" {
		t.Fatalf("provider ref = %q", res.ProviderRef)
	}
	if res.CanceledAt != "20260905155242" {
		t.Fatalf("canceled at = %q", res.CanceledAt)
	}
}

func TestCancelPayment_PartialReportsRemaining(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"resultCode":"0000","cancelAmt":"20000","remainAmt":"50000","apprTranNo":"2409030071109"}`))
	}))
	defer srv.Close()

	p := cancelTestProvider(srv.URL)
	res, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409030071109", PaymentID: "pay_000001", AmountCents: 20000,
	})
	if err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if res.CanceledAmountCents != 20000 {
		t.Fatalf("canceled = %d, want 20000", res.CanceledAmountCents)
	}
	if !res.RemainingKnown || res.RemainingCents != 50000 {
		t.Fatalf("remaining = %d (known=%t), want 50000 known", res.RemainingCents, res.RemainingKnown)
	}
}

// 수기결제 v2.5 documents remainAmt in the field table but omits it from the
// example response, so an absent value must read as unknown rather than zero —
// zero would look like a completed full cancel.
func TestCancelPayment_MissingRemainAmtIsUnknownNotZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"resultCode": "0000",
			"resultMsg": "정상",
			"shopcode": "230000002",
			"cancelDate": "20240904",
			"cancelTime": "155242",
			"cancelAmt": "1004",
			"apprTranNo": "2409040000019",
			"compOrderNo": ""
		}`))
	}))
	defer srv.Close()

	p := cancelTestProvider(srv.URL)
	res, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409040000019", PaymentID: "pay_000001", AmountCents: 1004,
	})
	if err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if res.RemainingKnown {
		t.Fatal("absent remainAmt must not report a known remaining balance")
	}
	if res.CanceledAmountCents != 1004 {
		t.Fatalf("canceled = %d, want 1004", res.CanceledAmountCents)
	}
}

func TestCancelPayment_RejectedByProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"resultCode":"9999","resultMsg":"취소 불가 거래"}`))
	}))
	defer srv.Close()

	p := cancelTestProvider(srv.URL)
	_, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409030071109", PaymentID: "pay_000001", AmountCents: 70000,
	})
	if !errors.Is(err, ports.ErrCancelRejected) {
		t.Fatalf("err = %v, want ErrCancelRejected", err)
	}
	if !strings.Contains(err.Error(), "취소 불가 거래") {
		t.Fatalf("err must carry the provider message, got %v", err)
	}
}

func TestCancelPayment_HTTPErrorIsRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := cancelTestProvider(srv.URL)
	_, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409030071109", PaymentID: "pay_000001", AmountCents: 70000,
	})
	if !errors.Is(err, ports.ErrCancelRejected) {
		t.Fatalf("err = %v, want ErrCancelRejected", err)
	}
}

// A 200 carrying HTML (a gateway error page) must never read as success.
func TestCancelPayment_NonJSONBodyIsRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>error</body></html>`))
	}))
	defer srv.Close()

	p := cancelTestProvider(srv.URL)
	_, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{
		ProviderRef: "2409030071109", PaymentID: "pay_000001", AmountCents: 70000,
	})
	if !errors.Is(err, ports.ErrCancelRejected) {
		t.Fatalf("err = %v, want ErrCancelRejected", err)
	}
}

func TestUnavailableProviderCancelUnsupported(t *testing.T) {
	p := NewUnavailableProvider("")
	_, err := p.CancelPayment(t.Context(), ports.CancelPaymentInput{AmountCents: 100})
	if !errors.Is(err, ports.ErrCancelUnsupported) {
		t.Fatalf("err = %v, want ErrCancelUnsupported", err)
	}
}

// A NANO payment can reach succeeded with the checkout-time placeholder still
// in provider_ref if the approval callback omitted tranNo. Sending that
// placeholder as a tranNo would earn a misleading rejection from NANO, so it
// must be refused locally as unsupported instead.
func TestBuildCancelRequest_RejectsPlaceholderProviderRef(t *testing.T) {
	p := cancelTestProvider("https://dev3.nanopay.co.kr")
	placeholder := PlaceholderProviderRef("pay_000001")

	_, _, err := p.BuildCancelRequest(placeholder, "pay_000001", 70000)
	if !errors.Is(err, ports.ErrCancelUnsupported) {
		t.Fatalf("err = %v, want ErrCancelUnsupported", err)
	}
	if !strings.Contains(err.Error(), "NANO console") {
		t.Fatalf("error should tell ops where to refund, got %v", err)
	}
}

// The ref stored at checkout must be recognisable as a placeholder, so the two
// cannot drift apart.
func TestPlaceholderProviderRefRoundTrip(t *testing.T) {
	p := cancelTestProvider("https://dev3.nanopay.co.kr")
	sess, err := p.CreateSession(t.Context(), ports.CheckoutSessionInput{
		OrderID: "ord_1", PaymentID: "pay_000001", AmountCents: 70000,
		OrderName: "윤라희", OrderTel: "010-1234-5678",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if !IsPlaceholderProviderRef(sess.ProviderRef) {
		t.Fatalf("checkout ref %q must read as a placeholder", sess.ProviderRef)
	}
	if IsPlaceholderProviderRef("2409030071109") {
		t.Fatal("a real tranNo must not read as a placeholder")
	}
}
