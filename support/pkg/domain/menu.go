package domain

import "strings"

// callbackVersion prefixes every callback_data value.
//
// Telegram keeps old messages tappable forever, so a menu shipped today can be
// tapped a year from now. The version lets a later redesign recognise a stale
// tap and answer it with the current root menu instead of misrouting it.
const callbackVersion = "v1"

// Node is one place in the consultation menu.
//
// Children are the buttons shown when the node is open, in order. A node with
// Escalates set hands the conversation to a human — the inquiry itself lands in
// Phase 4 (docs/support-telegram-bot.md); until then the node renders its copy
// and nothing is queued.
type Node struct {
	ID        string
	Label     string
	Children  []string
	Escalates bool
}

// Menu node ids. Short because they travel in callback_data, which Telegram
// caps at 64 bytes for the whole string.
const (
	NodeRoot     = "root"
	NodeOrder    = "ord"
	NodeOrderETA = "ord.eta"
	NodeOrderTrk = "ord.trk"
	NodeOrderAdr = "ord.adr"
	NodeProduct  = "prd"
	NodeReturn   = "ret"
	NodePayment  = "pay"
	NodeAgent    = "agt"
)

// BackLabel returns to the root menu from anywhere. Every non-root node carries
// it, so a shopper is never stranded down a branch.
const BackLabel = "⬅️ 처음으로"

// Nodes is the whole menu. One map, so a node cannot exist as a button without
// existing as a destination.
var Nodes = map[string]Node{
	NodeRoot: {
		ID:       NodeRoot,
		Children: []string{NodeOrder, NodeProduct, NodeReturn, NodePayment, NodeAgent},
	},
	NodeOrder: {
		ID:       NodeOrder,
		Label:    "📦 주문·배송 문의",
		Children: []string{NodeOrderETA, NodeOrderTrk, NodeOrderAdr, NodeAgent},
	},
	NodeOrderETA: {ID: NodeOrderETA, Label: "🚚 배송 기간", Children: []string{NodeAgent}},
	NodeOrderTrk: {ID: NodeOrderTrk, Label: "🔎 배송 조회", Children: []string{NodeAgent}},
	NodeOrderAdr: {ID: NodeOrderAdr, Label: "🏠 주소 변경", Children: []string{NodeAgent}},
	NodeProduct: {
		ID:       NodeProduct,
		Label:    "🛍 상품·재고 문의",
		Children: []string{NodeAgent},
	},
	NodeReturn: {
		ID:       NodeReturn,
		Label:    "🔁 교환·반품",
		Children: []string{NodeAgent},
	},
	NodePayment: {
		ID:       NodePayment,
		Label:    "💳 결제 문의",
		Children: []string{NodeAgent},
	},
	NodeAgent: {
		ID:        NodeAgent,
		Label:     "🙋 상담원 연결",
		Escalates: true,
	},
}

// RootMenu is the opening menu's buttons, in order.
func RootMenu() []Node {
	return ChildrenOf(NodeRoot)
}

// ChildrenOf returns the child nodes of a node, skipping any id with no
// definition — a button that leads nowhere is worse than a missing button.
func ChildrenOf(id string) []Node {
	node, ok := Nodes[id]
	if !ok {
		return nil
	}
	children := make([]Node, 0, len(node.Children))
	for _, childID := range node.Children {
		if child, ok := Nodes[childID]; ok {
			children = append(children, child)
		}
	}
	return children
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
	_, ok := Nodes[node]
	return ok
}
