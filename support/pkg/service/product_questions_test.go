package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

type questionHarness struct {
	svc       *service.ProductQuestions
	repo      *memory.ProductQuestionRepository
	published *fakePublisher
	notifier  *fakeNotifier
	now       time.Time
}

func newQuestionHarness() *questionHarness {
	h := &questionHarness{
		repo:      memory.NewProductQuestionRepository(),
		published: &fakePublisher{},
		notifier:  &fakeNotifier{},
		now:       time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC),
	}
	n := 0
	h.svc = service.NewProductQuestions(service.ProductQuestionsDeps{
		Repo: h.repo,
		Products: &fakeProducts{variants: map[string]ports.ProductRef{
			"SKU-M":   {ProductID: "JACKET", SkuID: "SKU-M", SKU: "DUPLI1_PUFF_Black_M", Name: "숏 패딩 재킷", Color: "Black"},
			"SKU-BAG": {ProductID: "BAG", SkuID: "SKU-BAG", SKU: "DUPLI1_TOTE_Tan_OS", Name: "토트백", Color: "Tan"},
		}},
		Publisher:     h.published,
		Notifier:      h.notifier,
		StorefrontURL: "https://dupli1.com/",
		NewID: func() string {
			n++
			return fmt.Sprintf("Q%03d", n)
		},
		Now: func() time.Time {
			h.now = h.now.Add(time.Second)
			return h.now
		},
	})
	return h
}

var (
	asker = service.Customer{ID: "u1", Email: "s3jin@gmail.com"}
	other = service.Customer{ID: "u2", Email: "mk@naver.com"}
)

func sizeQuestion() service.AskInput {
	return service.AskInput{
		ProductID: "JACKET", SkuID: "SKU-M", Type: domain.QuestionSize,
		Body: "  평소 L 입는데 M이랑 고민이에요.  ",
		Fit:  &domain.Fit{HeightCm: 178, WeightKg: 70, UsualSize: " L "},
	}
}

func TestAskRecordsVariantAndAnnounces(t *testing.T) {
	h := newQuestionHarness()
	q, err := h.svc.Ask(context.Background(), asker, sizeQuestion())
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if q.Body != "평소 L 입는데 M이랑 고민이에요." || q.Fit.UsualSize != "L" {
		t.Fatalf("not trimmed: %+v %+v", q.Body, q.Fit)
	}
	if q.VariantLabel != "Black / M" || q.ProductName != "숏 패딩 재킷" || q.CustomerEmail != asker.Email {
		t.Fatalf("snapshot = %q %q %q", q.VariantLabel, q.ProductName, q.CustomerEmail)
	}
	if q.Status != domain.QuestionWaiting {
		t.Fatalf("status = %s", q.Status)
	}
	if len(h.published.opened) != 1 {
		t.Fatalf("announced %d times", len(h.published.opened))
	}
	alert := h.published.opened[0]
	if alert.Channel != service.QuestionChannel || !strings.Contains(alert.ManagePath, "question=Q001") {
		t.Fatalf("alert = %+v", alert)
	}
}

