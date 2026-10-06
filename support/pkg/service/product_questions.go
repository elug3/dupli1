package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// Product question errors. The handler maps each to a status.
var (
	// ErrInvalidQuestion: unknown type, empty or overlong body, implausible
	// fit, or fit on a question that is not about size.
	ErrInvalidQuestion = errors.New("question needs a known type and 1 to 1000 characters")
	// ErrInvalidAnswer: empty or overlong answer.
	ErrInvalidAnswer = errors.New("answer needs 1 to 2000 characters")
	// ErrQuestionNotFound: no such question, or one the caller may not touch.
	// One error for both, so question ids cannot be probed.
	ErrQuestionNotFound = errors.New("question not found")
	// ErrQuestionLocked: the question has been answered and can no longer be
	// changed or withdrawn by its author.
	ErrQuestionLocked = errors.New("an answered question can no longer be changed")
)

// QuestionPageSize is how many questions one page of a product's list holds.
const QuestionPageSize = 20

// QuestionChannel is the alert channel product questions announce on, so the
// ops alert can say it is a product page question, not a consultation.
const QuestionChannel = "product_question"

// ProductQuestions is 상품 문의: shoppers ask about a product from its page,
// staff answer in the console, and the answer is published under the question
// unless the shopper asked privately.
type ProductQuestions struct {
	repo       ports.ProductQuestionRepository
	products   ports.ProductReader
	publisher  ports.InquiryPublisher
	notifier   ports.ShopperNotifier
	storefront string
	newID      IDGenerator
	now        Clock
	limiter    *sendLimiter
}

// ProductQuestionsDeps are ProductQuestions' collaborators. Publisher and
// Notifier are optional; Products is required to ask (a question names the
// variant the shopper had selected, which is checked against the catalog).
type ProductQuestionsDeps struct {
	Repo      ports.ProductQuestionRepository
	Products  ports.ProductReader
	Publisher ports.InquiryPublisher
	Notifier  ports.ShopperNotifier
	// StorefrontURL is where the answer email links (the product page).
	StorefrontURL string
	NewID         IDGenerator
	Now           Clock
}

func NewProductQuestions(deps ProductQuestionsDeps) *ProductQuestions {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	newID := deps.NewID
	if newID == nil {
		newID = func() string { return "" }
	}
	return &ProductQuestions{
		repo:       deps.Repo,
		products:   deps.Products,
		publisher:  deps.Publisher,
		notifier:   deps.Notifier,
		storefront: strings.TrimRight(strings.TrimSpace(deps.StorefrontURL), "/"),
		newID:      newID,
		now:        now,
		limiter:    &sendLimiter{perMin: 5, perDay: 30, windows: map[string]*sendWindow{}},
	}
}

// AskInput is a new question, or the new text of one being edited.
type AskInput struct {
	ProductID string
	SkuID     string
	Type      string
	Body      string
	Secret    bool
	Fit       *domain.Fit
}

func (in AskInput) normalized() (AskInput, error) {
	in.ProductID = strings.TrimSpace(in.ProductID)
	in.SkuID = strings.TrimSpace(in.SkuID)
	in.Body = strings.TrimSpace(in.Body)
	if !domain.ValidQuestionType(in.Type) || !domain.ValidQuestionBody(in.Body) || !in.Fit.Valid() {
		return in, ErrInvalidQuestion
	}
	if in.Fit.IsZero() {
		in.Fit = nil
	} else if in.Type != domain.QuestionSize {
		return in, ErrInvalidQuestion
	} else {
		fit := *in.Fit
		fit.UsualSize = strings.TrimSpace(fit.UsualSize)
		in.Fit = &fit
	}
	return in, nil
}

