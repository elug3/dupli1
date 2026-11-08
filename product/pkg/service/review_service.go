package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/imaging"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// Review service errors. The handler maps each to a status and a reason.
var (
	// ErrNotReviewable: the order is not the caller's, not fulfilled, or has
	// no such SKU. One error for all three, so the API reveals nothing about
	// other people's orders.
	ErrNotReviewable = errors.New("order line not reviewable")
	// ErrCustomerRequired: service accounts do not buy, so they do not review.
	ErrCustomerRequired = errors.New("customer account required")
	// ErrReviewHidden: a hidden review cannot be edited back into view.
	ErrReviewHidden = errors.New("review is hidden")
	// ErrInvalidReviewPhoto: wrong type, too large, unreadable.
	ErrInvalidReviewPhoto = errors.New("invalid review photo")
	// ErrReviewsUnavailable: photo storage or the gateway is not configured.
	ErrReviewsUnavailable = errors.New("reviews unavailable")
)

// Review photo upload limits.
const (
	ReviewPhotoMaxBytes = 10 << 20
	// OrphanReviewPhotoAge is how long an upload may wait for a review.
	OrphanReviewPhotoAge = 24 * time.Hour
)

var reviewPhotoTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// ReviewCaller is the signed-in shopper.
type ReviewCaller struct {
	CustomerID  string
	AccountType string
	// Bearer is the caller's own access token, forwarded to order and profile
	// so their ownership checks apply.
	Bearer string
}

func (c ReviewCaller) check() error {
	if strings.TrimSpace(c.CustomerID) == "" || strings.EqualFold(c.AccountType, "service") {
		return ErrCustomerRequired
	}
	return nil
}

// ReviewCatalog is the part of the catalog reviews read.
type ReviewCatalog interface {
	GetProduct(ctx context.Context, id string) (*domain.Product, error)
	GetVariantBySkuID(ctx context.Context, skuID string) (*domain.Variant, error)
}

// CreateReviewInput is a new review for one order line.
type CreateReviewInput struct {
	OrderID string `json:"orderId"`
	SkuID   string `json:"skuId"`
	domain.ReviewInput
}

