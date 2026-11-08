package memory

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// ReviewStore is an in-memory ports.ReviewStore for tests. It mirrors the
// Postgres store: counters are recomputed from the rows after every write and
// copied onto the product (ReviewCount, RatingAverage).
type ReviewStore struct {
	mu       sync.Mutex
	products *ProductStore
	reviews  map[string]domain.Review
	photos   map[string]domain.ReviewPhoto
	position map[string]int
	counters map[string]domain.ReviewCounters
}

func NewReviewStore(products *ProductStore) *ReviewStore {
	return &ReviewStore{
		products: products,
		reviews:  map[string]domain.Review{},
		photos:   map[string]domain.ReviewPhoto{},
		position: map[string]int{},
		counters: map[string]domain.ReviewCounters{},
	}
}

func (s *ReviewStore) productExists(id string) bool {
	if s.products == nil {
		return true
	}
	for _, p := range s.products.Products {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (s *ReviewStore) recount(productID string) {
	var c domain.ReviewCounters
	for _, r := range s.reviews {
		if r.ProductID != productID || r.Hidden {
			continue
		}
		c.Count++
		c.RatingSum += int64(r.Rating)
		c.RatingCounts[r.Rating-1]++
		if r.Fit != nil {
			if i := domain.FitIndex(r.Fit.Result); i >= 0 {
				c.FitCounts[i]++
			}
		}
		for _, p := range s.photos {
			if p.ReviewID == r.ID {
				c.PhotoCount++
			}
		}
	}
	s.counters[productID] = c
	if s.products != nil {
		for i := range s.products.Products {
			if s.products.Products[i].ID == productID {
				s.products.Products[i].ReviewCount = c.Count
				s.products.Products[i].RatingAverage = domain.RatingAverage(c.Count, c.RatingSum)
			}
		}
	}
}

func (s *ReviewStore) attach(reviewID, customerID string, ids []string) error {
	for _, id := range ids {
		p, ok := s.photos[id]
		if !ok || p.CustomerID != customerID || (p.ReviewID != "" && p.ReviewID != reviewID) {
			return fmt.Errorf("photo %s: %w", id, ports.ErrReviewPhotoUnusable)
		}
	}
	for i, id := range ids {
		p := s.photos[id]
		p.ReviewID = reviewID
		s.photos[id] = p
		s.position[id] = i
	}
	return nil
}

func (s *ReviewStore) withPhotos(r domain.Review) domain.Review {
	r.Photos = []domain.ReviewPhoto{}
	for id, p := range s.photos {
		if p.ReviewID == r.ID {
			_ = id
			r.Photos = append(r.Photos, p)
		}
	}
	sort.Slice(r.Photos, func(i, j int) bool { return s.position[r.Photos[i].ID] < s.position[r.Photos[j].ID] })
	r.VerifiedPurchase = true
	return r
}

func (s *ReviewStore) CreateReview(_ context.Context, r *domain.Review, photoIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.productExists(r.ProductID) {
		return ports.Invalid("unknown product")
	}
	for _, existing := range s.reviews {
		if existing.OrderID == r.OrderID && existing.SkuID == r.SkuID {
			return &ports.ReviewExistsError{ReviewID: existing.ID}
		}
	}
	if err := s.attach(r.ID, r.CustomerID, photoIDs); err != nil {
		return err
	}
	stored := *r
	stored.Photos = nil
	s.reviews[r.ID] = stored
	s.recount(r.ProductID)
	return nil
}

func (s *ReviewStore) GetReview(_ context.Context, id string) (*domain.Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reviews[id]
	if !ok {
		return nil, ports.ErrReviewNotFound
	}
	out := s.withPhotos(r)
	return &out, nil
}

func (s *ReviewStore) UpdateReview(_ context.Context, r *domain.Review, photoIDs []string, replacePhotos bool) ([]domain.ReviewPhoto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.reviews[r.ID]
	if !ok || stored.Hidden {
		return nil, ports.ErrReviewNotFound
	}
	var detached []domain.ReviewPhoto
	if replacePhotos {
		keep := map[string]bool{}
		for _, id := range photoIDs {
			keep[id] = true
		}
		for _, id := range photoIDs {
			p, ok := s.photos[id]
			if !ok || p.CustomerID != stored.CustomerID || (p.ReviewID != "" && p.ReviewID != r.ID) {
				return nil, fmt.Errorf("photo %s: %w", id, ports.ErrReviewPhotoUnusable)
			}
		}
		for id, p := range s.photos {
			if p.ReviewID == r.ID && !keep[id] {
				detached = append(detached, p)
				delete(s.photos, id)
			}
		}
		_ = s.attach(r.ID, stored.CustomerID, photoIDs)
	}
	stored.Rating, stored.Body, stored.Fit, stored.EditedAt = r.Rating, r.Body, r.Fit, r.EditedAt
	s.reviews[r.ID] = stored
	s.recount(stored.ProductID)
	return detached, nil
}

func (s *ReviewStore) DeleteReview(_ context.Context, id string) ([]domain.ReviewPhoto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reviews[id]
	if !ok {
		return nil, ports.ErrReviewNotFound
	}
	var photos []domain.ReviewPhoto
	for pid, p := range s.photos {
		if p.ReviewID == id {
			photos = append(photos, p)
			delete(s.photos, pid)
		}
	}
	delete(s.reviews, id)
	s.recount(r.ProductID)
	return photos, nil
}

func (s *ReviewStore) SetReviewHidden(_ context.Context, id string, hidden bool, reason, by string, at time.Time) (*domain.Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reviews[id]
	if !ok {
		return nil, ports.ErrReviewNotFound
	}
	r.Hidden, r.HiddenReason, r.HiddenBy, r.HiddenAt = hidden, reason, by, nil
	if hidden {
		t := at
		r.HiddenAt = &t
	} else {
		r.HiddenReason, r.HiddenBy = "", ""
	}
	s.reviews[id] = r
	s.recount(r.ProductID)
	out := s.withPhotos(r)
	return &out, nil
}

func (s *ReviewStore) sorted(filter func(domain.Review) bool) []domain.Review {
	var out []domain.Review
	for _, r := range s.reviews {
		if filter(r) {
			out = append(out, s.withPhotos(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *ReviewStore) ListProductReviews(_ context.Context, productID string, q domain.ReviewQuery) (ports.ReviewPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.sorted(func(r domain.Review) bool {
		return r.ProductID == productID && !r.Hidden && (q.Rating == 0 || r.Rating == q.Rating)
	})
	if q.PhotoOnly {
		kept := all[:0]
		for _, r := range all {
			if len(r.Photos) > 0 {
				kept = append(kept, r)
			}
		}
		all = kept
	}
	switch q.Sort {
	case domain.ReviewSortRatingDesc:
		sort.SliceStable(all, func(i, j int) bool { return all[i].Rating > all[j].Rating })
	case domain.ReviewSortRatingAsc:
		sort.SliceStable(all, func(i, j int) bool { return all[i].Rating < all[j].Rating })
	}
	start := 0
	if q.Cursor != "" {
		_, id, ok := decodeCursor(q.Cursor)
		if !ok {
			return ports.ReviewPage{}, ports.Invalid("bad cursor")
		}
		for i, r := range all {
			if r.ID == id {
				start = i + 1
			}
		}
	}
	return pageOf(all[start:], q.Limit), nil
}

func pageOf(rest []domain.Review, limit int) ports.ReviewPage {
	if len(rest) > limit {
		last := rest[limit-1]
		return ports.ReviewPage{Reviews: rest[:limit], NextCursor: encodeCursor(last.Rating, last.ID)}
	}
	return ports.ReviewPage{Reviews: rest}
}

func (s *ReviewStore) ListProductReviewPhotos(_ context.Context, productID string, limit int, cursor string) (ports.ReviewPhotoPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var photos []domain.ReviewPhoto
	for _, r := range s.sorted(func(r domain.Review) bool { return r.ProductID == productID && !r.Hidden }) {
		photos = append(photos, r.Photos...)
	}
	start := 0
	if cursor != "" {
		_, id, ok := decodeCursor(cursor)
		if !ok {
			return ports.ReviewPhotoPage{}, ports.Invalid("bad cursor")
		}
		for i, p := range photos {
			if p.ID == id {
				start = i + 1
			}
		}
	}
	photos = photos[start:]
	if len(photos) > limit {
		return ports.ReviewPhotoPage{Photos: photos[:limit], NextCursor: encodeCursor(0, photos[limit-1].ID)}, nil
	}
	return ports.ReviewPhotoPage{Photos: photos}, nil
}

func (s *ReviewStore) ReviewCounters(_ context.Context, productID string) (domain.ReviewCounters, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.productExists(productID) {
		return domain.ReviewCounters{}, ports.NotFound("product")
	}
	return s.counters[productID], nil
}

func (s *ReviewStore) ListCustomerReviews(_ context.Context, customerID, orderID string) ([]domain.Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sorted(func(r domain.Review) bool {
		return r.CustomerID == customerID && (orderID == "" || r.OrderID == orderID)
	}), nil
}

func (s *ReviewStore) ListReviewsForModeration(_ context.Context, q domain.ModerationQuery) (ports.ReviewPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.sorted(func(r domain.Review) bool {
		return r.Hidden == q.Hidden && (q.ProductID == "" || r.ProductID == q.ProductID) && (q.Rating == 0 || r.Rating == q.Rating)
	})
	start := 0
	if q.Cursor != "" {
		_, id, ok := decodeCursor(q.Cursor)
		if !ok {
			return ports.ReviewPage{}, ports.Invalid("bad cursor")
		}
		for i, r := range all {
			if r.ID == id {
				start = i + 1
			}
		}
	}
	return pageOf(all[start:], q.Limit), nil
}

func (s *ReviewStore) CreateReviewPhoto(_ context.Context, p *domain.ReviewPhoto) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := *p
	stored.URL, stored.ThumbURL, stored.ReviewID = "", "", ""
	s.photos[p.ID] = stored
	return nil
}

func (s *ReviewStore) DeleteCustomerReviews(_ context.Context, customerID string) ([]domain.ReviewPhoto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var photos []domain.ReviewPhoto
	for id, p := range s.photos {
		if p.CustomerID == customerID {
			photos = append(photos, p)
			delete(s.photos, id)
		}
	}
	touched := map[string]bool{}
	for id, r := range s.reviews {
		if r.CustomerID == customerID {
			touched[r.ProductID] = true
			delete(s.reviews, id)
		}
	}
	for id := range touched {
		s.recount(id)
	}
	return photos, nil
}

func (s *ReviewStore) DeleteOrphanReviewPhotos(_ context.Context, cutoff time.Time) ([]domain.ReviewPhoto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var photos []domain.ReviewPhoto
	for id, p := range s.photos {
		if p.ReviewID == "" && p.CreatedAt.Before(cutoff) {
			photos = append(photos, p)
			delete(s.photos, id)
		}
	}
	return photos, nil
}

func encodeCursor(n int, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(n) + ":" + id))
}

func decodeCursor(cursor string) (int, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", false
	}
	num, id, ok := strings.Cut(string(raw), ":")
	if !ok {
		return 0, "", false
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0, "", false
	}
	return n, id, true
}