// Ask records a question about the variant the shopper had selected.
func (s *ProductQuestions) Ask(ctx context.Context, customer Customer, in AskInput) (*domain.ProductQuestion, error) {
	if strings.TrimSpace(customer.ID) == "" {
		return nil, ErrCustomerRequired
	}
	in, err := in.normalized()
	if err != nil {
		return nil, err
	}
	if in.ProductID == "" || in.SkuID == "" {
		return nil, ErrInvalidReference
	}
	ref, err := s.variant(ctx, in.SkuID)
	if err != nil {
		return nil, err
	}
	if ref.ProductID != in.ProductID {
		return nil, ErrInvalidReference
	}
	now := s.now()
	if !s.limiter.allow(customer.ID, now) {
		return nil, ErrRateLimited
	}
	question := &domain.ProductQuestion{
		ID:            s.newID(),
		ProductID:     ref.ProductID,
		SkuID:         ref.SkuID,
		VariantLabel:  domain.VariantLabel(ref.Color, ref.SKU),
		ProductName:   ref.Name,
		CustomerID:    customer.ID,
		CustomerEmail: customer.Email,
		AuthorMask:    domain.MaskAuthor(customer.Email),
		Type:          in.Type,
		Body:          in.Body,
		Fit:           in.Fit,
		Secret:        in.Secret,
		Status:        domain.QuestionWaiting,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.Save(ctx, question); err != nil {
		return nil, err
	}
	s.announce(ctx, question)
	return question, nil
}

func (s *ProductQuestions) variant(ctx context.Context, skuID string) (*ports.ProductRef, error) {
	if s.products == nil {
		return nil, ErrReferencesUnavailable
	}
	ref, err := s.products.Variant(ctx, skuID)
	if errors.Is(err, ports.ErrReferenceNotFound) {
		return nil, ErrInvalidReference
	}
	if err != nil || ref == nil {
		return nil, ErrReferencesUnavailable
	}
	return ref, nil
}

// announce tells staff through the same alert path consultations use. A lost
// alert does not fail the question: it is saved and waits in the console.
func (s *ProductQuestions) announce(ctx context.Context, q *domain.ProductQuestion) {
	if s.publisher == nil {
		return
	}
	excerpt := q.Body
	if utf8.RuneCountInString(excerpt) > 200 {
		excerpt = string([]rune(excerpt)[:200]) + "…"
	}
	err := s.publisher.InquiryOpened(ctx, ports.InquiryOpened{
		InquiryID:    q.ID,
		Topic:        domain.NodeProduct,
		Language:     domain.DefaultLanguage,
		EntryContext: strings.TrimSpace(q.ProductName + " " + q.VariantLabel),
		Excerpt:      excerpt,
		Channel:      QuestionChannel,
		ManagePath:   "/support?tab=questions&question=" + url.QueryEscape(q.ID),
	})
	if err != nil {
		log.Printf("announce product question %s: %v", q.ID, err)
	}
}

// QuestionView is one question as a given viewer may see it.
type QuestionView struct {
	domain.ProductQuestion
	// Redacted: a secret question someone else asked. Its words, fit and
	// answer are blanked; the row still shows, so counts add up.
	Redacted bool
	// Mine: the viewer asked it.
	Mine bool
}

func viewOf(q domain.ProductQuestion, viewerID string, staff bool) QuestionView {
	view := QuestionView{ProductQuestion: q, Mine: viewerID != "" && q.CustomerID == viewerID}
	if !q.ReadableBy(viewerID, staff) {
		view.Redacted = true
		view.Body = ""
		view.Fit = nil
		view.Answer = ""
		view.VariantLabel = ""
	}
	// Only staff learn who asked, beyond the masked label.
	if !staff {
		view.CustomerID = ""
		view.CustomerEmail = ""
		view.AnsweredBy = ""
		view.HiddenBy = ""
	}
	return view
}

// ListQuery narrows a product's public list.
type ListQuery struct {
	Type         string
	AnsweredOnly bool
	MineOnly     bool
	Page         int
}

// QuestionPage is one page of a product's questions, with the per-type counts
// the filter chips show (of everything visible, before filtering).
type QuestionPage struct {
	Items  []QuestionView
	Total  int
	Counts map[string]int
	Page   int
	More   bool
}

// List returns a product's questions for the product page. Anyone may read;
// viewerID is the signed-in shopper, or empty.
func (s *ProductQuestions) List(ctx context.Context, productID, viewerID string, query ListQuery) (*QuestionPage, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil, ErrQuestionNotFound
	}
	rows, err := s.repo.ListByProduct(ctx, productID, false)
	if err != nil {
		return nil, err
	}
	page := &QuestionPage{Counts: map[string]int{}, Page: max(query.Page, 1)}
	var matched []domain.ProductQuestion
	for _, q := range rows {
		page.Counts[q.Type]++
		page.Total++
		if query.Type != "" && q.Type != query.Type {
			continue
		}
		if query.AnsweredOnly && !q.IsAnswered() {
			continue
		}
		if query.MineOnly && (viewerID == "" || q.CustomerID != viewerID) {
			continue
		}
		matched = append(matched, q)
	}
	start := (page.Page - 1) * QuestionPageSize
	end := min(start+QuestionPageSize, len(matched))
	if start < len(matched) {
		for _, q := range matched[start:end] {
			page.Items = append(page.Items, viewOf(q, viewerID, false))
		}
	}
	page.More = end < len(matched)
	return page, nil
}

// Mine returns everything the shopper asked, for 마이페이지 → 상품 문의.
func (s *ProductQuestions) Mine(ctx context.Context, customer Customer) ([]QuestionView, error) {
	if strings.TrimSpace(customer.ID) == "" {
		return nil, ErrCustomerRequired
	}
	rows, err := s.repo.ListByCustomer(ctx, customer.ID)
	if err != nil {
		return nil, err
	}
	out := make([]QuestionView, 0, len(rows))
	for _, q := range rows {
		out = append(out, viewOf(q, customer.ID, false))
	}
	return out, nil
}

