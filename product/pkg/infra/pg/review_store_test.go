package pg

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

func newReviewTestStore(t *testing.T, schema string) (*ReviewStore, *ProductSearchStore, string) {
	t.Helper()
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, schema)
	ctx := t.Context()

	products := &ProductSearchStore{pool: pool}
	if err := products.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stock, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	if err := products.SeedMockEcoBag(ctx, stock); err != nil {
		t.Fatalf("seed: %v", err)
	}
	v, err := products.GetVariant(ctx, "DUP_ECO01_BLK_OS")
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	reviews, err := NewReviewStore(pool)
	if err != nil {
		t.Fatalf("NewReviewStore: %v", err)
	}
	return reviews, products, v.ProductID
}

func testReview(productID, customer, order string, rating int) *domain.Review {
	return &domain.Review{
		ID: ulid.Make().String(), ProductID: productID, SkuID: "SKU-" + order, OrderID: order,
		CustomerID: customer, Author: "김**", Rating: rating, Body: "가죽 결이 곱고 좋아요.",
		CreatedAt: time.Now().UTC(),
	}
}

func testPhoto(t *testing.T, s *ReviewStore, customer string, at time.Time) string {
	t.Helper()
	id := ulid.Make().String()
	if err := s.CreateReviewPhoto(t.Context(), &domain.ReviewPhoto{
		ID: id, CustomerID: customer, ObjectKey: "reviews/" + id + ".jpg", ThumbKey: "reviews/" + id + ".w400.jpg", CreatedAt: at,
	}); err != nil {
		t.Fatalf("create photo: %v", err)
	}
	return id
}

