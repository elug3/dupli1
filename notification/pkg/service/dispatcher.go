package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/money"

	tg "github.com/elug3/dupli1/shared/pkg/telegram"
)

// Subject aliases of the shared event contract — see shared/pkg/events.
const (
	SubjectOrderCreated            = events.OrderCreated
	SubjectOrderStatusUpdate       = events.OrderStatusUpdate
	SubjectOrderPaid               = events.OrderPaid
	SubjectProductCreated          = events.ProductCreated
	SubjectProductUpdated          = events.ProductUpdated
	SubjectProductDeleted          = events.ProductDeleted
	SubjectProductImage            = events.ProductImage
	SubjectPaymentCanceled         = events.PaymentCanceled
	SubjectPaymentCallbackRejected = events.PaymentCallbackRejected
	SubjectSupportInquiryOpened    = events.SupportInquiryOpened
)

type ChatRouting interface {
	OrderChatIDs(ctx context.Context) []string
	ProductChatIDs(ctx context.Context) []string
	SupportChatIDs(ctx context.Context) []string
}

// SilentNotifier is implemented by notifiers that can deliver without a ping.
//
// Optional on purpose: ports.Notifier stays a one-method interface, so a test
// double or a future channel that has no such notion still satisfies it, and an
// after-hours alert simply arrives loudly there instead of failing.
type SilentNotifier interface {
	SendSilent(ctx context.Context, chatID string, message string) error
}

type DispatcherConfig struct {
	Routing       ChatRouting
	OrderChatID   string
	ProductChatID string
	ManageWebURL  string
}

type Dispatcher struct {
	notifier ports.Notifier
	cfg      DispatcherConfig
}

func NewDispatcher(notifier ports.Notifier, cfg DispatcherConfig) *Dispatcher {
	return &Dispatcher{notifier: notifier, cfg: cfg}
}

func (d *Dispatcher) Register(subscriber ports.EventSubscriber, ctx context.Context) error {
	subjects := []string{
		SubjectOrderCreated,
		SubjectOrderStatusUpdate,
		SubjectOrderPaid,
		SubjectPaymentCanceled,
		SubjectPaymentCallbackRejected,
		SubjectProductCreated,
		SubjectProductUpdated,
		SubjectProductDeleted,
		SubjectProductImage,
		SubjectSupportInquiryOpened,
	}
	for _, subject := range subjects {
		if err := subscriber.Subscribe(ctx, subject, d.handle); err != nil {
			return err
		}
	}
	return nil
}

// HandleForTest exposes event handling for unit tests.
func (d *Dispatcher) HandleForTest(ctx context.Context, subject string, payload []byte) error {
	return d.handle(ctx, subject, payload)
}

func (d *Dispatcher) handle(ctx context.Context, subject string, payload []byte) error {
	switch subject {
	case SubjectOrderCreated, SubjectOrderStatusUpdate, SubjectOrderPaid:
		return d.handleOrder(ctx, subject, payload)
	case SubjectPaymentCanceled:
		return d.handlePaymentCanceled(ctx, payload)
	case SubjectPaymentCallbackRejected:
		return d.handlePaymentCallbackRejected(ctx, payload)
	case SubjectProductCreated, SubjectProductUpdated, SubjectProductDeleted, SubjectProductImage:
		return d.handleProduct(ctx, subject, payload)
	case SubjectSupportInquiryOpened:
		return d.handleSupportInquiry(ctx, payload)
	default:
		return nil
	}
}

func (d *Dispatcher) handleOrder(ctx context.Context, subject string, payload []byte) error {
	var event events.Order
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode order event: %w", err)
	}

	chatIDs := d.orderChatIDs(ctx)
	if len(chatIDs) == 0 {
		log.Printf("order event %s for %s skipped: order telegram chat not configured", subject, event.OrderID)
		return nil
	}

	message := formatOrderMessage(subject, event, d.cfg.ManageWebURL)
	return d.sendAll(ctx, chatIDs, message, "order event")
}

