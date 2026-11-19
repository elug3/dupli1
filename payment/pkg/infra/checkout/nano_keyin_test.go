package checkout

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

// The test merchant's encData key and IV from 수기결제 v2.5 §1 (연동 테스트).
const (
	testEncKey = "Q2Jv7LkNp5X3M8Yc6rW9T1Eb4F6HdKx6"
	testEncIV  = "Nx5Lq7Kv4W8Jp6Mu"
)

func TestEncryptNanoCardData_MatchesOpenSSL(t *testing.T) {
	// printf '%s' '{"cardNo":"4111111111111111"}' |
	//   openssl enc -aes-256-cbc -K <hex key> -iv <hex iv> -base64 -A
	const want = "YThg1mHN3UX/LsemWwdlSdRtyL5fsHPaHIGLeKPLMXY="
	got, err := EncryptNanoCardData(testEncKey, testEncIV, []byte(`{"cardNo":"4111111111111111"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("encData = %q, want %q", got, want)
	}
}

func TestEncryptNanoCardData_PadsAWholeBlock(t *testing.T) {
	plain := []byte("0123456789abcdef") // exactly one block: PKCS7 adds a second
	enc, err := EncryptNanoCardData(testEncKey, testEncIV, plain)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decryptNanoCardData(testEncKey, testEncIV, enc)
	if err != nil || string(back) != string(plain) {
		t.Fatalf("round trip = %q, %v", back, err)
	}
}

func TestKeyInEnabled(t *testing.T) {
	base := NanoConfig{ShopCode: "s", LoginID: "l", APIKey: "k"}
	if base.KeyInEnabled() || base.KeyInMisconfigured() {
		t.Fatal("no key and IV: 인증결제, not misconfigured")
	}
	good := base
	good.EncKey, good.EncIV = testEncKey, testEncIV
	if !good.KeyInEnabled() {
		t.Fatal("key and IV of the right length enable key-in")
	}
	short := base
	short.EncKey, short.EncIV = "short", testEncIV
	if short.KeyInEnabled() || !short.KeyInMisconfigured() {
		t.Fatal("a short key is misconfigured and leaves key-in off")
	}
}

func TestCreateSession_KeyInHasNoCheckoutURL(t *testing.T) {
	p := NewNanoProvider(NanoConfig{ShopCode: "s", LoginID: "l", APIKey: "k", EncKey: testEncKey, EncIV: testEncIV})
	got, err := p.CreateSession(t.Context(), ports.CheckoutSessionInput{
		PaymentID: "pay_1", OrderID: "ord_1", AmountWon: 1000, OrderName: "홍길동", OrderTel: "010-1234-5678",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CheckoutURL != "" {
		t.Fatalf("checkout_url = %q, want empty (the storefront takes the card)", got.CheckoutURL)
	}
}

func testCard() domain.CardDetails {
	return domain.CardDetails{
		Number: "4111111111111111", ExpYY: "30", ExpMM: "07", Password2: "12", HolderID: "900101", Installment: "00",
	}
}

func keyInProvider(t *testing.T, handler http.HandlerFunc) *NanoProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewNanoProvider(NanoConfig{
		BaseURL: srv.URL, Ver: "240000005", ShopCode: "240000005", LoginID: "shoptest", APIKey: "api-key",
		EncKey: testEncKey, EncIV: testEncIV, HTTPClient: srv.Client(),
	})
}

func approvalInput() ports.CardApprovalInput {
	return ports.CardApprovalInput{
		PaymentID: "pay_1", OrderID: "ord_1", AmountWon: 77000,
		OrderName: "홍길동", OrderTel: "010-1234-5678", OrderEmail: "shopper@example.com",
		GoodsName: "Dupli1 ord_1", Card: testCard(),
	}
}

func TestApproveCard_SendsEncryptedCardAndParsesApproval(t *testing.T) {
	var got NanoApprovalRequest
	var raw string
	p := keyInProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/payment/approval.io" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("API_KEY") != "api-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("headers = %v", r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		raw = string(b)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(`{"resultCode":"0000","resultMsg":"","shopcode":"240000005","reqPayAmt":"77000",
			"installment":"00","cardNo":"41111111****1111","apprNo":"00647018","tranNo":"2409030071109","cardTxt":"현대","cardCode":"06","compOrderNo":"pay_1"}`))
	})
	result, err := p.ApproveCard(t.Context(), approvalInput())
	if err != nil {
		t.Fatalf("ApproveCard: %v", err)
	}
	if !result.Approved || result.TranNo != "2409030071109" || result.CardMasked != "41111111****1111" {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(raw, "4111111111111111") || strings.Contains(raw, `"12"`) || strings.Contains(raw, "900101") {
		t.Fatalf("card data in the clear on the wire: %s", raw)
	}
	if got.ShopCode != "240000005" || got.LoginID != "shoptest" || got.ReqPayAmt != "77000" ||
		got.OrderTel != "01012345678" || got.CompOrderNo != "pay_1" || got.Installment != "00" {
		t.Fatalf("body = %+v", got)
	}
	plain, err := decryptNanoCardData(testEncKey, testEncIV, got.EncData)
	if err != nil {
		t.Fatalf("decrypt encData: %v", err)
	}
	var card nanoCardData
	if err := json.Unmarshal(plain, &card); err != nil {
		t.Fatal(err)
	}
	want := nanoCardData{CardNo: "4111111111111111", CardExpYy: "30", CardExpMm: "07", CardPw: "12", CardHolderYmd: "900101"}
	if card != want {
		t.Fatalf("encData = %+v, want %+v", card, want)
	}
}

func TestApproveCard_Decline(t *testing.T) {
	p := keyInProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resultCode":"8035","resultMsg":"비밀번호 오류"}`))
	})
	result, err := p.ApproveCard(t.Context(), approvalInput())
	if err != nil {
		t.Fatalf("a decline is an answer, not an error: %v", err)
	}
	if result.Approved || result.ResultCode != "8035" || result.ResultMsg != "비밀번호 오류" {
		t.Fatalf("result = %+v", result)
	}
}

