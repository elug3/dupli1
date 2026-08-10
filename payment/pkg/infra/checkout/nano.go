package checkout

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

const (
	nanoPayWayCard        = "card"
	nanoDefaultTestBase   = "https://dev3.nanopay.co.kr"
	nanoDefaultProdBase   = "https://pay.nanopay.co.kr"
	nanoPCRequestPath     = "/api/payment/cert/pc/request.io"
	nanoMobileRequestPath = "/api/payment/cert/mobile/request.io"
)

// NanoConfig holds NANO Solution certified-payment (인증결제) credentials.
type NanoConfig struct {
	BaseURL       string // https://dev3.nanopay.co.kr or https://pay.nanopay.co.kr
	Ver           string
	ShopCode      string
	LoginID       string
	APIKey        string
	PublicBaseURL string // gateway URL used for receiveUrl + checkout bridge
	SuccessURL    string // storefront redirect after paid
	FailureURL    string // storefront redirect after failed/canceled
	HTTPClient    *http.Client
}

func (c NanoConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" &&
		strings.TrimSpace(c.ShopCode) != "" &&
		strings.TrimSpace(c.LoginID) != ""
}

func (c NanoConfig) normalizedBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nanoDefaultTestBase
	}
	return base
}

func (c NanoConfig) publicBase() string {
	base := strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
	if base == "" {
		return "http://localhost:8080"
	}
	return base
}

// NanoProvider starts NANO certified card checkout via a Dupli1 bridge URL.
// The bridge POSTs to NANO (PC or mobile) with a fresh timestamp/hash; NANO
// redirects the browser to receiveUrl with the approval form result.
type NanoProvider struct {
	cfg NanoConfig
}

func NewNanoProvider(cfg NanoConfig) *NanoProvider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if strings.TrimSpace(cfg.Ver) == "" {
		cfg.Ver = cfg.ShopCode
	}
	return &NanoProvider{cfg: cfg}
}

func (p *NanoProvider) Config() NanoConfig {
	return p.cfg
}

func (p *NanoProvider) CreateSession(_ context.Context, input ports.CheckoutSessionInput) (*ports.CheckoutSessionResult, error) {
	if !p.cfg.Enabled() {
		return nil, fmt.Errorf("%w: nano credentials not configured", ports.ErrMethodUnavailable)
	}
	name := strings.TrimSpace(input.OrderName)
	tel := normalizeKRPhone(input.OrderTel)
	if name == "" || tel == "" {
		return nil, fmt.Errorf("%w: order recipient name and phone are required for card payment", domain.ErrInvalidPayment)
	}
	if input.PaymentID == "" || input.AmountCents <= 0 {
		return nil, domain.ErrInvalidPayment
	}

	checkoutURL := fmt.Sprintf("%s/api/v1/payments/%s/nano/checkout", p.cfg.publicBase(), input.PaymentID)
	return &ports.CheckoutSessionResult{
		Provider:    domain.ProviderNano,
		ProviderRef: "nano_" + input.PaymentID,
		CheckoutURL: checkoutURL,
	}, nil
}

// NanoRequest is the JSON body for NANO cert PC/mobile request.io.
type NanoRequest struct {
	Ver          string `json:"ver"`
	LoginID      string `json:"loginId"`
	ShopCode     string `json:"shopcode"`
	OrderName    string `json:"orderName"`
	OrderTel     string `json:"orderTel"`
	OrderEmail   string `json:"orderEmail,omitempty"`
	PayWay       string `json:"payWay"`
	GoodsName    string `json:"goodsName"`
	ReqPayAmt    string `json:"reqPayAmt"`
	ReceiveURL   string `json:"receiveUrl"`
	CompOrderNo  string `json:"compOrderNo,omitempty"`
	CompOrderMem string `json:"compOrderMem,omitempty"`
	Timestamp    string `json:"timestamp"`
	HashValue    string `json:"hashValue"`
}