// handlePaymentCanceled alerts ops that money went back to a customer. Refunds
// were previously silent: order.* events covered creation and payment, so a
// cancelled payment produced no message at all and the only trace was a row in
// the payments table.
func (d *Dispatcher) handlePaymentCanceled(ctx context.Context, payload []byte) error {
	var event events.PaymentCanceledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payment.canceled event: %w", err)
	}

	chatIDs := d.orderChatIDs(ctx)
	if len(chatIDs) == 0 {
		log.Printf("payment.canceled for %s skipped: order telegram chat not configured", event.OrderID)
		return nil
	}

	return d.sendAll(ctx, chatIDs, formatPaymentCanceledMessage(event, d.cfg.ManageWebURL), "payment.canceled")
}

// formatPaymentCanceledMessage distinguishes a full refund from a partial one:
// a full refund cancels the order automatically, a partial leaves it standing
// with money still owed, and ops need to know which they are looking at.
func formatPaymentCanceledMessage(event events.PaymentCanceledEvent, manageWebURL string) string {
	manageLink := formatManageOrderLink(manageWebURL, event.OrderID)
	reason := strings.TrimSpace(event.Reason)
	reasonLine := ""
	if reason != "" {
		reasonLine = fmt.Sprintf("사유: %s\n", tg.EscapeHTML(reason))
	}
	byLine := ""
	if by := strings.TrimSpace(event.CanceledBy); by != "" {
		byLine = fmt.Sprintf("처리자: %s\n", tg.EscapeHTML(by))
	}

	if event.RemainingWon > 0 {
		return fmt.Sprintf(
			"↩️ <b>부분 환불</b> %s\n%s환불 금액: <b>%s</b>\n잔여 결제: <b>%s</b>\n%s%s주문은 그대로입니다 — 출고 여부를 확인하세요.",
			tg.EscapeHTML(event.OrderID), manageLink,
			formatMoney(event.AmountWon), formatMoney(event.RemainingWon),
			reasonLine, byLine,
		)
	}
	return fmt.Sprintf(
		"↩️ <b>전액 환불</b> %s\n%s환불 금액: <b>%s</b>\n%s%s주문이 취소되었고 재고가 해제되었습니다.",
		tg.EscapeHTML(event.OrderID), manageLink,
		formatMoney(event.AmountWon), reasonLine, byLine,
	)
}

// handlePaymentCallbackRejected alerts ops that a PG said it charged a card and
// dupli1 refused the callback. Nothing downstream fires for this — no order is
// paid, no payment row moves — so without the alert the only evidence is a line
// in the payment service log and a shopper who was charged for nothing
// (elug3/dupli1#232).
func (d *Dispatcher) handlePaymentCallbackRejected(ctx context.Context, payload []byte) error {
	var event events.PaymentCallbackRejectedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payment.callback_rejected event: %w", err)
	}

	chatIDs := d.orderChatIDs(ctx)
	if len(chatIDs) == 0 {
		log.Printf("payment.callback_rejected for %s skipped: order telegram chat not configured", event.PaymentID)
		return nil
	}

	return d.sendAll(ctx, chatIDs, formatPaymentCallbackRejectedMessage(event, d.cfg.ManageWebURL), "payment.callback_rejected")
}

