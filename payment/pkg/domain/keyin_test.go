package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var keyInNow = time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC) // 12:00 KST

func goodCard() CardDetails {
	return CardDetails{Number: "4111 1111-1111 1111", ExpYY: "30", ExpMM: "7", Password2: "12", HolderID: "900101"}
}

func TestNormalizeCard_Accepts(t *testing.T) {
	got, err := NormalizeCard(goodCard(), 10_000, keyInNow)
	if err != nil {
		t.Fatal(err)
	}
	want := CardDetails{Number: "4111111111111111", ExpYY: "30", ExpMM: "07", Password2: "12", HolderID: "900101", Installment: "00"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestNormalizeCard_Refuses(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CardDetails)
		amount int64
		field  string
	}{
		{"checksum", func(c *CardDetails) { c.Number = "4111111111111112" }, 10_000, "card_number"},
		{"too short", func(c *CardDetails) { c.Number = "4111111" }, 10_000, "card_number"},
		{"letters", func(c *CardDetails) { c.Number = "4111a11111111111" }, 10_000, "card_number"},
		{"month 13", func(c *CardDetails) { c.ExpMM = "13" }, 10_000, "exp_month"},
		{"expired last month", func(c *CardDetails) { c.ExpYY, c.ExpMM = "26", "10" }, 10_000, "exp_year"},
		{"pin 3 digits", func(c *CardDetails) { c.Password2 = "123" }, 10_000, "card_password"},
		{"birth 31 Feb", func(c *CardDetails) { c.HolderID = "900231" }, 10_000, "holder_id"},
		{"holder 8 digits", func(c *CardDetails) { c.HolderID = "19900101" }, 10_000, "holder_id"},
		{"installment under 5만원", func(c *CardDetails) { c.Installment = "03" }, 49_999, "installment"},
		{"installment 24 months", func(c *CardDetails) { c.Installment = "24" }, 100_000, "installment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := goodCard()
			tc.mutate(&c)
			_, err := NormalizeCard(c, tc.amount, keyInNow)
			var fe *CardFieldError
			if !errors.As(err, &fe) || fe.Field != tc.field || !errors.Is(err, ErrInvalidCard) {
				t.Fatalf("err = %v, want field %s", err, tc.field)
			}
		})
	}
}

func TestNormalizeCard_EdgesThatPass(t *testing.T) {
	c := goodCard()
	c.ExpYY, c.ExpMM = "26", "11" // good through the end of this month
	c.HolderID = "1234567890"     // corporate card: business number
	c.Installment = "3"
	got, err := NormalizeCard(c, 50_000, keyInNow)
	if err != nil {
		t.Fatal(err)
	}
	if got.Installment != "03" {
		t.Fatalf("installment = %q", got.Installment)
	}
}

func TestCardFieldError_NeverCarriesTheValue(t *testing.T) {
	c := goodCard()
	c.Number = "4111111111111112"
	_, err := NormalizeCard(c, 1, keyInNow)
	if err == nil || strings.Contains(err.Error(), "4111") {
		t.Fatalf("err = %v", err)
	}
}

func openPayment() *Payment {
	return &Payment{ID: "pay_1", Status: StatusRequiresPayment, Provider: ProviderNano, AmountWon: 1000}
}

func TestCardAttempts_DeclinesUntilFailed(t *testing.T) {
	p := openPayment()
	for i := 1; i <= MaxCardAttempts; i++ {
		if err := p.BeginCardAttempt(keyInNow); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		p.CardDeclined(keyInNow)
	}
	if p.Status != StatusFailed || p.CardAttempts != MaxCardAttempts {
		t.Fatalf("after %d declines: status %s attempts %d", MaxCardAttempts, p.Status, p.CardAttempts)
	}
	if err := p.BeginCardAttempt(keyInNow); !errors.Is(err, ErrCardAttemptsExhausted) {
		t.Fatalf("err = %v", err)
	}
}

func TestCardAttempts_OneAtATime(t *testing.T) {
	p := openPayment()
	if err := p.BeginCardAttempt(keyInNow); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginCardAttempt(keyInNow.Add(time.Second)); !errors.Is(err, ErrCardApprovalInProgress) {
		t.Fatalf("double submit: %v", err)
	}
	// A marker older than the window is a call whose answer was lost.
	if err := p.BeginCardAttempt(keyInNow.Add(CardApprovalInFlightWindow)); !errors.Is(err, ErrCardApprovalHeld) {
		t.Fatalf("stale marker: %v", err)
	}
	if !p.CardApprovalHeld(keyInNow.Add(CardApprovalInFlightWindow)) {
		t.Fatal("stale marker reads as held")
	}
}

func TestCardAttempts_UnconfirmedHolds(t *testing.T) {
	p := openPayment()
	_ = p.BeginCardAttempt(keyInNow)
	p.CardUnconfirmed(keyInNow)
	if err := p.BeginCardAttempt(keyInNow.Add(time.Second)); !errors.Is(err, ErrCardApprovalHeld) {
		t.Fatalf("err = %v", err)
	}
}

func TestCardAttempts_NotSentGivesTheAttemptBack(t *testing.T) {
	p := openPayment()
	_ = p.BeginCardAttempt(keyInNow)
	p.CardNotSent(keyInNow)
	if p.CardAttempts != 0 || p.CardApprovalStartedAt != nil {
		t.Fatalf("attempts %d marker %v", p.CardAttempts, p.CardApprovalStartedAt)
	}
}

func TestCardApproved(t *testing.T) {
	p := openPayment()
	p.ProviderRef = "nano_pay_1"
	_ = p.BeginCardAttempt(keyInNow)
	p.CardApproved("2409030071109", keyInNow)
	if p.Status != StatusSucceeded || p.ProviderRef != "2409030071109" || p.CardApprovalStartedAt != nil {
		t.Fatalf("payment = %+v", p)
	}
}
