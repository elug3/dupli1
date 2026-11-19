package checkout

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/payment/pkg/ports"
)

// Key-in card approval (수기결제).
//
// Source: [NANO] 수기결제 연동 API 안내 v2.5 §1 (공통) and §2 (결제요청,
// POST /api/payment/approval.io). NANO has no hosted card form for this API:
// the shopper types the card on the storefront, it reaches payment once, is
// encrypted into encData (AES-256-CBC, PKCS7, base64 — §1) and sent server to
// server. Nothing here stores, logs or echoes the card; the response carries
// only NANO's masked number. See docs/payment-nano-key-in.md.

const (
	nanoApprovalPath = "/api/payment/approval.io"
	// nanoEncKeyLen / nanoEncIVLen are AES-256 and the CBC block size. NANO
	// hands both out as ASCII strings of exactly these lengths (§1).
	nanoEncKeyLen = 32
	nanoEncIVLen  = aes.BlockSize
	// nanoApprovalTimeout bounds one approval call. It is never retried: a
	// timeout is an unknown outcome, not a failure (see ErrCardApprovalUnconfirmed).
	nanoApprovalTimeout = 30 * time.Second
)

// KeyInEnabled reports whether key-in approval can run: the merchant
// credentials plus the encData key and IV NANO issued with the contract.
func (c NanoConfig) KeyInEnabled() bool {
	return c.Enabled() && len(c.EncKey) == nanoEncKeyLen && len(c.EncIV) == nanoEncIVLen
}

// KeyInMisconfigured reports a key or IV that was set but has the wrong
// length, so startup can say so instead of quietly keeping 인증결제.
func (c NanoConfig) KeyInMisconfigured() bool {
	if c.EncKey == "" && c.EncIV == "" {
		return false
	}
	return len(c.EncKey) != nanoEncKeyLen || len(c.EncIV) != nanoEncIVLen
}

// nanoCardData is the encData plaintext (§2, encData json). Field names are
// NANO's. It only ever exists in memory for the length of one approval call.
type nanoCardData struct {
	CardNo        string `json:"cardNo"`
	CardExpYy     string `json:"cardExpYy"`
	CardExpMm     string `json:"cardExpMm"`
	CardPw        string `json:"cardPw"`
	CardHolderYmd string `json:"cardHolderYmd"`
}

// NanoApprovalRequest is the JSON body for approval.io (§2 요청).
type NanoApprovalRequest struct {
	Ver          string `json:"ver,omitempty"`
	LoginID      string `json:"loginId"`
	ShopCode     string `json:"shopcode"`
	OrderName    string `json:"orderName"`
	OrderTel     string `json:"orderTel"`
	OrderEmail   string `json:"orderEmail,omitempty"`
	GoodsName    string `json:"goodsName"`
	ReqPayAmt    string `json:"reqPayAmt"`
	Installment  string `json:"installment"`
	CompOrderNo  string `json:"compOrderNo,omitempty"`
	CompOrderMem string `json:"compOrderMem,omitempty"`
	EncData      string `json:"encData"`
}

// NanoApprovalResponse is approval.io's answer (§2 응답). CardNo is masked by
// NANO (48901682****1700).
type NanoApprovalResponse struct {
	ResultCode  string `json:"resultCode"`
	ResultMsg   string `json:"resultMsg"`
	ShopCode    string `json:"shopcode"`
	ApprDate    string `json:"apprDate"`
	ApprTime    string `json:"apprTime"`
	ReqPayAmt   string `json:"reqPayAmt"`
	Installment string `json:"installment"`
	CardNo      string `json:"cardNo"`
	ApprNo      string `json:"apprNo"`
	TranNo      string `json:"tranNo"`
	CardTxt     string `json:"cardTxt"`
	CardCode    string `json:"cardCode"`
	CompOrderNo string `json:"compOrderNo"`
}

