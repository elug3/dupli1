package domain

// DefaultAnswers is the copy each node opens with, seeded into the database on
// first start.
//
// The database is the source of truth once seeded, so staff can edit wording
// from the manager inbox without a deploy; seeding uses "insert if absent" and
// never overwrites an edit. This map is the starting text, not the live text.
//
// Two rules the copy obeys, from docs/support-telegram-bot.md:
//   - Never name a day the bot will reply. There is no holiday calendar, so
//     "내일" said on the eve of Chuseok is wrong by four days. State the window.
//   - Korean only at launch.
var DefaultAnswers = map[string]string{
	NodeOrder: "📦 <b>주문·배송 문의</b>\n\n" +
		"어떤 도움이 필요하신가요? 아래에서 선택해 주세요.",

	NodeOrderETA: "🚚 <b>배송 기간</b>\n\n" +
		"결제 확인 후 영업일 기준 <b>2~3일</b> 이내 출고되며, " +
		"출고 후 수령까지 보통 <b>1~2일</b>이 더 걸립니다.\n" +
		"주문 상태는 마이페이지 &gt; 주문 내역에서 확인하실 수 있습니다.",

	NodeOrderTrk: "🔎 <b>배송 조회</b>\n\n" +
		"마이페이지 &gt; 주문 내역에서 운송장 번호를 확인하실 수 있습니다.\n" +
		"운송장이 아직 없다면 출고 준비 중입니다.",

	NodeOrderAdr: "🏠 <b>주소 변경</b>\n\n" +
		"출고 전에는 주소를 변경해 드릴 수 있습니다. " +
		"이미 출고된 주문은 택배사를 통해 수취인이 직접 변경하셔야 합니다.\n" +
		"변경이 필요하시면 상담원 연결을 눌러 주문번호와 새 주소를 남겨 주세요.",

	NodeProduct: "🛍 <b>상품·재고 문의</b>\n\n" +
		"사이즈, 소재, 색상, 재고 여부를 확인해 드립니다.\n" +
		"상품 페이지의 상품명 또는 링크를 남겨 주시면 더 빠르게 안내해 드릴 수 있습니다.",

	NodeReturn: "🔁 <b>교환·반품</b>\n\n" +
		"수령 후 <b>14일</b> 이내 교환·반품이 가능합니다. " +
		"상품은 사용하지 않은 상태로 원래 포장과 구성품이 모두 있어야 합니다.\n" +
		"진행을 원하시면 상담원 연결을 눌러 주문번호와 사유를 남겨 주세요.",

	NodePayment: "💳 <b>결제 문의</b>\n\n" +
		"카드 결제를 지원하며, 결제 금액은 모두 원화(KRW)로 표시됩니다.\n" +
		"결제가 완료되었는데 주문이 보이지 않는다면 상담원 연결을 눌러 결제 시각을 알려 주세요.",

	NodeAgent: "🙋 <b>상담원 연결</b>\n\n" +
		"문의 내용을 이 채팅에 남겨 주세요. 주문번호가 있으면 함께 적어 주시면 빠릅니다.\n" +
		"상담 시간은 <b>평일 10:00~22:00</b>이며, 공휴일은 휴무입니다. " +
		"상담 시간에 순서대로 답변드립니다.",
}

// AnswerFor returns the seeded copy for a node, or the greeting for the root.
func AnswerFor(node string) string {
	if node == NodeRoot {
		return RootGreeting
	}
	return DefaultAnswers[node]
}
