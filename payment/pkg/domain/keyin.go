package domain

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Key-in card payment (NANO 수기결제): the shopper types the card on the
// storefront and payment sends it to the PG once. These are the rules that
// decide whether a typed card is worth sending, and how many times a payment
// may try. See docs/payment-nano-key-in.md.

var (
	// ErrInvalidCard is a typed card that is not worth sending to the PG
	// (wrong length, failed checksum, expired). It never counts as an attempt.
	ErrInvalidCard = errors.New("invalid card details")
	// ErrCardDeclined is the PG's refusal of a card.
	ErrCardDeclined = errors.New("card declined")
	// ErrCardAttemptsExhausted means the payment has used its attempts and is
	// failed; the shopper starts a new payment from the order.
	ErrCardAttemptsExhausted = errors.New("too many card attempts for this payment")
	// ErrCardApprovalInProgress means another approval for this payment is
	// still waiting on the PG (a double submit).
	ErrCardApprovalInProgress = errors.New("card approval already in progress")
	// ErrCardApprovalHeld means an earlier approval for this payment got no
	// answer from the PG, so it may have been charged. The payment takes no
	// more attempts until a person checks the PG console.
	ErrCardApprovalHeld = errors.New("card approval outcome unknown; payment held for review")
)

// MaxCardAttempts is how many declined cards one payment may try before it
// fails. It bounds card testing (trying stolen numbers against our checkout)
// while leaving room for a mistyped PIN or birth date.
const MaxCardAttempts = 5

// CardApprovalInFlightWindow is how long a started approval counts as still
// running. It is longer than the PG call's own timeout, so a marker older than
// this means the answer was lost (or the process died mid-call).
const CardApprovalInFlightWindow = 45 * time.Second

// MinInstallmentWon is the smallest amount Korean card issuers split into
// monthly installments (5만원); below it a payment is lump sum only.
const MinInstallmentWon = 50_000

// MaxInstallmentMonths is the longest installment the storefront offers.
const MaxInstallmentMonths = 12

// CardDetails is what the shopper typed. It is never stored, logged or
// returned; it exists for the length of one approval call.
type CardDetails struct {
	Number      string // digits only
	ExpYY       string // two digits
	ExpMM       string // two digits, 01–12
	Password2   string // first two digits of the card PIN
	HolderID    string // birth date YYMMDD (6) or business number (10)
	Installment string // "00" lump sum, or "02"–"12"
}

// CardFieldError names the field that failed, so the storefront can point at
// it. It never carries the value.
type CardFieldError struct {
	Field string
}

func (e *CardFieldError) Error() string { return ErrInvalidCard.Error() + ": " + e.Field }

func (e *CardFieldError) Is(target error) bool { return target == ErrInvalidCard }

func cardField(field string) error { return &CardFieldError{Field: field} }

// NormalizeCard strips spaces and dashes the shopper may have typed and
// zero-pads a one-digit month or installment, then validates. now is used
// for the expiry check, in KST.
func NormalizeCard(c CardDetails, amountWon int64, now time.Time) (CardDetails, error) {
	c.Number = digitsOnly(c.Number)
	c.ExpYY = strings.TrimSpace(c.ExpYY)
	c.ExpMM = strings.TrimSpace(c.ExpMM)
	c.Password2 = strings.TrimSpace(c.Password2)
	c.HolderID = digitsOnly(c.HolderID)
	c.Installment = strings.TrimSpace(c.Installment)
	if len(c.ExpMM) == 1 {
		c.ExpMM = "0" + c.ExpMM
	}
	if c.Installment == "" || c.Installment == "0" || c.Installment == "1" || c.Installment == "01" {
		c.Installment = "00"
	}
	if len(c.Installment) == 1 {
		c.Installment = "0" + c.Installment
	}

	if n := len(c.Number); n < 14 || n > 19 || !luhnValid(c.Number) {
		return CardDetails{}, cardField("card_number")
	}
	mm, err := strconv.Atoi(c.ExpMM)
	if len(c.ExpMM) != 2 || err != nil || mm < 1 || mm > 12 || !allDigits(c.ExpMM) {
		return CardDetails{}, cardField("exp_month")
	}
	yy, err := strconv.Atoi(c.ExpYY)
	if len(c.ExpYY) != 2 || err != nil || !allDigits(c.ExpYY) {
		return CardDetails{}, cardField("exp_year")
	}
	kst := now.In(kstZone)
	// A card is good through the last day of its expiry month.
	if 2000+yy < kst.Year() || (2000+yy == kst.Year() && mm < int(kst.Month())) {
		return CardDetails{}, cardField("exp_year")
	}
	if len(c.Password2) != 2 || !allDigits(c.Password2) {
		return CardDetails{}, cardField("card_password")
	}
	if !validHolderID(c.HolderID) {
		return CardDetails{}, cardField("holder_id")
	}
	if c.Installment != "00" {
		months, err := strconv.Atoi(c.Installment)
		if len(c.Installment) != 2 || err != nil || months < 2 || months > MaxInstallmentMonths || amountWon < MinInstallmentWon {
			return CardDetails{}, cardField("installment")
		}
	}
	return c, nil
}

