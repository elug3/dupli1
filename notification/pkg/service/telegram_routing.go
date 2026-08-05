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

func (r *TelegramRouting) OrderChatID(ctx context.Context) string {
	if r == nil || r.subs == nil {
		return ""
	}
	order, _ := r.subs.RoutingChats(ctx, r.env)
	return order
}

func (r *TelegramRouting) ProductChatID(ctx context.Context) string {
	if r == nil || r.subs == nil {
		return ""
	}
	_, product := r.subs.RoutingChats(ctx, r.env)
	return product
}
