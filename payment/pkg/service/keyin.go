package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// Key-in card payment (NANO 수기결제, docs/payment-nano-key-in.md).
//
// The shopper types the card on the storefront; this charges it once through
// the PG. The card is never stored, logged or returned. What can go wrong is
// handled so that a shopper is never invited to pay twice for one charge:
//   - a typed card that fails our own checks never reaches the PG
//   - one approval runs per payment at a time (BeginCardAttempt under the row lock)
//   - a decline leaves the payment open for another card, up to MaxCardAttempts
//   - a call the PG never answered holds the payment and alerts ops, because the
//     card may have been charged

// NanoSourceKeyIn marks an ops alert raised by a key-in approval.
const NanoSourceKeyIn = "key_in"

// Reasons carried on the payment.callback_rejected alert for key-in.
const (
	// KeyInRejectUnconfirmed — the approval was sent and no answer came back.
	KeyInRejectUnconfirmed NanoRejectReason = events.ReasonKeyInUnconfirmed
)

// WithCardApprover enables key-in card payments. shopCode is the merchant's
// NANO shop code, checked against the approval's echo.
func (s *Service) WithCardApprover(approver ports.CardApprover, shopCode string) *Service {
	s.cardApprover = approver
	s.nanoShopCode = strings.TrimSpace(shopCode)
	return s
}

// KeyInEnabled reports whether card payments take a typed card.
func (s *Service) KeyInEnabled() bool { return s.cardApprover != nil }

// PayByCardInput is one key-in attempt by the shopper who owns the payment.
type PayByCardInput struct {
	PaymentID   string
	CustomerID  string
	BearerToken string
	Card        domain.CardDetails
}

// CardDeclinedError is a PG decline, with the PG's own message for the
// shopper (wrong PIN, over the limit, …) and how many attempts are left.
type CardDeclinedError struct {
	Code              string
	Message           string
	AttemptsRemaining int
}

func (e *CardDeclinedError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", domain.ErrCardDeclined, e.Message, e.Code)
}

func (e *CardDeclinedError) Is(target error) bool {
	if target == domain.ErrCardDeclined {
		return true
	}
	// The last decline also fails the payment.
	return e.AttemptsRemaining == 0 && target == domain.ErrCardAttemptsExhausted
}

