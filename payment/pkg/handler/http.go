package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/authmiddleware"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

type Handler struct {
	svc          *service.Service
	jwtValidator authjwt.AccessTokenValidator
	nano         *checkout.NanoProvider
	settings     settings.Response
}

func New(svc *service.Service, jwtValidator authjwt.AccessTokenValidator) *Handler {
	return &Handler{
		svc:          svc,
		jwtValidator: jwtValidator,
		settings:     settings.NewResponse("payment"),
	}
}

// WithNano enables NANO certified-payment bridge + return/webhook routes.
func (h *Handler) WithNano(nano *checkout.NanoProvider) *Handler {
	h.nano = nano
	return h
}

// WithSettings sets the non-secret settings payload served by GET /settings.
func (h *Handler) WithSettings(s settings.Response) *Handler {
	h.settings = s
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/payments/health", h.health)
	mux.HandleFunc("/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/payments/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/payments", h.requireAuth(h.payments))
	mux.HandleFunc("/api/v1/payments/", h.paymentRoutes)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, h.settings)
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return authmiddleware.RequireAuth(h.jwtValidator, respondError)(next)
}

func (h *Handler) payments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	var req struct {
		OrderID string `json:"order_id"`
		Method  string `json:"method"`
		Note    string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	bearerToken := ""
	if auth := r.Header.Get("Authorization"); len(auth) > 7 {
		bearerToken = auth[7:]
	}
	payment, err := h.svc.CreatePayment(r.Context(), service.CreatePaymentInput{
		OrderID:           req.OrderID,
		CustomerID:        claims.UserID,
		BearerToken:       bearerToken,
		IdempotencyKey:    r.Header.Get("Idempotency-Key"),
		Method:            req.Method,
		Note:              req.Note,
		CreatedBy:         claims.UserID,
		BypassABAC:        permissions.BypassesPaymentCreateABAC(claims.Permissions),
		AllowMethodBypass: permissions.CanBypassPayment(claims.Permissions),
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, payment)
}

func (h *Handler) paymentRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/payments/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	if parts[0] == "nano" && len(parts) == 2 && parts[1] == "return" && r.Method == http.MethodPost {
		h.nanoReturn(w, r)
		return
	}
	if parts[0] == "webhooks" && len(parts) == 2 && parts[1] == "nano" && r.Method == http.MethodPost {
		h.nanoWebhook(w, r)
		return
	}

	if len(parts) == 3 && parts[1] == "nano" && parts[2] == "checkout" && r.Method == http.MethodGet {
		h.nanoCheckout(w, r, parts[0])
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
			h.getPayment(w, r, parts[0])
		})(w, r)
		return
	}

	respondError(w, http.StatusNotFound, "not found")
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request, paymentID string) {
	claims, _ := authjwt.FromContext(r.Context())
	ownerID := claims.UserID
	if h.jwtValidator != nil && permissions.BypassesPaymentReadABAC(claims.Permissions) {
		ownerID = ""
	}
	payment, err := h.svc.GetPayment(r.Context(), paymentID, ownerID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, payment)
}

// nanoCheckout bridges the browser into NANO certified checkout (PC or mobile).
// It POSTs a freshly signed request server-side and streams HTML / redirects.
func (h *Handler) nanoCheckout(w http.ResponseWriter, r *http.Request, paymentID string) {
	if h.nano == nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	payment, err := h.svc.GetPayment(r.Context(), paymentID, "")
	if err != nil {
		respondServiceError(w, err)
		return
	}
	if payment.Status != domain.StatusRequiresPayment {
		respondError(w, http.StatusBadRequest, "payment is not awaiting checkout")
		return
	}
	if payment.Provider != domain.ProviderNano {
		respondError(w, http.StatusBadRequest, "payment is not a nano checkout")
		return
	}

	mobile := checkout.IsMobileUserAgent(r.UserAgent())
	reqURL, body, err := h.nano.BuildRequest(
		payment.ID, payment.OrderID, payment.CustomerID,
		payment.PayerName, payment.PayerPhone, payment.PayerEmail,
		"Dupli1 "+payment.OrderID, payment.AmountCents, mobile,
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	payload, err := json.Marshal(body)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, reqURL, strings.NewReader(string(payload)))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CharSet", "UTF-8")
	cfg := h.nano.Config()
	if cfg.APIKey != "" {
		req.Header.Set("API_KEY", cfg.APIKey)
	}

	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("payment nano checkout request: %v", err)
		h.writeNanoLauncher(w, reqURL, body)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		respondError(w, http.StatusBadGateway, "nano upstream read failed")
		return
	}

	ct := resp.Header.Get("Content-Type")
	if loc := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode < 400 && loc != "" {
		http.Redirect(w, r, loc, http.StatusFound)
		return
	}
	if strings.Contains(ct, "application/json") || (len(respBody) > 0 && respBody[0] == '{') {
		var parsed map[string]any
		if err := json.Unmarshal(respBody, &parsed); err == nil {
			for _, key := range []string{"payUrl", "pay_url", "redirectUrl", "redirect_url", "checkoutUrl", "checkout_url", "paymentUrl", "payment_url"} {
				if v, ok := parsed[key].(string); ok && strings.TrimSpace(v) != "" {
					http.Redirect(w, r, v, http.StatusFound)
					return
				}
			}
			if code, _ := parsed["resultCode"].(string); code != "" && code != "0000" {
				msg, _ := parsed["resultMsg"].(string)
				respondError(w, http.StatusBadGateway, "nano checkout failed: "+msg)
				return
			}
		}
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		// Upstream IP allowlist — fall back to browser-side launch (CORS * on NANO).
		h.writeNanoLauncher(w, reqURL, body)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("payment nano checkout: status=%d body=%s", resp.StatusCode, truncateForLog(string(respBody), 200))
		h.writeNanoLauncher(w, reqURL, body)
		return
	}

	if ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}