// formatPaymentCallbackRejectedMessage leads with the action, because whoever
// reads it has to check the PG console against our records by hand. Every field
// is best-effort — a callback can be rejected precisely because it identified no
// payment — so each line is omitted rather than printed empty.
func formatPaymentCallbackRejectedMessage(event events.PaymentCallbackRejectedEvent, manageWebURL string) string {
	var b strings.Builder
	b.WriteString("🚨 <b>PG가 결제를 승인했으나 시스템에서 거부함</b>\n")
	b.WriteString("구매자가 이중 청구되지 않도록 PG 콘솔을 먼저 확인하세요.\n")

	if orderID := strings.TrimSpace(event.OrderID); orderID != "" {
		b.WriteString(fmt.Sprintf("주문: %s\n%s", tg.EscapeHTML(orderID), formatManageOrderLink(manageWebURL, orderID)))
	}
	if paymentID := strings.TrimSpace(event.PaymentID); paymentID != "" {
		b.WriteString(fmt.Sprintf("결제: %s\n", tg.EscapeHTML(paymentID)))
	}
	if detail := strings.TrimSpace(event.Detail); detail != "" {
		b.WriteString(fmt.Sprintf("원인: %s\n", tg.EscapeHTML(detail)))
	} else {
		b.WriteString(fmt.Sprintf("원인: %s\n", tg.EscapeHTML(event.Reason)))
	}
	if event.ExpectedWon > 0 {
		b.WriteString(fmt.Sprintf("예상 금액: <b>%s</b>\n", formatMoney(event.ExpectedWon)))
	}
	// Reported verbatim: a malformed amount is itself the clue.
	if reported := strings.TrimSpace(event.ReportedAmount); reported != "" {
		b.WriteString(fmt.Sprintf("PG 보고 금액: <b>%s</b>\n", tg.EscapeHTML(reported)))
	}
	if tran := strings.TrimSpace(event.TranNo); tran != "" {
		b.WriteString(fmt.Sprintf("PG 거래번호: %s\n", tg.EscapeHTML(tran)))
	}

	provider := strings.TrimSpace(event.Provider)
	if provider == "" {
		provider = "PG"
	}
	b.WriteString(fmt.Sprintf("출처: %s %s 콜백", tg.EscapeHTML(provider), tg.EscapeHTML(strings.TrimSpace(event.Source))))
	return b.String()
}

func (d *Dispatcher) handleProduct(ctx context.Context, subject string, payload []byte) error {
	var event events.Product
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode product event: %w", err)
	}

	chatIDs := d.productChatIDs(ctx)
	if len(chatIDs) == 0 {
		log.Printf("product event %s for %s skipped: product telegram chat not configured", subject, event.ProductID)
		return nil
	}

	message := formatProductMessage(subject, event)
	return d.sendAll(ctx, chatIDs, message, "product event")
}

// handleSupportInquiry tells staff a shopper is waiting for a person.
//
// An after-hours inquiry is delivered silently: the point is that it is in the
// queue by morning, not that someone's phone wakes them at 3am for a question
// that cannot be answered until the window opens.
func (d *Dispatcher) handleSupportInquiry(ctx context.Context, payload []byte) error {
	var event events.SupportInquiry
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode support inquiry event: %w", err)
	}

	chatIDs := d.supportChatIDs(ctx)
	if len(chatIDs) == 0 {
		// Deliberately loud: a shopper has been told a person will answer, and
		// nobody is listening. No chat id and no message body in the log —
		// only that the drop happened.
		log.Printf("support inquiry %s skipped: no chat has opted into support alerts (alert_support)", event.InquiryID)
		return nil
	}

	message := formatSupportInquiryMessage(event)
	if event.AfterHours {
		if silent, ok := d.notifier.(SilentNotifier); ok {
			return d.sendAllSilent(ctx, silent, chatIDs, message)
		}
	}
	return d.sendAll(ctx, chatIDs, message, "support inquiry")
}

// formatSupportInquiryMessage renders the doorbell.
//
// It quotes the excerpt the publisher chose and links to the manager inbox; the
// conversation itself stays behind that authenticated screen rather than being
// copied into an ops chat.
func formatSupportInquiryMessage(event events.SupportInquiry) string {
	var b strings.Builder
	if event.AfterHours {
		b.WriteString("🌙 <b>상담 요청</b> (영업시간 외)\n")
	} else {
		b.WriteString("🙋 <b>상담 요청</b>\n")
	}
	b.WriteString(fmt.Sprintf("문의 번호: <code>%s</code>\n", tg.EscapeHTML(event.InquiryID)))
	if topic := supportTopicLabel(event.Topic); topic != "" {
		b.WriteString(fmt.Sprintf("분류: %s\n", topic))
	}
	if user := strings.TrimSpace(event.Username); user != "" {
		b.WriteString(fmt.Sprintf("고객: @%s\n", tg.EscapeHTML(user)))
	}
	if ctxHint := strings.TrimSpace(event.EntryContext); ctxHint != "" {
		b.WriteString(fmt.Sprintf("유입: <code>%s</code>\n", tg.EscapeHTML(ctxHint)))
	}
	if excerpt := strings.TrimSpace(event.Excerpt); excerpt != "" {
		b.WriteString(fmt.Sprintf("\n<blockquote>%s</blockquote>\n", tg.EscapeHTML(excerpt)))
	}
	if url := strings.TrimSpace(event.ManageURL); url != "" {
		b.WriteString(fmt.Sprintf("\n<a href=\"%s\">상담 열기</a>", tg.EscapeHTML(url)))
	}
	return b.String()
}

