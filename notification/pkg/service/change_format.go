package service

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/money"

	tg "github.com/elug3/dupli1/shared/pkg/telegram"
)

// This file renders what an order or product event changed: the status an
// order left beside the one it is in, and each product field's old and new
// value. Events from before the publishers sent this carry none of it and
// render as they always did.

// orderChangeTitle names the action, "주문 변경 · 출고". Without an action
// (an older event) it stays "주문 변경".
func orderChangeTitle(event events.Order) string {
	label := orderChangeLabel(event.Change)
	if label == "" {
		return "주문 변경"
	}
	if event.Automatic {
		label += " (자동)"
	}
	return "주문 변경 · " + label
}

func orderChangeLabel(change string) string {
	switch change {
	case events.OrderChangePaid:
		return "결제 완료"
	case events.OrderChangeConfirmed:
		return "주문 확정"
	case events.OrderChangeShipped:
		return "출고"
	case events.OrderChangeDelivered:
		return "배송 완료"
	case events.OrderChangeReceiptConfirmed:
		return "고객 수령 확인"
	case events.OrderChangeDisputed:
		return "미수령 신고"
	case events.OrderChangeDisputeResolved:
		return "분쟁 종료 (배송 확인)"
	case events.OrderChangeFulfilled:
		return "구매 확정"
	case events.OrderChangeCanceled:
		return "주문 취소"
	case events.OrderChangeCancelRequested:
		return "취소 요청 접수"
	case events.OrderChangeCancelApproved:
		return "취소 요청 승인"
	case events.OrderChangeCancelRejected:
		return "취소 요청 거절"
	case events.OrderChangePaymentExpired:
		return "미결제 자동 취소"
	case events.OrderChangeRefunded:
		return "전액 환불로 취소"
	default:
		return ""
	}
}

// automaticReason says which policy acted, for an automatic change.
func automaticReason(change string) string {
	switch change {
	case events.OrderChangeConfirmed:
		return "2시간 안에 확정하지 않아 자동 확정"
	case events.OrderChangeCancelApproved:
		return "2시간 안에 처리하지 않아 자동 승인"
	case events.OrderChangeFulfilled:
		return "배송 완료 후 14일 동안 응답이 없어 자동 구매 확정"
	case events.OrderChangePaymentExpired:
		return "결제 기한이 지나 자동 취소"
	default:
		return "자동 처리"
	}
}

// formatOrderChangeLines writes the status line and whatever detail the
// action carries, each ending in a newline.
func formatOrderChangeLines(event events.Order) string {
	var b strings.Builder
	status := formatOrderStatus(event.Status)
	switch prev := strings.TrimSpace(event.PreviousStatus); {
	case prev == "":
		b.WriteString(fmt.Sprintf("상태: <b>%s</b>\n", status))
	case strings.EqualFold(prev, event.Status):
		b.WriteString(fmt.Sprintf("상태: <b>%s</b> (변경 없음)\n", status))
	default:
		b.WriteString(fmt.Sprintf("상태: %s → <b>%s</b>\n", formatOrderStatus(prev), status))
	}
	if event.Automatic {
		b.WriteString(fmt.Sprintf("처리: %s\n", automaticReason(event.Change)))
	}
	if event.Change == events.OrderChangeShipped {
		if carrier := formatCarrier(event.Carrier, event.CarrierNote); carrier != "" || event.TrackingNumber != "" {
			b.WriteString(fmt.Sprintf("송장: %s\n", strings.TrimSpace(carrier+" "+tg.EscapeHTML(event.TrackingNumber))))
		}
	}
	if reason := strings.TrimSpace(event.Reason); reason != "" {
		label := "사유"
		switch event.Change {
		case events.OrderChangeCancelRequested:
			label = "취소 사유"
		case events.OrderChangeDisputed:
			label = "신고 사유"
		}
		b.WriteString(fmt.Sprintf("%s: %s\n", label, tg.EscapeHTML(truncateRunes(reason, 200))))
	}
	return b.String()
}

// formatCarrier names the carrier the way ops know it; "other" is the free
// text staff typed.
func formatCarrier(carrier, note string) string {
	switch strings.ToLower(strings.TrimSpace(carrier)) {
	case "":
		return ""
	case "cj":
		return "CJ대한통운"
	case "hanjin":
		return "한진택배"
	case "lotte":
		return "롯데택배"
	case "logen":
		return "로젠택배"
	case "epost":
		return "우체국택배"
	case "other":
		if n := strings.TrimSpace(note); n != "" {
			return tg.EscapeHTML(n)
		}
		return "기타"
	default:
		return tg.EscapeHTML(carrier)
	}
}

// formatProductUpdatedMessage lists each changed field as "before → after".
// An event with no changes is either a save that changed nothing or one from
// before the field existed; both show the product as it now is.
func formatProductUpdatedMessage(event events.Product) string {
	var b strings.Builder
	if event.Reason == events.ProductUpdateAutoDraft {
		b.WriteString("✏️ <b>상품 자동 초안 전환</b>\n")
	} else {
		b.WriteString("✏️ <b>상품 수정</b>\n")
	}
	b.WriteString(fmt.Sprintf("%s — %s (%s)\n",
		tg.EscapeHTML(event.ProductID), tg.EscapeHTML(event.Name), tg.EscapeHTML(event.Brand)))
	if event.Reason == events.ProductUpdateAutoDraft {
		b.WriteString("판매 중인 SKU가 없어 초안으로 전환되었습니다.\n")
	}

	if len(event.Changes) == 0 {
		b.WriteString(fmt.Sprintf("상태: %s\n가격: %s",
			formatProductStatus(event.Status), money.FormatWon(money.FromProductPrice(event.Price))))
		return b.String()
	}
	b.WriteString("\n")
	for i, c := range event.Changes {
		if i == maxProductChangeLines {
			b.WriteString(fmt.Sprintf("외 %d개 항목 변경\n", len(event.Changes)-i))
			break
		}
		b.WriteString(fmt.Sprintf("%s: %s → <b>%s</b>\n",
			productFieldLabel(c.Field), productFieldValue(c.Field, c.Before), productFieldValue(c.Field, c.After)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// maxProductChangeLines keeps a sweeping edit well inside Telegram's 4096
// characters; each value is cut to maxProductValueRunes as well.
const (
	maxProductChangeLines = 20
	maxProductValueRunes  = 80
)

func productFieldLabel(field string) string {
	if key, ok := strings.CutPrefix(field, "attributes."); ok {
		return "속성 " + tg.EscapeHTML(key)
	}
	switch field {
	case "name":
		return "상품명"
	case "status":
		return "상태"
	case "price":
		return "판매가"
	case "officialPrice":
		return "정가"
	case "brand":
		return "브랜드"
	case "brandCode":
		return "브랜드 코드"
	case "styleCode":
		return "스타일 코드"
	case "category":
		return "카테고리"
	case "subCategory":
		return "세부 카테고리"
	case "style":
		return "스타일"
	case "target":
		return "대상"
	case "material":
		return "소재"
	case "capacity":
		return "용량"
	case "description":
		return "설명"
	case "tags":
		return "태그"
	case "sizeChart":
		return "사이즈 가이드"
	default:
		return tg.EscapeHTML(field)
	}
}

func productFieldValue(field, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(없음)"
	}
	switch field {
	case "price", "officialPrice":
		if won, err := strconv.ParseInt(value, 10, 64); err == nil {
			return money.FormatWon(won)
		}
	case "status":
		return formatProductStatus(value)
	}
	return tg.EscapeHTML(truncateRunes(strings.Join(strings.Fields(value), " "), maxProductValueRunes))
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
