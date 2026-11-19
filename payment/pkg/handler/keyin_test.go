package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

const keyInSecret = "keyin-secret"

type keyInOrders struct{}

func (keyInOrders) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	return &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalWon: 77000,
		RecipientName: "홍길동", RecipientPhone: "01012345678",
	}, nil
}

// newKeyInServer wires payment over a fake NANO approval.io that answers with
// whatever nanoBody holds.
func newKeyInServer(t *testing.T, nanoBody *atomic.Value) (*http.ServeMux, string, *int32) {
	t.Helper()
	var calls int32
	nano := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(nanoBody.Load().(string)))
	}))
	t.Cleanup(nano.Close)
	provider := checkout.NewNanoProvider(checkout.NanoConfig{
		BaseURL: nano.URL, ShopCode: "240000005", LoginID: "shoptest", APIKey: "k",
		EncKey: "Q2Jv7LkNp5X3M8Yc6rW9T1Eb4F6HdKx6", EncIV: "Nx5Lq7Kv4W8Jp6Mu", HTTPClient: nano.Client(),
	})
	svc := service.New(memory.NewRepository(), keyInOrders{}, provider, nil).WithCardApprover(provider, "240000005")
	h := handler.New(svc, authjwt.NewHMACValidator(keyInSecret)).WithNano(provider)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	payment, err := svc.CreatePayment(t.Context(), service.CreatePaymentInput{OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "t"})
	if err != nil {
		t.Fatal(err)
	}
	return mux, payment.ID, &calls
}

const goodCardBody = `{"card_number":"4111 1111 1111 1111","exp_month":"07","exp_year":"30","card_password":"12","holder_id":"900101","installment":"00"}`

func postCard(t *testing.T, mux *http.ServeMux, paymentID, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/"+paymentID+"/card", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+makeToken(t, keyInSecret, user, nil))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func assertNoCard(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, secret := range []string{"4111", "900101"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("response echoes card data: %s", rec.Body.String())
		}
	}
}

func TestPayByCardRoute_Approved(t *testing.T) {
	var body atomic.Value
	body.Store(`{"resultCode":"0000","shopcode":"240000005","reqPayAmt":"77000","tranNo":"T1","cardNo":"41111111****1111"}`)
	mux, id, _ := newKeyInServer(t, &body)
	rec := postCard(t, mux, id, "cust_1", goodCardBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["status"] != "succeeded" || got["provider_ref"] != "T1" {
		t.Fatalf("payment = %v", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("card responses must not be cached")
	}
	assertNoCard(t, rec)
}

func TestPayByCardRoute_Errors(t *testing.T) {
	cases := []struct {
		name, user, body, nano string
		status                 int
		reason                 string
		wantCalls              int32
	}{
		{"no token", "", goodCardBody, "", http.StatusUnauthorized, "", 0},
		{"not the owner", "cust_2", goodCardBody, "", http.StatusForbidden, "", 0},
		{"bad checksum", "cust_1", strings.Replace(goodCardBody, "1111\"", "1112\"", 1), "", http.StatusUnprocessableEntity, "invalid_card", 0},
		{"unknown field", "cust_1", `{"card_number":"4111111111111111","cvc":"123"}`, "", http.StatusBadRequest, "", 0},
		{"declined", "cust_1", goodCardBody, `{"resultCode":"8035","resultMsg":"비밀번호 오류"}`, http.StatusPaymentRequired, "card_declined", 1},
		{"no answer", "cust_1", goodCardBody, `<html>`, http.StatusConflict, "card_approval_unconfirmed", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body atomic.Value
			body.Store(tc.nano)
			mux, id, calls := newKeyInServer(t, &body)
			rec := postCard(t, mux, id, tc.user, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.reason != "" {
				var got map[string]any
				_ = json.Unmarshal(rec.Body.Bytes(), &got)
				if got["reason"] != tc.reason {
					t.Fatalf("reason = %v, want %s", got["reason"], tc.reason)
				}
			}
			if atomic.LoadInt32(calls) != tc.wantCalls {
				t.Fatalf("NANO calls = %d, want %d", *calls, tc.wantCalls)
			}
			assertNoCard(t, rec)
		})
	}
}

func TestPayByCardRoute_DeclineCarriesAttemptsLeft(t *testing.T) {
	var body atomic.Value
	body.Store(`{"resultCode":"8035","resultMsg":"비밀번호 오류"}`)
	mux, id, _ := newKeyInServer(t, &body)
	rec := postCard(t, mux, id, "cust_1", goodCardBody)
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["message"] != "비밀번호 오류" || got["attempts_remaining"] != float64(4) {
		t.Fatalf("body = %v", got)
	}
}