// supportTopicLabel names the menu node the shopper escalated from.
func supportTopicLabel(topic string) string {
	switch topic {
	case "ord", "ord.eta", "ord.trk", "ord.adr":
		return "주문·배송"
	case "prd":
		return "상품·재고"
	case "ret":
		return "교환·반품"
	case "pay":
		return "결제"
	case "agt":
		return "상담원 연결"
	default:
		return ""
	}
}

func (d *Dispatcher) sendAllSilent(ctx context.Context, silent SilentNotifier, chatIDs []string, message string) error {
	var errs []error
	for _, chatID := range chatIDs {
		if err := silent.SendSilent(ctx, chatID, message); err != nil {
			errs = append(errs, fmt.Errorf("notify support inquiry to chat %s: %w", chatID, err))
		}
	}
	return errors.Join(errs...)
}

// supportChatIDs has no env fallback. The order and product chat ids are
// transitional bootstrap config from before the subscription table existed; a
// chat configured for order alerts never asked to field consultations.
func (d *Dispatcher) supportChatIDs(ctx context.Context) []string {
	if d.cfg.Routing == nil {
		return nil
	}
	var set chatSet
	for _, id := range d.cfg.Routing.SupportChatIDs(ctx) {
		set.add(id)
	}
	return set.list()
}

func (d *Dispatcher) orderChatIDs(ctx context.Context) []string {
	var routed []string
	if d.cfg.Routing != nil {
		routed = d.cfg.Routing.OrderChatIDs(ctx)
	}
	return mergeChatIDs(routed, d.cfg.OrderChatID)
}

func (d *Dispatcher) productChatIDs(ctx context.Context) []string {
	var routed []string
	if d.cfg.Routing != nil {
		routed = d.cfg.Routing.ProductChatIDs(ctx)
	}
	return mergeChatIDs(routed, d.cfg.ProductChatID)
}

// mergeChatIDs keeps the routed destinations and adds the static fallback for
// the case where no routing is wired at all; routing already unions the env
// chat IDs, so the fallback is normally a duplicate and drops out.
func mergeChatIDs(routed []string, fallback string) []string {
	var set chatSet
	for _, id := range routed {
		set.add(id)
	}
	set.add(fallback)
	return set.list()
}

// sendAll delivers one message to every destination, reporting failures
// together so that one unreachable chat cannot silence the rest.
func (d *Dispatcher) sendAll(ctx context.Context, chatIDs []string, message, what string) error {
	var errs []error
	for _, chatID := range chatIDs {
		if err := d.notifier.Send(ctx, chatID, message); err != nil {
			errs = append(errs, fmt.Errorf("notify %s to chat %s: %w", what, chatID, err))
		}
	}
	return errors.Join(errs...)
}

