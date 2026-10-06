package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Product question types (상품 문의 유형). Order and delivery questions are not
// among them on purpose: those belong to the 1:1 consultation, where staff can
// see the order, not on a product page anyone can read.
const (
	QuestionSize    = "size"
	QuestionStock   = "stock"
	QuestionProduct = "product"
	QuestionOther   = "other"
)

// Product question states. Hidden is a separate flag, not a status: a hidden
// question keeps whether it was answered.
const (
	QuestionWaiting  = "waiting"
	QuestionAnswered = "answered"
)

// Bounds on what a shopper and a manager may write.
const (
	MaxQuestionRunes = 1000
	MaxAnswerRunes   = 2000
	// maxUsualSizeRunes bounds the free-text "평소 사이즈" (L, 95, 100…).
	maxUsualSizeRunes = 10
)

// ForgottenQuestionBody replaces what a deleted account wrote. Not
// PurgedBody: that says the retention window closed, and staff should be able
// to tell the two apart.
const ForgottenQuestionBody = "(탈퇴한 회원의 문의입니다)"

// Fit is the optional body information on a size question: what staff need
// to recommend a size, and nothing more.
type Fit struct {
	HeightCm  int    `json:"height_cm,omitempty"`
	WeightKg  int    `json:"weight_kg,omitempty"`
	UsualSize string `json:"usual_size,omitempty"`
}

// IsZero reports whether no fit field was given.
func (f *Fit) IsZero() bool {
	return f == nil || (f.HeightCm == 0 && f.WeightKg == 0 && strings.TrimSpace(f.UsualSize) == "")
}

// Valid reports whether each given field is plausible. Zero means not given.
func (f *Fit) Valid() bool {
	if f == nil {
		return true
	}
	if f.HeightCm != 0 && (f.HeightCm < 100 || f.HeightCm > 230) {
		return false
	}
	if f.WeightKg != 0 && (f.WeightKg < 30 || f.WeightKg > 200) {
		return false
	}
	return utf8.RuneCountInString(strings.TrimSpace(f.UsualSize)) <= maxUsualSizeRunes
}

// ProductQuestion is one question a shopper asked about a product, and the
// answer staff gave. It is private: only the shopper who asked and staff ever
// see it. Unlike a consultation it is a single question and answer, filed
// under the product and variant it is about.
type ProductQuestion struct {
	ID        string
	ProductID string
	// SkuID is the variant the shopper had selected; VariantLabel is how it
	// read when they asked ("Black / M"), kept so a renamed color does not
	// rewrite the question.
	SkuID        string
	VariantLabel string
	// ProductName is the product's name when asked, for the inbox and the
	// answer email.
	ProductName string

	CustomerID    string
	CustomerEmail string

	Type string
	Body string
	Fit  *Fit

	Status     string
	Answer     string
	AnsweredBy string
	AnsweredAt *time.Time

	Hidden   bool
	HiddenBy string
	HiddenAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ValidQuestionType reports whether t is one of the four question types.
func ValidQuestionType(t string) bool {
	switch t {
	case QuestionSize, QuestionStock, QuestionProduct, QuestionOther:
		return true
	}
	return false
}

// ValidQuestionBody reports whether a question has text, within bounds.
func ValidQuestionBody(body string) bool {
	trimmed := strings.TrimSpace(body)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= MaxQuestionRunes
}

// ValidAnswerBody reports whether an answer has text, within bounds.
func ValidAnswerBody(body string) bool {
	trimmed := strings.TrimSpace(body)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= MaxAnswerRunes
}

// VariantLabel reads "Color / Size" for a variant. The size is the last
// segment of the human SKU (Brand_Style_Color[_Edition]_Size).
func VariantLabel(color, sku string) string {
	size := ""
	if i := strings.LastIndex(sku, "_"); i >= 0 && i < len(sku)-1 {
		size = sku[i+1:]
	}
	color = strings.TrimSpace(color)
	switch {
	case color != "" && size != "":
		return color + " / " + size
	case color != "":
		return color
	default:
		return size
	}
}

// IsAnswered reports whether staff has answered.
func (q *ProductQuestion) IsAnswered() bool {
	return q != nil && q.Status == QuestionAnswered
}

// EditableBy reports whether a shopper may still change or delete the
// question: their own, and not answered yet. Once answered it is locked, so
// an answer never ends up under a question it did not answer.
func (q *ProductQuestion) EditableBy(customerID string) bool {
	return q != nil && customerID != "" && q.CustomerID == customerID && !q.IsAnswered()
}

// Forget removes what a deleted account wrote. The row stays, as a
// consultation's does, so the record of what was asked and answered survives
// without the shopper's words, fit or address.
func (q *ProductQuestion) Forget(now time.Time) {
	q.Body = ForgottenQuestionBody
	q.Fit = nil
	q.CustomerEmail = ""
	q.UpdatedAt = now
}