func TestReviewStore_CountersFollowEveryWrite(t *testing.T) {
	s, products, productID := newReviewTestStore(t, "review_store_counters_test")
	ctx := t.Context()

	p1 := testPhoto(t, s, "c1", time.Now())
	p2 := testPhoto(t, s, "c1", time.Now())
	r1 := testReview(productID, "c1", "O1", 5)
	r1.Fit = &domain.ReviewFit{Result: domain.FitLarge}
	if err := s.CreateReview(ctx, r1, []string{p2, p1}); err != nil {
		t.Fatalf("create: %v", err)
	}
	r2 := testReview(productID, "c2", "O2", 2)
	if err := s.CreateReview(ctx, r2, nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	c, err := s.ReviewCounters(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.ReviewCounters{Count: 2, RatingSum: 7, RatingCounts: [5]int64{0, 1, 0, 0, 1}, PhotoCount: 2, FitCounts: [3]int64{0, 0, 1}}
	if c != want {
		t.Fatalf("counters %+v, want %+v", c, want)
	}
	p, err := products.GetProduct(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if p.ReviewCount != 2 || p.RatingAverage == nil || *p.RatingAverage != 3.5 {
		t.Fatalf("product read: count=%d avg=%v", p.ReviewCount, p.RatingAverage)
	}

	got, err := s.GetReview(ctx, r1.ID)
	if err != nil || len(got.Photos) != 2 || got.Photos[0].ID != p2 || got.Fit == nil {
		t.Fatalf("get: %+v %v", got, err)
	}

	// Hide: out of the counters. Unhide: back.
	if _, err := s.SetReviewHidden(ctx, r1.ID, true, "spam", "m1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.ReviewCounters(ctx, productID); c.Count != 1 || c.PhotoCount != 0 || c.FitCounts[2] != 0 {
		t.Fatalf("after hide: %+v", c)
	}
	if _, err := s.UpdateReview(ctx, r1, nil, false); !errors.Is(err, ports.ErrReviewNotFound) {
		t.Fatalf("update hidden: %v", err)
	}
	if _, err := s.SetReviewHidden(ctx, r1.ID, false, "", "m1", time.Now()); err != nil {
		t.Fatal(err)
	}

	// Replace photos: p1 detached and returned.
	now := time.Now()
	r1.Rating, r1.EditedAt = 4, &now
	detached, err := s.UpdateReview(ctx, r1, []string{p2}, true)
	if err != nil || len(detached) != 1 || detached[0].ID != p1 {
		t.Fatalf("update: %+v %v", detached, err)
	}
	if c, _ := s.ReviewCounters(ctx, productID); c.RatingSum != 6 || c.PhotoCount != 1 {
		t.Fatalf("after update: %+v", c)
	}

	// Same line again.
	dup := testReview(productID, "c1", "O1", 3)
	dup.SkuID = r1.SkuID
	var exists *ports.ReviewExistsError
	if err := s.CreateReview(ctx, dup, nil); !errors.As(err, &exists) || exists.ReviewID != r1.ID {
		t.Fatalf("duplicate: %v", err)
	}

	// Another customer's photo cannot be attached.
	other := testPhoto(t, s, "c9", time.Now())
	if err := s.CreateReview(ctx, testReview(productID, "c1", "O3", 4), []string{other}); !errors.Is(err, ports.ErrReviewPhotoUnusable) {
		t.Fatalf("foreign photo: %v", err)
	}

	photos, err := s.DeleteReview(ctx, r1.ID)
	if err != nil || len(photos) != 1 {
		t.Fatalf("delete: %+v %v", photos, err)
	}
	if c, _ := s.ReviewCounters(ctx, productID); c.Count != 1 || c.RatingSum != 2 {
		t.Fatalf("after delete: %+v", c)
	}
}

func TestReviewStore_ListSortsAndPages(t *testing.T) {
	s, _, productID := newReviewTestStore(t, "review_store_list_test")
	ctx := t.Context()
	ratings := []int{3, 5, 1, 5, 4}
	for i, r := range ratings {
		rv := testReview(productID, fmt.Sprintf("c%d", i), fmt.Sprintf("O%d", i), r)
		var photos []string
		if i%2 == 0 {
			photos = []string{testPhoto(t, s, rv.CustomerID, time.Now())}
		}
		if err := s.CreateReview(ctx, rv, photos); err != nil {
			t.Fatal(err)
		}
	}

	collect := func(q domain.ReviewQuery) []int {
		var out []int
		for {
			page, err := s.ListProductReviews(ctx, productID, q)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range page.Reviews {
				out = append(out, r.Rating)
			}
			if page.NextCursor == "" {
				return out
			}
			q.Cursor = page.NextCursor
		}
	}
	if got := collect(domain.ReviewQuery{Sort: domain.ReviewSortLatest, Limit: 2}); fmt.Sprint(got) != "[4 5 1 5 3]" {
		t.Fatalf("latest: %v", got)
	}
	if got := collect(domain.ReviewQuery{Sort: domain.ReviewSortRatingDesc, Limit: 2}); fmt.Sprint(got) != "[5 5 4 3 1]" {
		t.Fatalf("rating desc: %v", got)
	}
	if got := collect(domain.ReviewQuery{Sort: domain.ReviewSortRatingAsc, Limit: 2}); fmt.Sprint(got) != "[1 3 4 5 5]" {
		t.Fatalf("rating asc: %v", got)
	}
	if got := collect(domain.ReviewQuery{Sort: domain.ReviewSortLatest, Limit: 10, PhotoOnly: true}); fmt.Sprint(got) != "[4 1 3]" {
		t.Fatalf("photo only: %v", got)
	}
	if got := collect(domain.ReviewQuery{Sort: domain.ReviewSortLatest, Limit: 10, Rating: 5}); fmt.Sprint(got) != "[5 5]" {
		t.Fatalf("rating 5: %v", got)
	}

	var photoIDs []string
	q := ""
	for {
		page, err := s.ListProductReviewPhotos(ctx, productID, 2, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range page.Photos {
			photoIDs = append(photoIDs, p.ID)
		}
		if page.NextCursor == "" {
			break
		}
		q = page.NextCursor
	}
	if len(photoIDs) != 3 {
		t.Fatalf("photo pages: %v", photoIDs)
	}
}

func TestReviewStore_CustomerDeletionAndOrphans(t *testing.T) {
	s, _, productID := newReviewTestStore(t, "review_store_customer_test")
	ctx := t.Context()
	attached := testPhoto(t, s, "gone", time.Now())
	loose := testPhoto(t, s, "gone", time.Now())
	if err := s.CreateReview(ctx, testReview(productID, "gone", "O1", 5), []string{attached}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateReview(ctx, testReview(productID, "stays", "O2", 1), nil); err != nil {
		t.Fatal(err)
	}
	photos, err := s.DeleteCustomerReviews(ctx, "gone")
	if err != nil || len(photos) != 2 {
		t.Fatalf("delete customer: %+v %v", photos, err)
	}
	_ = loose
	if c, _ := s.ReviewCounters(ctx, productID); c.Count != 1 || c.RatingSum != 1 || c.PhotoCount != 0 {
		t.Fatalf("counters keep the deleted account: %+v", c)
	}

	old := testPhoto(t, s, "c1", time.Now().Add(-48*time.Hour))
	testPhoto(t, s, "c1", time.Now())
	swept, err := s.DeleteOrphanReviewPhotos(ctx, time.Now().Add(-24*time.Hour))
	if err != nil || len(swept) != 1 || swept[0].ID != old {
		t.Fatalf("sweep: %+v %v", swept, err)
	}
}

// Concurrent writers on one product must leave exact counters: the products
// row lock makes each recount see the other's committed row.
func TestReviewStore_ConcurrentWritesKeepExactCounters(t *testing.T) {
	s, _, productID := newReviewTestStore(t, "review_store_concurrent_test")
	ctx := t.Context()
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.CreateReview(ctx, testReview(productID, fmt.Sprintf("c%d", i), fmt.Sprintf("O%d", i), 1+i%5), nil)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.ReviewCounters(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Count != n || c.RatingSum != 60 {
		t.Fatalf("counters %+v, want count %d sum 60", c, n)
	}
}