// ProductReviews is the public answer for a product page.
type ProductReviews struct {
	Summary    domain.ReviewSummary `json:"summary"`
	Reviews    []domain.Review      `json:"reviews"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

// ReviewService implements docs/product-reviews-plan.md.
type ReviewService struct {
	store    ports.ReviewStore
	catalog  ReviewCatalog
	objects  ports.ReviewObjectStore
	orders   ports.OrderReader
	profiles ports.ProfileReader
	now      func() time.Time
}

func NewReviewService(store ports.ReviewStore, catalog ReviewCatalog) *ReviewService {
	return &ReviewService{store: store, catalog: catalog, now: time.Now}
}

// WithObjects enables photo uploads.
func (s *ReviewService) WithObjects(objects ports.ReviewObjectStore) *ReviewService {
	s.objects = objects
	return s
}

// PhotosEnabled reports whether photo storage is configured.
func (s *ReviewService) PhotosEnabled() bool { return s.objects != nil }

// WithGateway wires the order check and the author's display name.
func (s *ReviewService) WithGateway(orders ports.OrderReader, profiles ports.ProfileReader) *ReviewService {
	s.orders = orders
	s.profiles = profiles
	return s
}

// WithClock overrides time.Now (tests).
func (s *ReviewService) WithClock(now func() time.Time) *ReviewService {
	s.now = now
	return s
}

// ── Public reads ──────────────────────────────────────────────────────────

// ProductReviews returns a product's summary and one page of visible reviews.
func (s *ReviewService) ProductReviews(ctx context.Context, productID string, q domain.ReviewQuery) (*ProductReviews, error) {
	product, err := s.publicProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	sort, ok := domain.NormalizeReviewSort(q.Sort)
	if !ok {
		return nil, ports.Invalid("sort must be latest, rating_desc or rating_asc")
	}
	if q.Rating < 0 || q.Rating > domain.ReviewMaxRating {
		return nil, ports.Invalid("rating must be 1–5")
	}
	q.Sort = sort
	q.Limit = domain.ClampLimit(q.Limit, domain.ReviewDefaultLimit, domain.ReviewMaxLimit)

	counters, err := s.store.ReviewCounters(ctx, product.ID)
	if err != nil {
		return nil, err
	}
	page, err := s.store.ListProductReviews(ctx, product.ID, q)
	if err != nil {
		return nil, err
	}
	out := &ProductReviews{
		Summary:    counters.Summary(domain.IsClothing(product.Category)),
		Reviews:    make([]domain.Review, 0, len(page.Reviews)),
		NextCursor: page.NextCursor,
	}
	for _, r := range page.Reviews {
		out.Reviews = append(out.Reviews, s.withURLs(r).Public())
	}
	return out, nil
}

// ProductReviewPhotos pages every photo of a product's visible reviews.
func (s *ReviewService) ProductReviewPhotos(ctx context.Context, productID string, limit int, cursor string) (ports.ReviewPhotoPage, error) {
	if _, err := s.publicProduct(ctx, productID); err != nil {
		return ports.ReviewPhotoPage{}, err
	}
	limit = domain.ClampLimit(limit, domain.ReviewPhotoDefaultLimit, domain.ReviewPhotoMaxLimit)
	page, err := s.store.ListProductReviewPhotos(ctx, productID, limit, cursor)
	if err != nil {
		return ports.ReviewPhotoPage{}, err
	}
	if page.Photos == nil {
		page.Photos = []domain.ReviewPhoto{}
	}
	for i := range page.Photos {
		page.Photos[i] = s.photoURLs(page.Photos[i])
	}
	return page, nil
}

// publicProduct is a product a visitor may see reviews of: an active one,
// matching the public PDP, which answers 404 for drafts and archived parents.
func (s *ReviewService) publicProduct(ctx context.Context, productID string) (*domain.Product, error) {
	product, err := s.catalog.GetProduct(ctx, strings.TrimSpace(productID))
	if err != nil {
		return nil, err
	}
	if product.Status != "active" {
		return nil, ports.NotFound("product not found")
	}
	return product, nil
}

// ── The shopper's writes ──────────────────────────────────────────────────

// UploadPhoto re-encodes a photo and keeps it unattached until a review
// takes it (or the sweep deletes it).
func (s *ReviewService) UploadPhoto(ctx context.Context, caller ReviewCaller, data []byte, contentType string) (*domain.ReviewPhoto, error) {
	if err := caller.check(); err != nil {
		return nil, err
	}
	if s.objects == nil {
		return nil, ErrReviewsUnavailable
	}
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if !reviewPhotoTypes[contentType] {
		return nil, fmt.Errorf("%w: use JPEG, PNG or WebP", ErrInvalidReviewPhoto)
	}
	if len(data) == 0 || len(data) > ReviewPhotoMaxBytes {
		return nil, fmt.Errorf("%w: at most 10 MB", ErrInvalidReviewPhoto)
	}
	full, thumb, err := imaging.ReviewJPEGs(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidReviewPhoto, err)
	}

	// The key names neither the shopper nor the product: the URL is public.
	id := ulid.Make().String()
	photo := &domain.ReviewPhoto{
		ID:         id,
		ObjectKey:  "reviews/" + id + ".jpg",
		ThumbKey:   "reviews/" + id + ".w400.jpg",
		CustomerID: caller.CustomerID,
		CreatedAt:  s.now().UTC(),
	}
	if photo.URL, err = s.objects.Upload(ctx, photo.ObjectKey, bytes.NewReader(full), int64(len(full)), "image/jpeg"); err != nil {
		return nil, err
	}
	if photo.ThumbURL, err = s.objects.Upload(ctx, photo.ThumbKey, bytes.NewReader(thumb), int64(len(thumb)), "image/jpeg"); err != nil {
		s.deleteObjects(ctx, []domain.ReviewPhoto{{ObjectKey: photo.ObjectKey}})
		return nil, err
	}
	if err := s.store.CreateReviewPhoto(ctx, photo); err != nil {
		s.deleteObjects(ctx, []domain.ReviewPhoto{*photo})
		return nil, err
	}
	return photo, nil
}

// Create writes a review for one fulfilled order line.
func (s *ReviewService) Create(ctx context.Context, caller ReviewCaller, productID string, in CreateReviewInput) (*domain.Review, error) {
	if err := caller.check(); err != nil {
		return nil, err
	}
	if s.orders == nil {
		return nil, ErrReviewsUnavailable
	}
	in.OrderID = strings.TrimSpace(in.OrderID)
	in.SkuID = strings.TrimSpace(in.SkuID)
	if in.OrderID == "" || in.SkuID == "" {
		return nil, ports.Invalid("orderId and skuId are required")
	}
	if in.Rating == nil || in.Body == nil {
		return nil, ports.Invalid("rating and body are required")
	}

	product, err := s.catalog.GetProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	variant, err := s.catalog.GetVariantBySkuID(ctx, in.SkuID)
	if errors.Is(err, ports.ErrNotFound) || (err == nil && variant.ProductID != product.ID) {
		return nil, ErrNotReviewable
	}
	if err != nil {
		return nil, err
	}

	body := strings.TrimSpace(*in.Body)
	fit := domain.NormalizeReviewFit(in.Fit)
	var photoIDs []string
	if in.PhotoIDs != nil {
		photoIDs = dedupeIDs(*in.PhotoIDs)
	}
	if err := domain.ValidateReviewContent(*in.Rating, body, fit, len(photoIDs), domain.IsClothing(product.Category)); err != nil {
		return nil, err
	}

	// The order is checked last: it is the only step that leaves the service.
	order, err := s.orders.Order(ctx, caller.Bearer, in.OrderID)
	if errors.Is(err, ports.ErrOrderNotReadable) {
		return nil, ErrNotReviewable
	}
	if err != nil {
		return nil, err
	}
	// A manager holding order.read.all can read anyone's order, so ownership
	// is checked here too, not left to order.
	if order.CustomerID != caller.CustomerID || order.Status != "fulfilled" || !orderHasLine(order, variant) {
		return nil, ErrNotReviewable
	}

	author := domain.ReviewAnonymousAuthor
	if s.profiles != nil {
		name, err := s.profiles.DisplayName(ctx, caller.Bearer)
		if err != nil {
			// The name is a courtesy; a review is not refused for it.
			log.Printf("product: review author name unavailable for %s: %v", caller.CustomerID, err)
		}
		author = domain.MaskAuthor(name)
	}

	review := &domain.Review{
		ID:               ulid.Make().String(),
		ProductID:        product.ID,
		SkuID:            variant.SkuID,
		VariantLabel:     variantLabel(variant),
		Rating:           *in.Rating,
		Body:             body,
		Fit:              fit,
		Author:           author,
		VerifiedPurchase: true,
		CreatedAt:        s.now().UTC(),
		OrderID:          order.ID,
		CustomerID:       caller.CustomerID,
	}
	if err := s.store.CreateReview(ctx, review, photoIDs); err != nil {
		return nil, err
	}
	created, err := s.store.GetReview(ctx, review.ID)
	if err != nil {
		return nil, err
	}
	out := s.withURLs(*created).ForAuthor()
	return &out, nil
}

// Mine lists the caller's reviews; orderID narrows to one order.
func (s *ReviewService) Mine(ctx context.Context, caller ReviewCaller, orderID string) ([]domain.Review, error) {
	if err := caller.check(); err != nil {
		return nil, err
	}
	reviews, err := s.store.ListCustomerReviews(ctx, caller.CustomerID, strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Review, 0, len(reviews))
	for _, r := range reviews {
		out = append(out, s.withURLs(r).ForAuthor())
	}
	return out, nil
}

// Update edits the caller's review. Product, variant and order stay.
func (s *ReviewService) Update(ctx context.Context, caller ReviewCaller, reviewID string, in domain.ReviewInput) (*domain.Review, error) {
	if err := caller.check(); err != nil {
		return nil, err
	}
	review, err := s.ownReview(ctx, caller, reviewID)
	if err != nil {
		return nil, err
	}
	if review.Hidden {
		return nil, ErrReviewHidden
	}
	product, err := s.catalog.GetProduct(ctx, review.ProductID)
	if err != nil {
		return nil, err
	}

	if in.Rating != nil {
		review.Rating = *in.Rating
	}
	if in.Body != nil {
		review.Body = strings.TrimSpace(*in.Body)
	}
	if in.Fit != nil {
		review.Fit = domain.NormalizeReviewFit(in.Fit)
	}
	photoCount := len(review.Photos)
	var photoIDs []string
	if in.PhotoIDs != nil {
		photoIDs = dedupeIDs(*in.PhotoIDs)
		photoCount = len(photoIDs)
	}
	if err := domain.ValidateReviewContent(review.Rating, review.Body, review.Fit, photoCount, domain.IsClothing(product.Category)); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	review.EditedAt = &now

	detached, err := s.store.UpdateReview(ctx, review, photoIDs, in.PhotoIDs != nil)
	if err != nil {
		return nil, err
	}
	s.deleteObjects(ctx, detached)
	updated, err := s.store.GetReview(ctx, review.ID)
	if err != nil {
		return nil, err
	}
	out := s.withURLs(*updated).ForAuthor()
	return &out, nil
}

// Delete removes the caller's review and its photos.
func (s *ReviewService) Delete(ctx context.Context, caller ReviewCaller, reviewID string) error {
	if err := caller.check(); err != nil {
		return err
	}
	if _, err := s.ownReview(ctx, caller, reviewID); err != nil {
		return err
	}
	photos, err := s.store.DeleteReview(ctx, reviewID)
	if err != nil {
		return err
	}
	s.deleteObjects(ctx, photos)
	return nil
}

// ownReview returns the caller's review; someone else's is not found.
func (s *ReviewService) ownReview(ctx context.Context, caller ReviewCaller, reviewID string) (*domain.Review, error) {
	review, err := s.store.GetReview(ctx, strings.TrimSpace(reviewID))
	if err != nil {
		return nil, err
	}
	if review.CustomerID != caller.CustomerID {
		return nil, ports.ErrReviewNotFound
	}
	return review, nil
}

// ── Staff ─────────────────────────────────────────────────────────────────

// Moderation pages every review with the staff fields.
func (s *ReviewService) Moderation(ctx context.Context, q domain.ModerationQuery) (ports.ReviewPage, error) {
	if q.Rating < 0 || q.Rating > domain.ReviewMaxRating {
		return ports.ReviewPage{}, ports.Invalid("rating must be 1–5")
	}
	q.Limit = domain.ClampLimit(q.Limit, 50, domain.ReviewModerationMaxLimit)
	page, err := s.store.ListReviewsForModeration(ctx, q)
	if err != nil {
		return ports.ReviewPage{}, err
	}
	if page.Reviews == nil {
		page.Reviews = []domain.Review{}
	}
	for i := range page.Reviews {
		page.Reviews[i] = s.withURLs(page.Reviews[i])
	}
	return page, nil
}

// SetHidden hides (with a reason the author sees) or unhides a review.
func (s *ReviewService) SetHidden(ctx context.Context, reviewID string, hidden bool, reason, by string) (*domain.Review, error) {
	reason = strings.TrimSpace(reason)
	if hidden && reason == "" {
		return nil, ports.Invalid("reason is required to hide a review")
	}
	if utf8.RuneCountInString(reason) > domain.ReviewHideReasonMaxChars {
		return nil, ports.Invalid("reason is at most 200 characters")
	}
	if !hidden {
		reason = ""
	}
	review, err := s.store.SetReviewHidden(ctx, strings.TrimSpace(reviewID), hidden, reason, by, s.now().UTC())
	if err != nil {
		return nil, err
	}
	out := s.withURLs(*review)
	return &out, nil
}

// ── Account deletion and housekeeping ─────────────────────────────────────

// Register subscribes to auth's user.deleted.
func (s *ReviewService) Register(ctx context.Context, subscriber ports.EventSubscriber) error {
	if s == nil || subscriber == nil {
		return nil
	}
	return subscriber.Subscribe(ctx, events.UserDeleted, s.HandleUserDeleted)
}

// HandleUserDeleted deletes every review, rating and photo of a deleted
// account. There are no anonymous ratings: nothing of the account stays.
func (s *ReviewService) HandleUserDeleted(ctx context.Context, subject string, payload []byte) error {
	var event events.UserDeletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("%s: decode: %w", subject, err)
	}
	if strings.TrimSpace(event.UserID) == "" {
		return fmt.Errorf("%s: event carries no user_id", subject)
	}
	photos, err := s.store.DeleteCustomerReviews(ctx, event.UserID)
	if err != nil {
		return fmt.Errorf("delete reviews of %s: %w", event.UserID, err)
	}
	s.deleteObjects(ctx, photos)
	return nil
}

// SweepOrphanPhotos deletes uploads no review took within a day.
func (s *ReviewService) SweepOrphanPhotos(ctx context.Context) error {
	photos, err := s.store.DeleteOrphanReviewPhotos(ctx, s.now().Add(-OrphanReviewPhotoAge))
	if err != nil {
		return err
	}
	s.deleteObjects(ctx, photos)
	return nil
}

// StartOrphanPhotoSweeper runs SweepOrphanPhotos every interval until ctx ends.
func (s *ReviewService) StartOrphanPhotoSweeper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.SweepOrphanPhotos(ctx); err != nil {
					log.Printf("product: review photo sweep: %v", err)
				}
			}
		}
	}()
}

// withURLs fills photo URLs from their object keys. URLs are built at read
// time, never stored, so moving the public image host needs no data change.
func (s *ReviewService) withURLs(r domain.Review) domain.Review {
	photos := make([]domain.ReviewPhoto, len(r.Photos))
	for i, p := range r.Photos {
		photos[i] = s.photoURLs(p)
	}
	r.Photos = photos
	return r
}

func (s *ReviewService) photoURLs(p domain.ReviewPhoto) domain.ReviewPhoto {
	if s.objects != nil {
		p.URL = s.objects.PublicURL(p.ObjectKey)
		p.ThumbURL = s.objects.PublicURL(p.ThumbKey)
	}
	return p
}

// deleteObjects removes photo objects after their rows are gone. A failure
// leaves an unreferenced object, which is logged, never a broken review.
func (s *ReviewService) deleteObjects(ctx context.Context, photos []domain.ReviewPhoto) {
	if s.objects == nil {
		return
	}
	for _, p := range photos {
		for _, key := range []string{p.ObjectKey, p.ThumbKey} {
			if key == "" {
				continue
			}
			if err := s.objects.Delete(ctx, key); err != nil {
				log.Printf("product: delete review photo object %s: %v", key, err)
			}
		}
	}
}

func orderHasLine(order *ports.ReviewOrder, variant *domain.Variant) bool {
	for _, line := range order.Lines {
		if line.SkuID != "" && line.SkuID == variant.SkuID {
			return true
		}
		// Orders written before sku_id carried only the SKU string.
		if line.SkuID == "" && line.SKU != "" && strings.EqualFold(line.SKU, variant.SKU) {
			return true
		}
	}
	return false
}

func variantLabel(v *domain.Variant) string {
	parts := make([]string, 0, 2)
	if c := strings.TrimSpace(v.Color); c != "" {
		parts = append(parts, c)
	}
	if size := strings.TrimSpace(v.Size); size != "" {
		parts = append(parts, size)
	}
	return strings.Join(parts, " / ")
}

func dedupeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