// writeNanoLauncher serves an HTML page that POSTs the signed JSON from the browser.
// Used when the payment service host is not yet allowlisted by NANO.
func (h *Handler) writeNanoLauncher(w http.ResponseWriter, reqURL string, body checkout.NanoRequest) {
	payload, err := json.Marshal(body)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ko"><head><meta charset="utf-8"><title>결제 연결 중…</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>body{font-family:system-ui,sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#f7f7f5;color:#222}
.box{text-align:center;padding:2rem} .err{color:#b00020;margin-top:1rem;white-space:pre-wrap}</style>
</head><body><div class="box"><p>나노페이 결제창으로 이동 중입니다…</p><p class="err" id="err" hidden></p></div>
<script>
(async function(){
  const url = %s;
  const body = %s;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: {"Content-Type": "application/json", "CharSet": "UTF-8"},
      body: JSON.stringify(body),
      credentials: "omit",
      redirect: "follow"
    });
    const ct = res.headers.get("content-type") || "";
    if (ct.includes("application/json")) {
      const j = await res.json();
      const next = j.payUrl || j.pay_url || j.redirectUrl || j.redirect_url || j.checkoutUrl || j.checkout_url || j.paymentUrl || j.payment_url;
      if (next) { location.href = next; return; }
      if (j.resultCode && j.resultCode !== "0000") {
        throw new Error(j.resultMsg || ("결제 요청 실패: " + j.resultCode));
      }
      document.open(); document.write(JSON.stringify(j)); document.close();
      return;
    }
    const html = await res.text();
    document.open(); document.write(html); document.close();
  } catch (e) {
    const el = document.getElementById("err");
    el.hidden = false;
    el.textContent = "결제창을 열 수 없습니다. " + (e && e.message ? e.message : e);
  }
})();
</script></body></html>`, jsonString(reqURL), string(payload))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (h *Handler) nanoReturn(w http.ResponseWriter, r *http.Request) {
	if h.nano == nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := parseNanoResult(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid nano return payload")
		return
	}
	cfg := h.nano.Config()
	payment, err := h.svc.HandleNanoResult(r.Context(), nanoCallbackAuth(cfg), result)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	dest := cfg.SuccessURL
	if payment.Status != domain.StatusSucceeded {
		dest = cfg.FailureURL
		if dest == "" {
			dest = cfg.SuccessURL
		}
	}
	if dest == "" {
		respondJSON(w, http.StatusOK, map[string]any{
			"message": "Payment processed",
			"payment": payment,
		})
		return
	}
	http.Redirect(w, r, appendOrderPaymentQuery(dest, payment.OrderID, payment.ID), http.StatusSeeOther)
}

func (h *Handler) nanoWebhook(w http.ResponseWriter, r *http.Request) {
	if h.nano == nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := parseNanoResult(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid nano webhook payload")
		return
	}
	if _, err := h.svc.HandleNanoResult(r.Context(), nanoCallbackAuth(h.nano.Config()), result); err != nil {
		respondServiceError(w, err)
		return
	}
	// NANO virtual-account NOTI expects resultCode "00"; card webhook ack is JSON OK.
	respondJSON(w, http.StatusOK, map[string]string{"resultCode": "00"})
}

func nanoCallbackAuth(cfg checkout.NanoConfig) service.NanoCallbackAuth {
	return service.NanoCallbackAuth{
		Ver:      cfg.Ver,
		LoginID:  cfg.LoginID,
		ShopCode: cfg.ShopCode,
		APIKey:   cfg.APIKey,
	}
}

func parseNanoResult(r *http.Request) (service.NanoResult, error) {
	ct := r.Header.Get("Content-Type")
	var result service.NanoResult
	if strings.Contains(ct, "application/json") {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			return result, err
		}
		return result, nil
	}
	if err := r.ParseForm(); err != nil {
		return result, err
	}
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(r.FormValue(k)); v != "" {
				return v
			}
		}
		return ""
	}
	result = service.NanoResult{
		ResultCode:  get("resultCode", "result_code"),
		ResultMsg:   get("resultMsg", "result_msg"),
		ShopCode:    get("shopcode", "shopCode"),
		CompOrderNo: get("compOrderNo", "comp_order_no"),
		ReqPayAmt:   get("reqPayAmt", "req_pay_amt"),
		TranNo:      get("tranNo", "tran_no"),
		PayWay:      get("payWay", "pay_way"),
		Timestamp:   get("timestamp"),
		HashValue:   get("hashValue", "hash_value"),
	}
	if result.CompOrderNo == "" && result.ResultCode == "" {
		return result, fmt.Errorf("empty nano payload")
	}
	return result, nil
}

func appendOrderPaymentQuery(base, orderID, paymentID string) string {
	u, err := url.Parse(base)
	if err != nil {
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		return base + sep + "order_id=" + url.QueryEscape(orderID) + "&payment_id=" + url.QueryEscape(paymentID)
	}
	q := u.Query()
	q.Set("order_id", orderID)
	q.Set("payment_id", paymentID)
	u.RawQuery = q.Encode()
	return u.String()
}

func truncateForLog(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound), errors.Is(err, ports.ErrOrderNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ports.ErrOrderForbidden), errors.Is(err, ports.ErrPaymentForbidden):
		respondError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ports.ErrMethodUnavailable):
		respondError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, ports.ErrOrderNotPending), errors.Is(err, domain.ErrInvalidPayment):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("payment: internal error: %v", err)
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]any{"error": message, "code": status})
}
