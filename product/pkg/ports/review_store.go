package ports

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
)

var (
	// ErrReviewExists is a second review for the same order line.
	ErrReviewExists = errors.New("review exists")
	// ErrReviewNotFound is a missing review (or, on author routes, someone
	// else's).
	ErrReviewNotFound = errors.New("review not found")
	// ErrReviewPhotoUnusable is a photo id that is not the caller's, does not
	// exist, or already belongs to another review.
	ErrReviewPhotoUnusable = errors.New("review photo unusable")
)

// ReviewPage is one page of reviews and the cursor for the next.
type ReviewPage struct {
	Reviews    []domain.Review
	NextCursor string
}

// ReviewPhotoPage is one page of photos from visible reviews.
type ReviewPhotoPage struct {
	Photos     []domain.ReviewPhoto
	NextCursor string
}

// ReviewStore keeps reviews and their photos, and the review counters on
// products. Every write recomputes the parent's counters in the same
// transaction, so they never drift from the rows.
type ReviewStore interface {
	// CreateReview inserts r (its ID set by the caller) and attaches photoIDs
	// in order. ErrReviewExists when (OrderID, SkuID) already has one;
	// ErrReviewPhotoUnusable for a bad photo id.
	CreateReview(ctx context.Context, r *domain.Review, photoIDs []string) error
	// GetReview returns a review with its photos, hidden or not.
	GetReview(ctx context.Context, id string) (*domain.Review, error)
	// UpdateReview rewrites rating, body, fit and edited_at. A non-nil
	// photoIDs replaces the photo set; the returned photos are the ones it
	// detached, whose objects the caller deletes.
	UpdateReview(ctx context.Context, r *domain.Review, photoIDs []string, replacePhotos bool) ([]domain.ReviewPhoto, error)
	// DeleteReview removes a review and returns its photos.
	DeleteReview(ctx context.Context, id string) ([]domain.ReviewPhoto, error)
	// SetReviewHidden hides or unhides a review.
	SetReviewHidden(ctx context.Context, id string, hidden bool, reason, by string, at time.Time) (*domain.Review, error)

	// ListProductReviews pages a product's visible reviews.
	ListProductReviews(ctx context.Context, productID string, q domain.ReviewQuery) (ReviewPage, error)
	// ListProductReviewPhotos pages the photos of a product's visible
	// reviews, newest first.
	ListProductReviewPhotos(ctx context.Context, productID string, limit int, cursor string) (ReviewPhotoPage, error)
	// ReviewCounters reads a product's stored counters.
	ReviewCounters(ctx context.Context, productID string) (domain.ReviewCounters, error)
	// ListCustomerReviews returns a customer's reviews, newest first;
	// orderID narrows to one order.
	ListCustomerReviews(ctx context.Context, customerID, orderID string) ([]domain.Review, error)
	// ListReviewsForModeration pages every review for staff.
	ListReviewsForModeration(ctx context.Context, q domain.ModerationQuery) (ReviewPage, error)

	// CreateReviewPhoto records an uploaded, not yet attached photo.
	CreateReviewPhoto(ctx context.Context, p *domain.ReviewPhoto) error
	// DeleteCustomerReviews removes every review and photo of a customer
	// (account deletion) and returns the photos.
	DeleteCustomerReviews(ctx context.Context, customerID string) ([]domain.ReviewPhoto, error)
	// DeleteOrphanReviewPhotos removes photos uploaded before cutoff that no
	// review took, and returns them.
	DeleteOrphanReviewPhotos(ctx context.Context, cutoff time.Time) ([]domain.ReviewPhoto, error)
}

// ReviewObjectStore holds review photo objects.
type ReviewObjectStore interface {
	Upload(ctx context.Context, objectKey string, r io.Reader, size int64, contentType string) (string, error)
	PublicURL(objectKey string) string
	Delete(ctx context.Context, objectKey string) error
}

// OrderLine is one line of an order, as the review check needs it.
type OrderLine struct {
	SkuID string
	SKU   string
}

// ReviewOrder is the part of an order the review check reads.
type ReviewOrder struct {
	ID         string
	CustomerID string
	Status     string
	Lines      []OrderLine
}

var (
	// ErrOrderNotReadable is an order that does not exist or is not the
	// caller's: order answers both the same.
	ErrOrderNotReadable = errors.New("order not readable")
	// ErrReferenceUnavailable is order or profile not answering.
	ErrReferenceUnavailable = errors.New("reference unavailable")
)

// OrderReader reads one order with the shopper's own bearer token, so
// order's ownership check decides who may see it.
type OrderReader interface {
	Order(ctx context.Context, bearer, orderID string) (*ReviewOrder, error)
}

// ProfileReader reads the shopper's own display name with their token.
// Empty when they have none.
type ProfileReader interface {
	DisplayName(ctx context.Context, bearer string) (string, error)
}

// ReviewExistsError is ErrReviewExists with the existing review's id, so the
// storefront can open it for editing instead.
type ReviewExistsError struct {
	ReviewID string
}

func (e *ReviewExistsError) Error() string { return "review exists: " + e.ReviewID }
func (e *ReviewExistsError) Unwrap() error { return ErrReviewExists }