func TestAskRefusesBadInput(t *testing.T) {
	h := newQuestionHarness()
	cases := map[string]struct {
		in   func(service.AskInput) service.AskInput
		want error
	}{
		"unknown type":           {func(in service.AskInput) service.AskInput { in.Type = "delivery"; return in }, service.ErrInvalidQuestion},
		"empty body":             {func(in service.AskInput) service.AskInput { in.Body = "  "; return in }, service.ErrInvalidQuestion},
		"long body":              {func(in service.AskInput) service.AskInput { in.Body = strings.Repeat("가", 1001); return in }, service.ErrInvalidQuestion},
		"fit off size":           {func(in service.AskInput) service.AskInput { in.Type = domain.QuestionStock; return in }, service.ErrInvalidQuestion},
		"implausible height":     {func(in service.AskInput) service.AskInput { in.Fit = &domain.Fit{HeightCm: 20}; return in }, service.ErrInvalidQuestion},
		"no sku":                 {func(in service.AskInput) service.AskInput { in.SkuID = ""; return in }, service.ErrInvalidReference},
		"unknown sku":            {func(in service.AskInput) service.AskInput { in.SkuID = "NOPE"; return in }, service.ErrInvalidReference},
		"sku of another product": {func(in service.AskInput) service.AskInput { in.SkuID = "SKU-BAG"; return in }, service.ErrInvalidReference},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.Ask(context.Background(), asker, tc.in(sizeQuestion()))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := h.svc.Ask(context.Background(), service.Customer{}, sizeQuestion()); !errors.Is(err, service.ErrCustomerRequired) {
		t.Fatalf("anonymous ask: %v", err)
	}
}

func TestAskWithoutCatalogIsUnavailable(t *testing.T) {
	svc := service.NewProductQuestions(service.ProductQuestionsDeps{Repo: memory.NewProductQuestionRepository()})
	if _, err := svc.Ask(context.Background(), asker, sizeQuestion()); !errors.Is(err, service.ErrReferencesUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestAskIsRateLimited(t *testing.T) {
	h := newQuestionHarness()
	var err error
	for i := 0; i < 6 && err == nil; i++ {
		_, err = h.svc.Ask(context.Background(), asker, sizeQuestion())
	}
	if !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("sixth ask in a minute: %v", err)
	}
}

func TestMineShowsOnlyTheCallersQuestions(t *testing.T) {
	h := newQuestionHarness()
	ctx := context.Background()
	jacket, _ := h.svc.Ask(ctx, asker, sizeQuestion())
	bag := service.AskInput{ProductID: "BAG", SkuID: "SKU-BAG", Type: domain.QuestionProduct, Body: "스트랩 조절 되나요?"}
	if _, err := h.svc.Ask(ctx, asker, bag); err != nil {
		t.Fatal(err)
	}
	stock := service.AskInput{ProductID: "JACKET", SkuID: "SKU-M", Type: domain.QuestionStock, Body: "재입고 되나요?"}
	if _, err := h.svc.Ask(ctx, other, stock); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Answer(ctx, "mgr", jacket.ID, "M을 권해드려요."); err != nil {
		t.Fatal(err)
	}

	onJacket, err := h.svc.Mine(ctx, asker, "JACKET")
	if err != nil {
		t.Fatal(err)
	}
	if len(onJacket) != 1 || onJacket[0].ID != jacket.ID || onJacket[0].Answer != "M을 권해드려요." {
		t.Fatalf("asker's jacket questions = %+v", onJacket)
	}
	view := onJacket[0]
	if view.CustomerID != "" || view.CustomerEmail != "" || view.AnsweredBy != "" || view.Staff {
		t.Fatalf("staff fields reached the shopper: %+v", view)
	}
	all, _ := h.svc.Mine(ctx, asker, "")
	if len(all) != 2 || all[0].ProductID != "BAG" {
		t.Fatalf("all of asker's questions, newest first = %+v", all)
	}
	strangers, _ := h.svc.Mine(ctx, other, "JACKET")
	if len(strangers) != 1 || strangers[0].Type != domain.QuestionStock {
		t.Fatalf("other shopper sees = %+v", strangers)
	}
	if _, err := h.svc.Mine(ctx, service.Customer{}, "JACKET"); !errors.Is(err, service.ErrCustomerRequired) {
		t.Fatalf("anonymous: %v", err)
	}
}

func TestPurgeDropsOldQuestionTextButKeepsAnswers(t *testing.T) {
	h := newQuestionHarness()
	ctx := context.Background()
	old, _ := h.svc.Ask(ctx, asker, sizeQuestion())
	_, _ = h.svc.Answer(ctx, "mgr", old.ID, "M을 권해드려요.")
	h.now = h.now.Add(181 * 24 * time.Hour)
	recent, _ := h.svc.Ask(ctx, other, sizeQuestion())

	purged, err := h.svc.PurgeExpiredBodies(ctx, 180*24*time.Hour)
	if err != nil || purged != 1 {
		t.Fatalf("purged %d, %v", purged, err)
	}
	got, _ := h.repo.FindByID(ctx, old.ID)
	if got.Body != domain.PurgedBody || got.Fit != nil || got.Answer != "M을 권해드려요." {
		t.Fatalf("old = %+v", got)
	}
	if kept, _ := h.repo.FindByID(ctx, recent.ID); kept.Body == domain.PurgedBody {
		t.Fatal("recent question purged")
	}
	if again, _ := h.svc.PurgeExpiredBodies(ctx, 180*24*time.Hour); again != 0 {
		t.Fatalf("purged twice: %d", again)
	}
}

func TestEditAndWithdrawOnlyOwnUnanswered(t *testing.T) {
	h := newQuestionHarness()
	ctx := context.Background()
	q, _ := h.svc.Ask(ctx, asker, sizeQuestion())

	if _, err := h.svc.Edit(ctx, other, q.ID, sizeQuestion()); !errors.Is(err, service.ErrQuestionNotFound) {
		t.Fatalf("stranger edit: %v", err)
	}
	edit := service.AskInput{Type: domain.QuestionSize, Body: "XS도 괜찮을까요?"}
	view, err := h.svc.Edit(ctx, asker, q.ID, edit)
	if err != nil || view.Body != "XS도 괜찮을까요?" || view.Fit != nil || view.SkuID != "SKU-M" {
		t.Fatalf("edit = %+v, %v", view, err)
	}

	if _, err := h.svc.Answer(ctx, "mgr", q.ID, "네"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Edit(ctx, asker, q.ID, edit); !errors.Is(err, service.ErrQuestionLocked) {
		t.Fatalf("edit after answer: %v", err)
	}
	if err := h.svc.Withdraw(ctx, asker, q.ID); !errors.Is(err, service.ErrQuestionLocked) {
		t.Fatalf("withdraw after answer: %v", err)
	}

	q2, _ := h.svc.Ask(ctx, asker, sizeQuestion())
	if err := h.svc.Withdraw(ctx, other, q2.ID); !errors.Is(err, service.ErrQuestionNotFound) {
		t.Fatalf("stranger withdraw: %v", err)
	}
	if err := h.svc.Withdraw(ctx, asker, q2.ID); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if found, _ := h.repo.FindByID(ctx, q2.ID); found != nil {
		t.Fatal("withdrawn question still stored")
	}
}

func TestAnswerEmailsOnceAndCorrectionsDoNot(t *testing.T) {
	h := newQuestionHarness()
	ctx := context.Background()
	q, _ := h.svc.Ask(ctx, asker, sizeQuestion())

	if _, err := h.svc.Answer(ctx, "mgr", q.ID, " "); !errors.Is(err, service.ErrInvalidAnswer) {
		t.Fatalf("empty answer: %v", err)
	}
	view, err := h.svc.Answer(ctx, "mgr", q.ID, "M을 권해드려요.")
	if err != nil || view.Status != domain.QuestionAnswered || view.AnsweredBy != "mgr" || view.AnsweredAt == nil {
		t.Fatalf("answer = %+v, %v", view, err)
	}
	if len(h.notifier.sent) != 1 {
		t.Fatalf("emails = %d", len(h.notifier.sent))
	}
	sent := h.notifier.sent[0]
	if sent.to != asker.Email || sent.subject != "숏 패딩 재킷" || sent.link != "https://dupli1.com/product/JACKET?qna="+q.ID {
		t.Fatalf("notice = %+v", sent)
	}
	if strings.Contains(sent.subject+sent.link, "M을") {
		t.Fatal("answer text leaked into the email")
	}
	if _, err := h.svc.Answer(ctx, "mgr", q.ID, "L을 권해드려요."); err != nil {
		t.Fatal(err)
	}
	if len(h.notifier.sent) != 1 {
		t.Fatal("a correction sent a second email")
	}
	if _, err := h.svc.Answer(ctx, "mgr", "missing", "네"); !errors.Is(err, service.ErrQuestionNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestHiddenQuestionsLeaveTheWaitingQueue(t *testing.T) {
	h := newQuestionHarness()
	ctx := context.Background()
	q, _ := h.svc.Ask(ctx, asker, sizeQuestion())

	waiting, _ := h.svc.Queue(ctx, ports.ProductQuestionFilter{})
	if len(waiting) != 1 || waiting[0].CustomerEmail != asker.Email || !waiting[0].Staff {
		t.Fatalf("staff queue = %+v", waiting)
	}
	if _, err := h.svc.SetHidden(ctx, "mgr", q.ID, true); err != nil {
		t.Fatal(err)
	}
	hidden, _ := h.svc.Queue(ctx, ports.ProductQuestionFilter{Queue: ports.QuestionQueueHidden})
	if len(hidden) != 1 || hidden[0].HiddenBy != "mgr" {
		t.Fatalf("hidden queue = %+v", hidden)
	}
	if waiting, _ := h.svc.Queue(ctx, ports.ProductQuestionFilter{}); len(waiting) != 0 {
		t.Fatalf("hidden question still waiting: %+v", waiting)
	}
	mine, _ := h.svc.Mine(ctx, asker, "JACKET")
	if len(mine) != 1 || mine[0].Hidden {
		t.Fatalf("the asker still sees their question, without the moderation flag: %+v", mine)
	}
	if view, _ := h.svc.SetHidden(ctx, "mgr", q.ID, false); view.Hidden || view.HiddenAt != nil {
		t.Fatalf("unhide = %+v", view)
	}
}

func TestForgetCustomerKeepsTheAnswer(t *testing.T) {
	h := newQuestionHarness()
	ctx := context.Background()
	q, _ := h.svc.Ask(ctx, asker, sizeQuestion())
	_, _ = h.svc.Answer(ctx, "mgr", q.ID, "M을 권해드려요.")

	if err := h.svc.ForgetCustomer(ctx, asker.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := h.repo.FindByID(ctx, q.ID)
	if got.Body != domain.ForgottenQuestionBody || got.Fit != nil || got.CustomerEmail != "" {
		t.Fatalf("not forgotten: %+v", got)
	}
	if got.Answer != "M을 권해드려요." {
		t.Fatalf("answer lost: %q", got.Answer)
	}
}