// validHolderID accepts a six-digit birth date (YYMMDD, a real calendar day)
// or a ten-digit business registration number for a corporate card.
func validHolderID(s string) bool {
	switch len(s) {
	case 10:
		return true
	case 6:
		mm, _ := strconv.Atoi(s[2:4])
		dd, _ := strconv.Atoi(s[4:6])
		if mm < 1 || mm > 12 || dd < 1 {
			return false
		}
		// Year 2000 is a leap year, so 29 Feb is accepted for any YY.
		days := time.Date(2000, time.Month(mm)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		return dd <= days
	default:
		return false
	}
}

func luhnValid(number string) bool {
	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		d := int(number[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
		default:
			// Anything else makes the value invalid rather than silently
			// dropped; keep it so the length/checksum check fails.
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

var kstZone = time.FixedZone("KST", 9*60*60)

// BeginCardAttempt claims the payment for one approval call. It refuses a
// payment that is not awaiting payment, is out of attempts, already has a
// call running, or is held after an unanswered call.
func (p *Payment) BeginCardAttempt(now time.Time) error {
	if p.Status != StatusRequiresPayment {
		if p.Status == StatusFailed && p.CardAttempts >= MaxCardAttempts {
			return ErrCardAttemptsExhausted
		}
		return ErrInvalidPayment
	}
	if p.CardApprovalUnconfirmed {
		return ErrCardApprovalHeld
	}
	if p.CardApprovalStartedAt != nil {
		if now.Sub(*p.CardApprovalStartedAt) < CardApprovalInFlightWindow {
			return ErrCardApprovalInProgress
		}
		return ErrCardApprovalHeld
	}
	if p.CardAttempts >= MaxCardAttempts {
		return ErrCardAttemptsExhausted
	}
	p.CardAttempts++
	started := now
	p.CardApprovalStartedAt = &started
	p.UpdatedAt = now
	return nil
}

// CardDeclined records a decline. The payment stays open for another card
// until it runs out of attempts, then fails.
func (p *Payment) CardDeclined(now time.Time) {
	p.CardApprovalStartedAt = nil
	if p.CardAttempts >= MaxCardAttempts {
		p.Status = StatusFailed
	}
	p.UpdatedAt = now
}

// CardNotSent releases the claim when the call never reached the PG (it could
// not be built), and gives the attempt back.
func (p *Payment) CardNotSent(now time.Time) {
	p.CardApprovalStartedAt = nil
	if p.CardAttempts > 0 {
		p.CardAttempts--
	}
	p.UpdatedAt = now
}

// CardUnconfirmed records a call the PG never answered: the card may have
// been charged, so the payment is held until a person checks the PG console.
func (p *Payment) CardUnconfirmed(now time.Time) {
	p.CardApprovalUnconfirmed = true
	p.UpdatedAt = now
}

// CardApproved records the PG's approval: its transaction number becomes the
// provider reference refunds are sent with.
func (p *Payment) CardApproved(tranNo string, now time.Time) {
	p.CardApprovalStartedAt = nil
	p.CardApprovalUnconfirmed = false
	if t := strings.TrimSpace(tranNo); t != "" {
		p.ProviderRef = t
	}
	if p.Status != StatusSucceeded {
		p.MarkSucceeded(now)
	}
}

// CardApprovalHeld reports whether an approval call for this payment went
// unanswered long enough ago that it must be checked by hand.
func (p *Payment) CardApprovalHeld(now time.Time) bool {
	if p.Status != StatusRequiresPayment {
		return false
	}
	return p.CardApprovalUnconfirmed || (p.CardApprovalStartedAt != nil &&
		now.Sub(*p.CardApprovalStartedAt) >= CardApprovalInFlightWindow)
}
