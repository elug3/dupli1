package domain

import (
	"strings"
	"time"
)

// Review rewards (docs/payment-points.md → Review rewards). A review earns
// once per order line, (order id, sku id), whatever happens to the review
// afterwards: deleting it and writing it again earns nothing more.
const (
	// ReviewRewardMinChars is the shortest review text that earns points.
	ReviewRewardMinChars = 20
	// ReviewTextPoints is what a review earns.
	ReviewTextPoints int64 = 2_000
	// ReviewPhotoPoints is what a review with at least one photo earns in all.
	ReviewPhotoPoints int64 = 5_000
)

// EntryReview is a review reward in the history.
const EntryReview = "review"

// Review entry notes, so the history can say which reward it was.
const (
	ReviewNoteText  = "text"
	ReviewNotePhoto = "photo"
)

// reviewLineRef names an order line's text reward lot. Its photo bonus lot
// adds ":photo". Order ids belong to one customer, so the ref is unique.
func reviewLineRef(orderID, skuID string) string {
	return orderID + ":" + skuID
}

func reviewPhotoRef(orderID, skuID string) string {
	return reviewLineRef(orderID, skuID) + ":photo"
}

// ReviewWritten rewards a review of one order line, or tops it up when an
// edit made it eligible for more: 2,000 P for text of at least 20 characters,
// 5,000 P in all once it has a photo. It returns the points it added.
//
// Nothing is granted when the line's reward was taken back (the review was
// hidden, or the order canceled), or when the order itself was canceled.
// Shortening the text or removing the photos later keeps what was earned.
func (a *PointAccount) ReviewWritten(orderID, skuID, reviewID string, bodyChars, photoCount int, at time.Time) int64 {
	if orderID == "" || skuID == "" || bodyChars < ReviewRewardMinChars {
		return 0
	}
	if l := a.lot(PointSourceOrder, orderID); l != nil && l.Status == LotRevoked {
		return 0
	}
	textRef, photoRef := reviewLineRef(orderID, skuID), reviewPhotoRef(orderID, skuID)
	text, photo := a.lot(PointSourceReview, textRef), a.lot(PointSourceReview, photoRef)
	if (text != nil && text.Status == LotRevoked) || (photo != nil && photo.Status == LotRevoked) {
		return 0
	}
	var added int64
	note := ReviewNoteText
	if text == nil {
		a.addLot(PointSourceReview, textRef, ReviewTextPoints, "", at)
		added += ReviewTextPoints
	}
	if photoCount > 0 && photo == nil {
		bonus := ReviewPhotoPoints - ReviewTextPoints
		a.addLot(PointSourceReview, photoRef, bonus, "", at)
		added += bonus
		note = ReviewNotePhoto
	}
	if added > 0 {
		a.entry(EntryReview, added, orderID, reviewID, note, at)
	}
	return added
}

// ReviewHidden takes a line's review reward back when staff hide the review.
// Unhiding does not give it back; a manager can grant points by hand. It
// returns what could not be collected because it was already spent.
func (a *PointAccount) ReviewHidden(orderID, skuID string, at time.Time) (shortfall int64) {
	if orderID == "" || skuID == "" {
		return 0
	}
	for _, ref := range []string{reviewLineRef(orderID, skuID), reviewPhotoRef(orderID, skuID)} {
		shortfall += a.revokeReviewLot(a.lot(PointSourceReview, ref), orderID, at)
	}
	return shortfall
}

// revokeReviewsForOrder takes back every review reward on a canceled order.
func (a *PointAccount) revokeReviewsForOrder(orderID string, at time.Time) (shortfall int64) {
	prefix := orderID + ":"
	for _, l := range a.Lots {
		if l.Source == PointSourceReview && strings.HasPrefix(l.Ref, prefix) {
			shortfall += a.revokeReviewLot(l, orderID, at)
		}
	}
	return shortfall
}

func (a *PointAccount) revokeReviewLot(l *PointLot, orderID string, at time.Time) int64 {
	if l == nil {
		return 0
	}
	switch l.Status {
	case LotAvailable, LotExpired:
		return a.clawback(l, orderID, at)
	case LotPending:
		l.Status = LotRevoked
	}
	return 0
}

// addLot credits a new spendable lot without writing a history line.
func (a *PointAccount) addLot(source, ref string, points int64, note string, at time.Time) {
	credited := at
	a.Lots = append(a.Lots, &PointLot{
		ID: a.newID(), CustomerID: a.CustomerID, Source: source, Ref: ref,
		Status: LotAvailable, Points: points, Remaining: points, Note: note,
		CreatedAt: at, CreditedAt: &credited, ExpiresAt: expiry(at),
	})
}