// Anything short of a parsed resultCode may have charged the card.
func TestApproveCard_NoAnswerIsUnconfirmed(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"http 500":     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"not json":     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) },
		"no code":      func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) },
		"dropped conn": func(w http.ResponseWriter, _ *http.Request) { panic(http.ErrAbortHandler) },
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			p := keyInProvider(t, func(w http.ResponseWriter, r *http.Request) { calls++; h(w, r) })
			_, err := p.ApproveCard(t.Context(), approvalInput())
			if !errors.Is(err, ports.ErrCardApprovalUnconfirmed) {
				t.Fatalf("err = %v, want ErrCardApprovalUnconfirmed", err)
			}
			if calls != 1 {
				t.Fatalf("approval sent %d times; it must never be retried", calls)
			}
		})
	}
}

func TestApproveCard_TimeoutIsUnconfirmed(t *testing.T) {
	release := make(chan struct{})
	p := keyInProvider(t, func(w http.ResponseWriter, _ *http.Request) { <-release })
	defer close(release)
	p.cfg.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	_, err := p.ApproveCard(t.Context(), approvalInput())
	if !errors.Is(err, ports.ErrCardApprovalUnconfirmed) {
		t.Fatalf("err = %v", err)
	}
}

func TestApproveCard_NotConfiguredSendsNothing(t *testing.T) {
	p := NewNanoProvider(NanoConfig{ShopCode: "s", LoginID: "l", APIKey: "k"})
	_, err := p.ApproveCard(t.Context(), approvalInput())
	if !errors.Is(err, ports.ErrMethodUnavailable) || errors.Is(err, ports.ErrCardApprovalUnconfirmed) {
		t.Fatalf("err = %v", err)
	}
}

func TestMaskedCardOnly(t *testing.T) {
	if got := maskedCardOnly("4111111111111111"); got != "" {
		t.Fatalf("an unmasked number must be dropped, got %q", got)
	}
	if got := maskedCardOnly(" 41111111****1111 "); got != "41111111****1111" {
		t.Fatalf("got %q", got)
	}
}
