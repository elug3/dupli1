package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func newKillSwitchMux(t *testing.T) *http.ServeMux {
	t.Helper()
	svc := service.New(memory.NewRepository(), stubOrderClient{}, fakeCheckoutProvider{}, nil).
		WithKillSwitches(memory.NewKillSwitches())
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret"))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func doJSON(t *testing.T, mux *http.ServeMux, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func methodStatus(t *testing.T, mux *http.ServeMux, method string) service.MethodStatus {
	t.Helper()
	rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/methods", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("methods: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Methods []service.MethodStatus `json:"methods"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, m := range body.Methods {
		if m.Method == method {
			return m
		}
	}
	t.Fatalf("method %s not listed: %s", method, rec.Body.String())
	return service.MethodStatus{}
}

func TestKillSwitchBlocksCardPaymentUntilReleased(t *testing.T) {
	mux := newKillSwitchMux(t)
	admin := makeToken(t, "test-secret", "admin-1", []string{permissions.PaymentKillSwitchManage})
	customer := makeToken(t, "test-secret", "u-1", nil)
	key := domain.PaymentMethodKillSwitchKey(domain.MethodCreditCard)

	if st := methodStatus(t, mux, domain.MethodCreditCard); !st.Available || st.Disabled {
		t.Fatalf("card before engage = %+v, want available", st)
	}

	rec := doJSON(t, mux, http.MethodPut, "/api/v1/payments/kill-switches/"+key, admin, map[string]any{
		"engaged": true,
		"message": "카드 결제 점검 중입니다",
		"reason":  "PG outage",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("engage: %d %s", rec.Code, rec.Body.String())
	}
	var sw domain.KillSwitch
	if err := json.Unmarshal(rec.Body.Bytes(), &sw); err != nil {
		t.Fatal(err)
	}
	if !sw.Engaged || sw.UpdatedBy != "admin-1@example.com" || sw.UpdatedAt == nil {
		t.Fatalf("engaged switch = %+v", sw)
	}

	st := methodStatus(t, mux, domain.MethodCreditCard)
	if st.Available || !st.Disabled || st.Message != "카드 결제 점검 중입니다" {
		t.Fatalf("card while engaged = %+v", st)
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/payments", customer, map[string]any{"order_id": "ord-1"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("card payment while engaged: %d %s, want 409", rec.Code, rec.Body.String())
	}
	var refused map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &refused)
	if refused["reason"] != "payment_method_disabled" || refused["method"] != "credit_card" || refused["message"] != "카드 결제 점검 중입니다" {
		t.Fatalf("refusal body = %v", refused)
	}

	rec = doJSON(t, mux, http.MethodPut, "/api/v1/payments/kill-switches/"+key, admin, map[string]any{"engaged": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("release: %d %s", rec.Code, rec.Body.String())
	}
	if st := methodStatus(t, mux, domain.MethodCreditCard); !st.Available || st.Message != "" {
		t.Fatalf("card after release = %+v", st)
	}
	rec = doJSON(t, mux, http.MethodPost, "/api/v1/payments", customer, map[string]any{"order_id": "ord-1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("card payment after release: %d %s", rec.Code, rec.Body.String())
	}
}

func TestKillSwitchCardLeavesBypassAlone(t *testing.T) {
	mux := newKillSwitchMux(t)
	admin := makeToken(t, "test-secret", "admin-1", []string{permissions.PaymentKillSwitchManage, permissions.PaymentBypass, permissions.PaymentCreate})
	rec := doJSON(t, mux, http.MethodPut, "/api/v1/payments/kill-switches/payment_method.credit_card", admin, map[string]any{"engaged": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("engage: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, mux, http.MethodPost, "/api/v1/payments", admin, map[string]any{"order_id": "ord-1", "method": "bypass"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("bypass while card is blocked: %d %s", rec.Code, rec.Body.String())
	}
}

func TestKillSwitchPermissions(t *testing.T) {
	mux := newKillSwitchMux(t)
	nobody := makeToken(t, "test-secret", "u-1", nil)
	reader := makeToken(t, "test-secret", "ops-1", []string{permissions.PaymentKillSwitchRead})

	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/kill-switches", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d", rec.Code)
	}
	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/kill-switches", nobody, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("customer list: %d", rec.Code)
	}
	rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/kill-switches", reader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reader list: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		KillSwitches []domain.KillSwitch `json:"kill_switches"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.KillSwitches) != len(domain.KillSwitchDefs) || body.KillSwitches[0].Engaged {
		t.Fatalf("list = %+v, want every def released", body.KillSwitches)
	}
	if rec := doJSON(t, mux, http.MethodPut, "/api/v1/payments/kill-switches/payment_method.credit_card", reader, map[string]any{"engaged": true}); rec.Code != http.StatusForbidden {
		t.Fatalf("reader engage: %d", rec.Code)
	}
}

func TestKillSwitchRejectsBadChanges(t *testing.T) {
	mux := newKillSwitchMux(t)
	admin := makeToken(t, "test-secret", "admin-1", []string{"*"})
	cases := []struct {
		name string
		key  string
		body map[string]any
		want int
	}{
		{"unknown key", "payment_method.bitcoin", map[string]any{"engaged": true}, http.StatusNotFound},
		{"missing engaged", "payment_method.credit_card", map[string]any{"reason": "x"}, http.StatusBadRequest},
		{"message too long", "payment_method.credit_card", map[string]any{"engaged": true, "message": string(bytes.Repeat([]byte("가"), domain.KillSwitchMessageMax+1))}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, mux, http.MethodPut, "/api/v1/payments/kill-switches/"+tc.key, admin, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%d %s, want %d", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}

func TestMethodsReportsUnconfiguredCard(t *testing.T) {
	svc := service.New(memory.NewRepository(), stubOrderClient{}, fakeCheckoutProvider{}, nil).
		WithKillSwitches(memory.NewKillSwitches()).
		WithConfiguredMethods(map[string]bool{domain.MethodBypass: true})
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret"))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	st := methodStatus(t, mux, domain.MethodCreditCard)
	if st.Available || st.Configured || st.Disabled {
		t.Fatalf("unconfigured card = %+v", st)
	}
	if st := methodStatus(t, mux, domain.MethodBypass); !st.Available || st.RequiresPermission != permissions.PaymentBypass {
		t.Fatalf("bypass = %+v", st)
	}
}