// BuildRequest builds a signed NANO cert payment request for the given payment snapshot.
func (p *NanoProvider) BuildRequest(paymentID, orderID, customerID, orderName, orderTel, orderEmail, goodsName string, amountCents int64, mobile bool) (requestURL string, body NanoRequest, err error) {
	if !p.cfg.Enabled() {
		return "", NanoRequest{}, fmt.Errorf("nano credentials not configured")
	}
	name := strings.TrimSpace(orderName)
	tel := normalizeKRPhone(orderTel)
	if name == "" || tel == "" {
		return "", NanoRequest{}, fmt.Errorf("order name and phone required")
	}
	if amountCents <= 0 {
		return "", NanoRequest{}, fmt.Errorf("invalid amount")
	}
	if strings.TrimSpace(goodsName) == "" {
		goodsName = "Dupli1 " + orderID
	}
	amt := fmt.Sprintf("%d", amountCents)
	ts := nanoTimestamp(time.Now())
	ver := strings.TrimSpace(p.cfg.Ver)
	login := strings.TrimSpace(p.cfg.LoginID)
	shop := strings.TrimSpace(p.cfg.ShopCode)
	hash := NanoHash(ver, login, shop, amt, ts, p.cfg.APIKey)

	path := nanoPCRequestPath
	if mobile {
		path = nanoMobileRequestPath
	}
	body = NanoRequest{
		Ver:          ver,
		LoginID:      login,
		ShopCode:     shop,
		OrderName:    name,
		OrderTel:     tel,
		OrderEmail:   strings.TrimSpace(orderEmail),
		PayWay:       nanoPayWayCard,
		GoodsName:    goodsName,
		ReqPayAmt:    amt,
		ReceiveURL:   p.cfg.publicBase() + "/api/v1/payments/nano/return",
		CompOrderNo:  paymentID,
		CompOrderMem: customerID,
		Timestamp:    ts,
		HashValue:    hash,
	}
	return p.cfg.normalizedBaseURL() + path, body, nil
}

// NanoHash returns SHA256(ver+loginId+shopcode+reqPayAmt+timestamp+API_KEY+"NANO") hex digest.
// Used for outbound cert requests and (until merchant docs specify otherwise) for
// verifying receiveUrl / webhook hashValue with the callback timestamp.
func NanoHash(ver, loginID, shopCode, reqPayAmt, timestamp, apiKey string) string {
	sum := sha256.Sum256([]byte(ver + loginID + shopCode + reqPayAmt + timestamp + apiKey + "NANO"))
	return hex.EncodeToString(sum[:])
}

// VerifyNanoCallbackHash reports whether hashValue matches the NANO request-style
// hash over callback fields. Callers must fail closed on false for resultCode=0000.
//
// Formula (same as request until merchant return-hash spec is confirmed):
//
//	SHA256(ver+loginId+shopcode+reqPayAmt+timestamp+API_KEY+"NANO")
//
// ver/loginId come from merchant config (not client-supplied). shopcode, reqPayAmt,
// and timestamp must be present on the callback. If NANO’s live return hash differs,
// update this function to match the merchant API guide — do not disable verification.
func VerifyNanoCallbackHash(cfg NanoConfig, shopCode, reqPayAmt, timestamp, hashValue string) bool {
	got := strings.TrimSpace(hashValue)
	ts := strings.TrimSpace(timestamp)
	if got == "" || ts == "" || !cfg.Enabled() {
		return false
	}
	ver := strings.TrimSpace(cfg.Ver)
	if ver == "" {
		ver = strings.TrimSpace(cfg.ShopCode)
	}
	want := NanoHash(
		ver,
		strings.TrimSpace(cfg.LoginID),
		strings.TrimSpace(shopCode),
		strings.TrimSpace(reqPayAmt),
		ts,
		cfg.APIKey,
	)
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(got)), []byte(strings.ToLower(want))) == 1
}

func nanoTimestamp(now time.Time) string {
	// Docs: unix seconds (UTC) concatenated with 3-digit millis — equivalent to UnixMilli.
	return fmt.Sprintf("%d", now.UTC().UnixMilli())
}

func normalizeKRPhone(phone string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(phone) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsMobileUserAgent reports whether the client should use the mobile cert endpoint.
func IsMobileUserAgent(ua string) bool {
	ua = strings.ToLower(ua)
	for _, needle := range []string{"iphone", "ipod", "ipad", "android", "mobile", "blackberry", "windows phone"} {
		if strings.Contains(ua, needle) {
			return true
		}
	}
	return false
}
