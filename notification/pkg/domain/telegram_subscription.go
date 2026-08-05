package domain

import "time"

const (
	SubscriptionStatusPending  = "pending"
	SubscriptionStatusAccepted = "accepted"
	SubscriptionStatusRejected = "rejected"
)

// TelegramSubscription is a Telegram user or chat registered for ops alerts.
type TelegramSubscription struct {
	ID              string     `json:"id"`
	TelegramUserID  *int64     `json:"telegram_user_id,omitempty"`
	ChatID          string     `json:"chat_id"`
	ChatType        string     `json:"chat_type,omitempty"`
	ChatLabel       string     `json:"chat_label,omitempty"`
	Username        string     `json:"username,omitempty"`
	Status          string     `json:"status"`
	AlertOrder      bool       `json:"alert_order"`
	AlertProduct    bool       `json:"alert_product"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	AcceptedBy      string     `json:"accepted_by,omitempty"`
}

func (s TelegramSubscription) IsAccepted() bool {
	return s.Status == SubscriptionStatusAccepted
}
