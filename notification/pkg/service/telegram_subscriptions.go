package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/jackc/pgx/v4"
)

// ErrIdentifierRequired reports a manual subscription with neither a Telegram
// user ID nor a chat ID — a bad request, distinct from a failing store.
var ErrIdentifierRequired = errors.New("telegram_user_id or chat_id is required")

// ErrNoAlertChange reports an alert update that sets no flag — a bad request.
var ErrNoAlertChange = errors.New("at least one of alert_order, alert_product, alert_support or muted_events is required")

// ErrUnknownEvent reports a muted_events entry that is not a message a chat can
// mute — a bad request, refused rather than stored as a mute that silences
// nothing.
var ErrUnknownEvent = errors.New("unknown event in muted_events")

type TelegramSubscriptions struct {
	repo ports.TelegramRepository
}

func NewTelegramSubscriptions(repo ports.TelegramRepository) *TelegramSubscriptions {
	return &TelegramSubscriptions{repo: repo}
}

func (s *TelegramSubscriptions) Enabled() bool {
	return s != nil && s.repo != nil
}

func (s *TelegramSubscriptions) RegisterFromMessage(ctx context.Context, in ports.TelegramSubscriptionInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.UpsertPending(ctx, in)
}

func (s *TelegramSubscriptions) List(ctx context.Context, status string) ([]domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.List(ctx, status)
}

func (s *TelegramSubscriptions) CreateManual(ctx context.Context, in ports.TelegramManualInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	if in.TelegramUserID == nil && strings.TrimSpace(in.ChatID) == "" {
		return nil, ErrIdentifierRequired
	}
	return s.repo.CreateAccepted(ctx, in)
}

func (s *TelegramSubscriptions) Accept(ctx context.Context, id string, in ports.TelegramAcceptInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	muted, err := normalizeMuted(in.MutedEvents)
	if err != nil {
		return nil, err
	}
	in.MutedEvents = muted
	return s.repo.Accept(ctx, id, in)
}

// normalizeMuted validates, sorts and de-duplicates a muted list; nil stays
// nil, meaning "leave it as it is".
func normalizeMuted(events *[]string) (*[]string, error) {
	if events == nil {
		return nil, nil
	}
	out, unknown := domain.NormalizeMutedEvents(*events)
	if unknown != "" {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEvent, unknown)
	}
	return &out, nil
}

func (s *TelegramSubscriptions) Reject(ctx context.Context, id, rejectedBy string) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.Reject(ctx, id, rejectedBy)
}

func (s *TelegramSubscriptions) Get(ctx context.Context, id string) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.GetByID(ctx, id)
}

// UpdateAlerts changes which alert classes a subscription receives, after it
// was accepted as well as before.
func (s *TelegramSubscriptions) UpdateAlerts(ctx context.Context, id string, in ports.TelegramAlertsInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	if in.Empty() {
		return nil, ErrNoAlertChange
	}
	muted, err := normalizeMuted(in.MutedEvents)
	if err != nil {
		return nil, err
	}
	in.MutedEvents = muted
	return s.repo.UpdateAlerts(ctx, id, in)
}

func (s *TelegramSubscriptions) Delete(ctx context.Context, id string) error {
	if !s.Enabled() {
		return fmt.Errorf("telegram repository not configured")
	}
	return s.repo.Delete(ctx, id)
}

// UpdateMetadata refreshes the display fields of a subscription that already
// exists, so a renamed group does not keep its old label in the manager UI.
func (s *TelegramSubscriptions) UpdateMetadata(ctx context.Context, id string, in ports.TelegramMetadataInput) error {
	if !s.Enabled() {
		return fmt.Errorf("telegram repository not configured")
	}
	return s.repo.UpdateMetadata(ctx, id, in)
}