// EncryptNanoCardData returns base64(AES-256-CBC(PKCS7(plaintext))) with the
// merchant key and IV, as §1 specifies.
func EncryptNanoCardData(key, iv string, plaintext []byte) (string, error) {
	if len(key) != nanoEncKeyLen || len(iv) != nanoEncIVLen {
		return "", errors.New("nano encData key must be 32 bytes and IV 16 bytes")
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	buf := make([]byte, len(plaintext)+pad)
	copy(buf, plaintext)
	for i := len(plaintext); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	cipher.NewCBCEncrypter(block, []byte(iv)).CryptBlocks(buf, buf)
	return base64.StdEncoding.EncodeToString(buf), nil
}

// decryptNanoCardData reverses EncryptNanoCardData. Tests only (a fake NANO
// reads encData back); production never decrypts.
func decryptNanoCardData(key, iv, encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, errors.New("encData is not a whole number of blocks")
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	cipher.NewCBCDecrypter(block, []byte(iv)).CryptBlocks(raw, raw)
	pad := int(raw[len(raw)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(raw) {
		return nil, errors.New("bad padding")
	}
	return raw[:len(raw)-pad], nil
}

// BuildApprovalRequest encrypts the card and builds the approval body.
func (p *NanoProvider) BuildApprovalRequest(input ports.CardApprovalInput) (requestURL string, body NanoApprovalRequest, err error) {
	if !p.cfg.KeyInEnabled() {
		return "", NanoApprovalRequest{}, fmt.Errorf("%w: nano key-in is not configured", ports.ErrMethodUnavailable)
	}
	name := strings.TrimSpace(input.OrderName)
	tel := normalizeKRPhone(input.OrderTel)
	if name == "" || tel == "" || input.PaymentID == "" || input.AmountWon <= 0 {
		return "", NanoApprovalRequest{}, errors.New("nano approval needs payer name, phone, payment id and amount")
	}
	plain, err := json.Marshal(nanoCardData{
		CardNo:        input.Card.Number,
		CardExpYy:     input.Card.ExpYY,
		CardExpMm:     input.Card.ExpMM,
		CardPw:        input.Card.Password2,
		CardHolderYmd: input.Card.HolderID,
	})
	if err != nil {
		return "", NanoApprovalRequest{}, err
	}
	enc, err := EncryptNanoCardData(p.cfg.EncKey, p.cfg.EncIV, plain)
	for i := range plain {
		plain[i] = 0
	}
	if err != nil {
		return "", NanoApprovalRequest{}, err
	}
	goods := strings.TrimSpace(input.GoodsName)
	if goods == "" {
		goods = "Dupli1 " + input.OrderID
	}
	installment := strings.TrimSpace(input.Card.Installment)
	if installment == "" {
		installment = "00"
	}
	body = NanoApprovalRequest{
		Ver:          strings.TrimSpace(p.cfg.Ver),
		LoginID:      strings.TrimSpace(p.cfg.LoginID),
		ShopCode:     strings.TrimSpace(p.cfg.ShopCode),
		OrderName:    name,
		OrderTel:     tel,
		OrderEmail:   strings.TrimSpace(input.OrderEmail),
		GoodsName:    goods,
		ReqPayAmt:    fmt.Sprintf("%d", input.AmountWon),
		Installment:  installment,
		CompOrderNo:  input.PaymentID,
		CompOrderMem: clampNanoCompOrderMem(input.OrderEmail),
		EncData:      enc,
	}
	return p.cfg.normalizedBaseURL() + nanoApprovalPath, body, nil
}

// ApproveCard charges a typed card through approval.io.
//
// Outcomes:
//   - a parsed answer (approved or declined) → result, nil
//   - nothing was sent (not configured, bad input) → error, never
//     ErrCardApprovalUnconfirmed: the card was not charged
//   - sent, but no parsed answer (timeout, transport error, non-2xx, not
//     JSON) → ErrCardApprovalUnconfirmed: the card may have been charged, so
//     the caller must not let the shopper try again on its own
func (p *NanoProvider) ApproveCard(ctx context.Context, input ports.CardApprovalInput) (*ports.CardApprovalResult, error) {
	reqURL, body, err := p.BuildApprovalRequest(input)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal nano approval: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, nanoApprovalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build nano approval request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CharSet", "UTF-8")
	req.Header.Set("API_KEY", p.cfg.APIKey)

	client := p.cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	// Never retried: a second POST after a lost response could charge twice.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: nano approval request: %v", ports.ErrCardApprovalUnconfirmed, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: nano approval read: %v", ports.ErrCardApprovalUnconfirmed, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: nano approval http %d", ports.ErrCardApprovalUnconfirmed, resp.StatusCode)
	}
	var parsed NanoApprovalResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.ResultCode) == "" {
		return nil, fmt.Errorf("%w: nano approval response has no resultCode", ports.ErrCardApprovalUnconfirmed)
	}
	approved := strings.TrimSpace(parsed.ResultCode) == nanoResultCodeSuccess
	result := &ports.CardApprovalResult{
		Approved:    approved,
		ResultCode:  strings.TrimSpace(parsed.ResultCode),
		ResultMsg:   strings.TrimSpace(parsed.ResultMsg),
		ShopCode:    strings.TrimSpace(parsed.ShopCode),
		ReqPayAmt:   strings.TrimSpace(parsed.ReqPayAmt),
		TranNo:      strings.TrimSpace(parsed.TranNo),
		ApprNo:      strings.TrimSpace(parsed.ApprNo),
		CardMasked:  maskedCardOnly(parsed.CardNo),
		CardName:    strings.TrimSpace(parsed.CardTxt),
		CompOrderNo: strings.TrimSpace(parsed.CompOrderNo),
	}
	return result, nil
}

// maskedCardOnly keeps NANO's masked number only if it really is masked, so a
// PG that ever echoed the full number would not have it carried any further.
func maskedCardOnly(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "*") {
		return ""
	}
	return s
}