// Edit changes the shopper's own unanswered question. The product and variant
// stay as asked.
func (s *ProductQuestions) Edit(ctx context.Context, customer Customer, id string, in AskInput) (*QuestionView, error) {
	q, err := s.own(ctx, customer, id)
	if err != nil {
		return nil, err
	}
	in, err = in.normalized()
	if err != nil {
		return nil, err
	}
	q.Type, q.Body, q.Fit, q.Secret = in.Type, in.Body, in.Fit, in.Secret
	q.UpdatedAt = s.now()
	if err := s.repo.Save(ctx, q); err != nil {
		return nil, err
	}
	view := viewOf(*q, customer.ID, false)
	return &view, nil
}

// Withdraw deletes the shopper's own unanswered question.
func (s *ProductQuestions) Withdraw(ctx context.Context, customer Customer, id string) error {
	q, err := s.own(ctx, customer, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, q.ID)
}

func (s *ProductQuestions) own(ctx context.Context, customer Customer, id string) (*domain.ProductQuestion, error) {
	if strings.TrimSpace(customer.ID) == "" {
		return nil, ErrCustomerRequired
	}
	q, err := s.repo.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if q == nil || q.CustomerID != customer.ID {
		return nil, ErrQuestionNotFound
	}
	if !q.EditableBy(customer.ID) {
		return nil, ErrQuestionLocked
	}
	return q, nil
}

// Queue is the console's list.
func (s *ProductQuestions) Queue(ctx context.Context, filter ports.ProductQuestionFilter) ([]QuestionView, error) {
	rows, err := s.repo.ListForStaff(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]QuestionView, 0, len(rows))
	for _, q := range rows {
		out = append(out, viewOf(q, "", true))
	}
	return out, nil
}

// Get returns one question for staff.
func (s *ProductQuestions) Get(ctx context.Context, id string) (*QuestionView, error) {
	q, err := s.repo.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}
	view := viewOf(*q, "", true)
	return &view, nil
}

// Answer records staff's answer, publishes it under the question, and emails
// the shopper. Answering again replaces the answer (a correction) and sends
// no second email.
func (s *ProductQuestions) Answer(ctx context.Context, managerID, id, body string) (*QuestionView, error) {
	body = strings.TrimSpace(body)
	if !domain.ValidAnswerBody(body) {
		return nil, ErrInvalidAnswer
	}
	q, err := s.repo.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}
	first := !q.IsAnswered()
	now := s.now()
	q.Answer, q.AnsweredBy, q.AnsweredAt = body, managerID, &now
	q.Status = domain.QuestionAnswered
	q.UpdatedAt = now
	if err := s.repo.Save(ctx, q); err != nil {
		return nil, err
	}
	if first {
		s.notifyAnswer(ctx, q)
	}
	view := viewOf(*q, "", true)
	return &view, nil
}

// notifyAnswer emails the shopper that their question was answered. Like the
// consultation notice it carries the product name and a link, never the text.
func (s *ProductQuestions) notifyAnswer(ctx context.Context, q *domain.ProductQuestion) {
	if s.notifier == nil || s.storefront == "" || strings.TrimSpace(q.CustomerEmail) == "" {
		return
	}
	link := s.storefront + "/product/" + url.PathEscape(q.ProductID) + "?qna=" + url.QueryEscape(q.ID)
	if err := s.notifier.NotifyReply(ctx, q.CustomerEmail, q.ProductName, link); err != nil {
		log.Printf("answer notice for product question %s: %v", q.ID, err)
	}
}

// SetHidden hides a question from the product page (spam, abuse, personal
// details), or shows it again.
func (s *ProductQuestions) SetHidden(ctx context.Context, managerID, id string, hidden bool) (*QuestionView, error) {
	q, err := s.repo.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}
	now := s.now()
	q.Hidden = hidden
	if hidden {
		q.HiddenBy, q.HiddenAt = managerID, &now
	} else {
		q.HiddenBy, q.HiddenAt = "", nil
	}
	q.UpdatedAt = now
	if err := s.repo.Save(ctx, q); err != nil {
		return nil, err
	}
	view := viewOf(*q, "", true)
	return &view, nil
}

// ForgetCustomer removes a deleted account's words from every question it
// asked. Answers stay, so the product page keeps what other shoppers read.
func (s *ProductQuestions) ForgetCustomer(ctx context.Context, customerID string) error {
	if strings.TrimSpace(customerID) == "" {
		return nil
	}
	rows, err := s.repo.ListByCustomer(ctx, customerID)
	if err != nil {
		return fmt.Errorf("list questions to forget: %w", err)
	}
	now := s.now()
	var errs []error
	for i := range rows {
		rows[i].Forget(now)
		if err := s.repo.Save(ctx, &rows[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