func formatOrderMessage(subject string, event events.Order, manageWebURL string) string {
	items := make([]string, 0, len(event.Items))
	for _, item := range event.Items {
		items = append(items, fmt.Sprintf("%d× %s", item.Quantity, tg.EscapeHTML(item.SKU)))
	}
	itemsLine := strings.Join(items, ", ")
	if itemsLine == "" {
		itemsLine = "상품 없음"
	}

	total := formatMoney(event.TotalWon)
	createdLine := formatOrderCreatedAt(event.CreatedAt, event.Occurred)
	manageLink := formatManageOrderLink(manageWebURL, event.OrderID)
	status := formatOrderStatus(event.Status)

	switch subject {
	case SubjectOrderPaid:
		return fmt.Sprintf(
			"💳 <b>주문 결제 완료 — 조치 필요</b> %s\n%s%s상태: <b>%s</b>\n고객: %s\n상품: %s\n합계: <b>%s</b>\n준비되면 출고하세요.",
			tg.EscapeHTML(event.OrderID),
			createdLine,
			manageLink,
			status,
			tg.EscapeHTML(event.CustomerID),
			itemsLine,
			total,
		)
	case SubjectOrderCreated:
		return fmt.Sprintf(
			"🛒 <b>신규 주문</b> %s\n%s%s상태: <b>%s</b>\n고객: %s\n상품: %s\n합계: <b>%s</b>",
			tg.EscapeHTML(event.OrderID),
			createdLine,
			manageLink,
			status,
			tg.EscapeHTML(event.CustomerID),
			itemsLine,
			total,
		)
	default:
		return fmt.Sprintf(
			"📦 <b>주문 변경</b> %s\n%s%s상태: <b>%s</b>\n고객: %s\n합계: <b>%s</b>",
			tg.EscapeHTML(event.OrderID),
			createdLine,
			manageLink,
			status,
			tg.EscapeHTML(event.CustomerID),
			total,
		)
	}
}

func formatOrderCreatedAt(createdAt, occurredAt time.Time) string {
	t := createdAt
	if t.IsZero() {
		t = occurredAt
	}
	if t.IsZero() {
		return ""
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.UTC
	}
	return fmt.Sprintf("주문 시각: <b>%s</b>\n", tg.EscapeHTML(t.In(loc).Format("2006-01-02 15:04 KST")))
}

func formatManageOrderLink(manageWebURL, orderID string) string {
	manageWebURL = strings.TrimRight(strings.TrimSpace(manageWebURL), "/")
	orderID = strings.TrimSpace(orderID)
	if manageWebURL == "" || orderID == "" {
		return ""
	}
	url := fmt.Sprintf("%s/orders/%s", manageWebURL, tg.EscapeHTML(orderID))
	return fmt.Sprintf("<a href=\"%s\">관리자에서 주문 보기</a>\n", url)
}

func formatProductMessage(subject string, event events.Product) string {
	price := money.FormatWon(money.FromProductPrice(event.Price))
	name := tg.EscapeHTML(event.Name)
	brand := tg.EscapeHTML(event.Brand)
	id := tg.EscapeHTML(event.ProductID)
	status := formatProductStatus(event.Status)

	switch subject {
	case SubjectProductCreated:
		return fmt.Sprintf("📦 <b>상품 등록</b>\n%s — %s (%s)\n카테고리: %s\n상태: %s\n가격: %s",
			id, name, brand, tg.EscapeHTML(event.Category), status, price)
	case SubjectProductUpdated:
		return fmt.Sprintf("✏️ <b>상품 수정</b>\n%s — %s (%s)\n상태: %s\n가격: %s",
			id, name, brand, status, price)
	case SubjectProductDeleted:
		return fmt.Sprintf("🗑️ <b>상품 삭제</b>\n%s — %s", id, name)
	case SubjectProductImage:
		return fmt.Sprintf("🖼️ <b>상품 이미지 업로드</b>\n%s — %s\n%s",
			id, name, tg.EscapeHTML(event.ImageURL))
	default:
		return fmt.Sprintf("상품 이벤트 %s — %s", tg.EscapeHTML(subject), id)
	}
}

// formatOrderStatus maps wire statuses to the Korean labels ops already see
// in manage-web. Unknown values stay escaped as-is.
func formatOrderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "대기 중"
	case "paid":
		return "결제 완료"
	case "confirmed":
		return "확인됨"
	case "in_transit":
		return "배송 중"
	case "delivered":
		return "배송 완료"
	case "fulfilled":
		return "완료"
	case "disputed":
		return "분쟁"
	case "canceled", "cancelled":
		return "취소됨"
	default:
		return tg.EscapeHTML(status)
	}
}

func formatProductStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "활성"
	case "draft":
		return "초안"
	case "archived":
		return "보관됨"
	default:
		return tg.EscapeHTML(status)
	}
}

func formatMoney(won int64) string {
	return money.FormatWon(won)
}
