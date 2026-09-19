package domain

import "strings"

// callbackVersion prefixes every callback_data value.
//
// Telegram keeps old messages tappable forever, so a menu shipped today can be
// tapped a year from now. The version lets a later redesign recognise a stale
// tap and answer it with the current root menu instead of misrouting it.
const callbackVersion = "v1"

// MenuItem is one button on a menu: the label a shopper reads and the node it
// leads to.
type MenuItem struct {
	Label string
	Node  string
}

// RootMenu is the opening consultation menu. Order matters — it is the order
// the buttons appear in, most common request first.
var RootMenu = []MenuItem{
	{Label: "📦 주문·배송 문의", Node: NodeOrder},
	{Label: "🛍 상품·재고 문의", Node: NodeProduct},
	{Label: "🔁 교환·반품", Node: NodeReturn},
	{Label: "💳 결제 문의", Node: NodePayment},
	{Label: "🙋 상담원 연결", Node: NodeAgent},
}

// RootGreeting opens a consultation.
const RootGreeting = "안녕하세요, <b>Dupli1 고객센터</b>입니다.\n무엇을 도와드릴까요?"

// CallbackData encodes a node as the payload of an inline-keyboard button.
func CallbackData(node string) string {
	return callbackVersion + ":" + node
}

// ParseCallbackData returns the node a button tap refers to.
//
// A payload from an older menu version, or anything malformed, resolves to the
// root: an unrecognised tap re-opens the menu rather than erroring at someone
// who did nothing wrong.
func ParseCallbackData(data string) string {
	version, node, found := strings.Cut(strings.TrimSpace(data), ":")
	if !found || version != callbackVersion {
		return NodeRoot
	}
	if !IsKnownNode(node) {
		return NodeRoot
	}
	return node
}

// IsKnownNode reports whether node is one this version of the menu serves.
func IsKnownNode(node string) bool {
	switch node {
	case NodeRoot, NodeOrder, NodeProduct, NodeReturn, NodePayment, NodeAgent:
		return true
	}
	return false
}