func (s *TelegramSubscriptions) LookupForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if sub, err := s.repo.FindByChatID(ctx, chatID); err == nil {
		return sub, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if userID != nil {
		if sub, err := s.repo.FindByUserID(ctx, *userID); err == nil {
			return sub, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

func (s *TelegramSubscriptions) IsAllowedIncoming(ctx context.Context, chatID string, userID *int64, env *ports.TelegramEnvAllowlist) bool {
	if userID != nil && env != nil && env.AllowsUser(*userID) {
		return true
	}
	if env != nil && env.AllowsChat(chatID) {
		return true
	}
	sub, err := s.LookupForMessage(ctx, chatID, userID)
	if err != nil || sub == nil {
		return false
	}
	return sub.IsAccepted()
}

// RoutingChats returns every chat that should receive event among the order
// and product alerts: the transitional env destinations first, then each
// accepted subscription carrying the matching flag, without duplicates. A chat
// that muted event is left out; an empty event ignores mutes.
//
// Env and database destinations are unioned rather than one overriding the
// other. Preferring env meant that as long as TELEGRAM_ORDER_CHAT_ID was set —
// which it is in production — accepting a chat in manage-web did nothing at
// all, so the whole subscription UI was inert wherever the fallback existed.
// For the same reason a mute recorded on the env chat's own row applies to the
// env destination too; an env chat with no row has nowhere to record one and
// receives everything.
func (s *TelegramSubscriptions) RoutingChats(ctx context.Context, env *ports.TelegramEnvAllowlist, event string) (orderChatIDs, productChatIDs []string) {
	var order, product chatSet
	var accepted []domain.TelegramSubscription
	if s.Enabled() {
		// A failed read still alerts the env chats, unmuted.
		accepted, _ = s.repo.ListAccepted(ctx)
	}
	muted := make(map[string]struct{})
	if event != "" {
		for _, sub := range accepted {
			if sub.Mutes(event) {
				muted[strings.TrimSpace(sub.ChatID)] = struct{}{}
			}
		}
	}
	addUnmuted := func(set *chatSet, chatID string) {
		if _, ok := muted[strings.TrimSpace(chatID)]; !ok {
			set.add(chatID)
		}
	}
	if env != nil {
		addUnmuted(&order, env.OrderChatID)
		addUnmuted(&product, env.ProductChatID)
	}
	for _, sub := range accepted {
		if sub.AlertOrder {
			addUnmuted(&order, sub.ChatID)
		}
		if sub.AlertProduct {
			addUnmuted(&product, sub.ChatID)
		}
	}
	return order.list(), product.list()
}

// SupportChats returns the chats that opted into customer inquiry handoffs.
//
// No env fallback: the order and product chat ids are transitional bootstrap
// config from before the subscription table existed, and a chat that never
// asked for consultations should not start receiving them because it was
// configured for order alerts.
func (s *TelegramSubscriptions) SupportChats(ctx context.Context) []string {
	var support chatSet
	if !s.Enabled() {
		return support.list()
	}
	accepted, err := s.repo.ListAccepted(ctx)
	if err != nil {
		return support.list()
	}
	for _, sub := range accepted {
		if sub.AlertSupport {
			support.add(sub.ChatID)
		}
	}
	return support.list()
}

// chatSet collects chat IDs in insertion order, trimming blanks and repeats so
// a chat listed in both env and the database is alerted once.
type chatSet struct {
	ids  []string
	seen map[string]struct{}
}

func (c *chatSet) add(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	if _, ok := c.seen[chatID]; ok {
		return
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	c.seen[chatID] = struct{}{}
	c.ids = append(c.ids, chatID)
}

func (c *chatSet) list() []string { return c.ids }

func (s *TelegramSubscriptions) AllowedChatIDs(ctx context.Context, env *ports.TelegramEnvAllowlist) (map[string]struct{}, error) {
	allowed := make(map[string]struct{})
	if env != nil {
		for _, id := range env.ChatIDs() {
			allowed[id] = struct{}{}
		}
	}
	if !s.Enabled() {
		return allowed, nil
	}
	accepted, err := s.repo.ListAccepted(ctx)
	if err != nil {
		return nil, err
	}
	for _, sub := range accepted {
		allowed[sub.ChatID] = struct{}{}
	}
	return allowed, nil
}