// PayByCard charges a typed card for a payment awaiting key-in.
func (s *Service) PayByCard(ctx context.Context, input PayByCardInput) (*domain.Payment, error) {
	if s.cardApprover == nil {
		return nil, ports.ErrMethodUnavailable
	}
	payment, err := s.repo.Get(ctx, strings.TrimSpace(input.PaymentID))
	if err != nil {
		return nil, err
	}
	if payment.CustomerID != input.CustomerID {
		return nil, ports.ErrOrderForbidden
	}
	if payment.Status == domain.StatusSucceeded {
		// A retried submit of a card that already went through.
		return payment, nil
	}
	if payment.Provider != domain.ProviderNano || payment.Method != domain.MethodCreditCard {
		return nil, domain.ErrInvalidPayment
	}
	if err := s.CheckMethodAllowed(ctx, domain.MethodCreditCard); err != nil {
		return nil, err
	}
	card, err := domain.NormalizeCard(input.Card, payment.AmountWon, s.now())
	if err != nil {
		return nil, err
	}
	// The order must still be waiting for this payment, at this amount: an
	// order that auto-canceled while the shopper typed must not be charged.
	order, err := s.orders.GetOrder(ctx, input.BearerToken, payment.OrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != "pending" {
		return nil, ports.ErrOrderNotPending
	}
	if order.TotalWon != payment.AmountWon {
		return nil, domain.ErrInvalidPayment
	}

	claimed, err := s.repo.MutateLocked(ctx, payment.ID, func(p *domain.Payment) ([]ports.OutboxEvent, error) {
		return nil, p.BeginCardAttempt(s.now())
	})
	if err != nil {
		return nil, err
	}

	result, err := s.cardApprover.ApproveCard(ctx, ports.CardApprovalInput{
		PaymentID:  claimed.ID,
		OrderID:    claimed.OrderID,
		AmountWon:  claimed.AmountWon,
		OrderName:  claimed.PayerName,
		OrderTel:   claimed.PayerPhone,
		OrderEmail: claimed.PayerEmail,
		GoodsName:  "Dupli1 " + claimed.OrderID,
		Card:       card,
	})
	card = domain.CardDetails{}
	// The outcome is recorded even if the shopper's request was canceled.
	saveCtx := context.WithoutCancel(ctx)

	switch {
	case errors.Is(err, ports.ErrCardApprovalUnconfirmed):
		return nil, s.holdUnconfirmed(saveCtx, claimed, err)
	case err != nil:
		// Never sent: nothing was charged, and the attempt is given back.
		log.Printf("payment: key-in %s not sent: %v", claimed.ID, err)
		if _, saveErr := s.repo.MutateLocked(saveCtx, claimed.ID, func(p *domain.Payment) ([]ports.OutboxEvent, error) {
			p.CardNotSent(s.now())
			return nil, nil
		}); saveErr != nil {
			log.Printf("payment: key-in %s release claim: %v", claimed.ID, saveErr)
		}
		return nil, err
	case !result.Approved:
		return s.recordDecline(saveCtx, claimed, result)
	default:
		return s.recordApproval(saveCtx, claimed, result)
	}
}

func (s *Service) recordDecline(ctx context.Context, payment *domain.Payment, result *ports.CardApprovalResult) (*domain.Payment, error) {
	saved, err := s.repo.MutateLocked(ctx, payment.ID, func(p *domain.Payment) ([]ports.OutboxEvent, error) {
		p.CardDeclined(s.now())
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	log.Printf("payment: key-in %s declined: code=%s attempt=%d/%d",
		saved.ID, result.ResultCode, saved.CardAttempts, domain.MaxCardAttempts)
	remaining := domain.MaxCardAttempts - saved.CardAttempts
	if remaining < 0 {
		remaining = 0
	}
	return nil, &CardDeclinedError{
		Code:              result.ResultCode,
		Message:           shopperDeclineMessage(result.ResultMsg),
		AttemptsRemaining: remaining,
	}
}

// recordApproval applies an approval after checking it is for this payment.
// An approval that does not match (another shop, another amount) is held and
// alerted like an unanswered one: money moved, and not as we asked.
func (s *Service) recordApproval(ctx context.Context, payment *domain.Payment, result *ports.CardApprovalResult) (*domain.Payment, error) {
	asNano := NanoResult{
		ResultCode:  result.ResultCode,
		ResultMsg:   result.ResultMsg,
		ShopCode:    result.ShopCode,
		CompOrderNo: payment.ID,
		ReqPayAmt:   result.ReqPayAmt,
		TranNo:      result.TranNo,
		Source:      NanoSourceKeyIn,
	}
	reason := NanoRejectReason("")
	if want := s.nanoShopCode; want != "" && result.ShopCode != "" && result.ShopCode != want {
		reason = NanoRejectShopMismatch
	}
	if result.ReqPayAmt != "" && result.ReqPayAmt != fmt.Sprintf("%d", payment.AmountWon) {
		reason = NanoRejectAmountMismatch
	}
	if reason != "" {
		s.markHeld(ctx, payment.ID)
		return nil, s.nanoReject(ctx, asNano, reason, payment, domain.ErrCardApprovalHeld)
	}

	payment.CardApproved(result.TranNo, s.now())
	if err := s.persistSucceeded(ctx, payment); err != nil {
		s.markHeld(ctx, payment.ID)
		return nil, s.nanoReject(ctx, asNano, NanoRejectPersistFailed, payment, err)
	}
	log.Printf("payment: key-in %s approved: tran=%s card=%s %s",
		payment.ID, result.TranNo, result.CardName, result.CardMasked)
	return payment, nil
}

// holdUnconfirmed keeps the payment from taking another card and tells ops.
func (s *Service) holdUnconfirmed(ctx context.Context, payment *domain.Payment, cause error) error {
	s.markHeld(ctx, payment.ID)
	log.Printf("payment: key-in %s unconfirmed: %v", payment.ID, cause)
	return s.nanoReject(ctx, NanoResult{
		// The PG may have approved; nanoReject alerts only for an approved
		// result, so this is reported as one. The alert says it is unknown.
		ResultCode:  nanoApprovedCode,
		CompOrderNo: payment.ID,
		ReqPayAmt:   fmt.Sprintf("%d", payment.AmountWon),
		Source:      NanoSourceKeyIn,
	}, KeyInRejectUnconfirmed, payment, domain.ErrCardApprovalHeld)
}

func (s *Service) markHeld(ctx context.Context, paymentID string) {
	if _, err := s.repo.MutateLocked(ctx, paymentID, func(p *domain.Payment) ([]ports.OutboxEvent, error) {
		p.CardUnconfirmed(s.now())
		return nil, nil
	}); err != nil {
		// The in-flight marker is still set, so the payment reads as held
		// once CardApprovalInFlightWindow passes either way.
		log.Printf("payment: key-in %s mark held: %v", paymentID, err)
	}
}

// shopperDeclineMessage keeps the PG's reason (it is written for card
// holders: 비밀번호 오류, 한도 초과, …) but bounds it.
func shopperDeclineMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "카드사에서 결제를 승인하지 않았습니다."
	}
	if r := []rune(msg); len(r) > 120 {
		msg = string(r[:120])
	}
	return msg
}
