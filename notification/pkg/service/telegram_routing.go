package service

import (
	"context"

	"github.com/elug3/dupli1/notification/pkg/ports"
)

type TelegramRouting struct {
	subs *TelegramSubscriptions
	env  *ports.TelegramEnvAllowlist
}

func NewTelegramRouting(subs *TelegramSubscriptions, env *ports.TelegramEnvAllowlist) *TelegramRouting {
	return &TelegramRouting{subs: subs, env: env}
}

func (r *TelegramRouting) OrderChatIDs(ctx context.Context) []string {
	if r == nil || r.subs == nil {
		return nil
	}
	order, _ := r.subs.RoutingChats(ctx, r.env)
	return order
}

// SupportChatIDs returns the chats opted into customer inquiry handoffs.
func (r *TelegramRouting) SupportChatIDs(ctx context.Context) []string {
	if r == nil || r.subs == nil {
		return nil
	}
	return r.subs.SupportChats(ctx)
}

func (r *TelegramRouting) ProductChatIDs(ctx context.Context) []string {
	if r == nil || r.subs == nil {
		return nil
	}
	_, product := r.subs.RoutingChats(ctx, r.env)
	return product
}
