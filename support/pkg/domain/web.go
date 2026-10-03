package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Channels a consultation can arrive through.
const (
	ChannelTelegram = "telegram"
	ChannelWeb      = "web"
)

// Message kinds.
const (
	MessageText       = "text"
	MessageProductRef = "product_ref"
	MessageOrderRef   = "order_ref"
	MessageSystem     = "system"
)

// Reply-notice outcomes, recorded on the manager reply they concern.
const (
	// NoticePending marks a web reply whose notice is not decided yet; the
	// notice job picks these up once the reply has gone unread long enough.
	NoticePending = "pending"
	NoticeSent    = "sent"
	NoticeFailed  = "failed"
	NoticeSkipped = "skipped"
)

// MaxWebMessageRunes bounds one web chat message. Telegram's own limit is
// 4096; a web textarea has none, so the bound is ours.
const MaxWebMessageRunes = 2000

// webChatPrefix namespaces web conversations inside chat_id, which is unique
// per conversation. A Telegram chat id is numeric, so the two never collide,
// and one customer keeps exactly one web conversation without a new key.
const webChatPrefix = "web:"

// WebChatID is the chat id of a customer's web conversation.
func WebChatID(customerID string) string {
	return webChatPrefix + customerID
}

// ChannelOf normalizes a stored channel: empty means Telegram, which every
// row was before web chat existed.
func ChannelOf(channel string) string {
	if strings.TrimSpace(channel) == "" {
		return ChannelTelegram
	}
	return channel
}

// IsWeb reports whether the inquiry came from the storefront chat.
func (i *Inquiry) IsWeb() bool {
	return i != nil && ChannelOf(i.Channel) == ChannelWeb
}

// IsWeb reports whether the conversation is a storefront chat.
func (c *Conversation) IsWeb() bool {
	return c != nil && ChannelOf(c.Channel) == ChannelWeb
}

// WebTopic derives a web inquiry's topic from its subject, so the existing
// topic labels (ops alert, inbox filters) keep meaning something: an order
// question is 주문·배송, a product question 상품·재고, anything else a plain
// request for a person.
func WebTopic(orderID, skuID string) string {
	switch {
	case strings.TrimSpace(orderID) != "":
		return NodeOrder
	case strings.TrimSpace(skuID) != "":
		return NodeProduct
	default:
		return NodeAgent
	}
}

// ValidWebBody reports whether text is a sendable chat message: something
// besides whitespace, within MaxWebMessageRunes.
func ValidWebBody(body string) bool {
	trimmed := strings.TrimSpace(body)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= MaxWebMessageRunes
}

// UnreadBy reports whether a message is a manager reply the shopper has not
// read yet. System lines (after-hours notes, "상담이 종료되었습니다") are not
// replies and never count.
func (m Message) UnreadBy(lastRead *time.Time) bool {
	if m.Direction != DirectionOutbound || m.Kind == MessageSystem {
		return false
	}
	return lastRead == nil || m.CreatedAt.After(*lastRead)
}

// NeedsNotice reports whether the shopper should be emailed now about
// unread replies: they have not read past the reply, and have not already
// been emailed since they last read. One email covers every reply until the
// shopper comes back.
func (c *Conversation) NeedsNotice(reply Message) bool {
	if c == nil || !reply.UnreadBy(c.CustomerLastReadAt) {
		return false
	}
	if c.CustomerNotifiedAt == nil {
		return true
	}
	if c.CustomerLastReadAt == nil {
		return false
	}
	return c.CustomerNotifiedAt.Before(*c.CustomerLastReadAt)
}
