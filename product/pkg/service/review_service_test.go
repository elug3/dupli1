package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

type fakeOrders struct {
	orders map[string]*ports.ReviewOrder
	err    error
	calls  int
}

func (f *fakeOrders) Order(_ context.Context, bearer, id string) (*ports.ReviewOrder, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if bearer == "" {
		return nil, ports.ErrOrderNotReadable
	}
	o, ok := f.orders[id]
	if !ok {
		return nil, ports.ErrOrderNotReadable
	}
	return o, nil
}

type fakeProfiles struct{ name string }

func (f fakeProfiles) DisplayName(context.Context, string) (string, error) { return f.name, nil }

type fakeObjects struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
}

func newFakeObjects() *fakeObjects { return &fakeObjects{objects: map[string][]byte{}} }

func (f *fakeObjects) Upload(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	data, _ := io.ReadAll(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = data
	return f.PublicURL(key), nil
}
func (f *fakeObjects) PublicURL(key string) string { return "https://img.test/" + key }
func (f *fakeObjects) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

type reviewFixture struct {
	svc      *service.ReviewService
	store    *memory.ReviewStore
	products *memory.ProductStore
	orders   *fakeOrders
	objects  *fakeObjects
	now      time.Time
}

const (
	buyer    = "cust-1"
	stranger = "cust-2"
)

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	products := memory.NewProductStore()
	products.Products = []domain.Product{
		{ID: "BAG-1", Name: "Mini Shoulder", Category: "bags", Status: "active"},
		{ID: "JKT-1", Name: "Padded Jacket", Category: "clothing", Status: "active"},
		{ID: "DRAFT-1", Name: "Draft", Category: "bags", Status: "draft"},
	}
	products.Variants = []domain.Variant{
		{SkuID: "SKU-BAG-BLK", SKU: "DUP_BAG_BLK_OS", ProductID: "BAG-1", Color: "Black", Status: "active"},
		{SkuID: "SKU-BAG-BEI", SKU: "DUP_BAG_BEI_OS", ProductID: "BAG-1", Color: "Beige", Status: "active"},
		{SkuID: "SKU-JKT-M", SKU: "DUP_JKT_BLK_M", ProductID: "JKT-1", Color: "Black", Size: "M", Status: "active"},
	}
	store := memory.NewReviewStore(products)
	orders := &fakeOrders{orders: map[string]*ports.ReviewOrder{
		"ORD-1": {ID: "ORD-1", CustomerID: buyer, Status: "fulfilled", Lines: []ports.OrderLine{
			{SkuID: "SKU-BAG-BLK"}, {SkuID: "SKU-JKT-M"},
		}},
		"ORD-OLD":       {ID: "ORD-OLD", CustomerID: buyer, Status: "fulfilled", Lines: []ports.OrderLine{{SKU: "DUP_BAG_BEI_OS"}}},
		"ORD-DELIVERED": {ID: "ORD-DELIVERED", CustomerID: buyer, Status: "delivered", Lines: []ports.OrderLine{{SkuID: "SKU-BAG-BLK"}}},
		"ORD-2":         {ID: "ORD-2", CustomerID: stranger, Status: "fulfilled", Lines: []ports.OrderLine{{SkuID: "SKU-BAG-BLK"}}},
	}}
	objects := newFakeObjects()
	f := &reviewFixture{store: store, products: products, orders: orders, objects: objects,
		now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
	f.svc = service.NewReviewService(store, products).
		WithObjects(objects).
		WithGateway(orders, fakeProfiles{name: "김민지"}).
		WithClock(func() time.Time { return f.now })
	return f
}

func caller(id string) service.ReviewCaller {
	return service.ReviewCaller{CustomerID: id, AccountType: "customer", Bearer: "token-" + id}
}

func ptr[T any](v T) *T { return &v }

func input(order, sku string, rating int, body string) service.CreateReviewInput {
	return service.CreateReviewInput{OrderID: order, SkuID: sku, ReviewInput: domain.ReviewInput{Rating: &rating, Body: &body}}
}

const goodBody = "가죽 결이 곱고 각이 잘 잡혀 있어요."

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		for y := 0; y < 20; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 6), uint8(y * 12), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCreateReview_FulfilledBuyer(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()

	r, err := f.svc.Create(ctx, caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, goodBody))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if r.Author != "김**" || r.VariantLabel != "Black" || !r.VerifiedPurchase || r.OrderID != "ORD-1" {
		t.Fatalf("unexpected review: %+v", r)
	}
	if r.CustomerID != "" {
		t.Fatalf("author view leaks customerId")
	}

	out, err := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Summary.Count != 1 || *out.Summary.Average != 5 || out.Summary.ByRating["5"] != 1 || out.Summary.Fit != nil {
		t.Fatalf("summary: %+v", out.Summary)
	}
	if len(out.Reviews) != 1 || out.Reviews[0].OrderID != "" {
		t.Fatalf("public review leaks order id: %+v", out.Reviews)
	}
	if f.products.Products[0].ReviewCount != 1 || *f.products.Products[0].RatingAverage != 5 {
		t.Fatalf("product counters not updated: %+v", f.products.Products[0])
	}
}

func TestCreateReview_OldOrderMatchesBySKU(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.svc.Create(context.Background(), caller(buyer), "BAG-1", input("ORD-OLD", "SKU-BAG-BEI", 4, goodBody)); err != nil {
		t.Fatalf("create from an order without sku_id: %v", err)
	}
}

func TestCreateReview_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		caller  service.ReviewCaller
		product string
		in      service.CreateReviewInput
		want    error
	}{
		{"not fulfilled", caller(buyer), "BAG-1", input("ORD-DELIVERED", "SKU-BAG-BLK", 5, goodBody), service.ErrNotReviewable},
		{"someone else's order", caller(buyer), "BAG-1", input("ORD-2", "SKU-BAG-BLK", 5, goodBody), service.ErrNotReviewable},
		{"order readable but not the caller's", caller(stranger), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, goodBody), service.ErrNotReviewable},
		{"sku not on the order", caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BEI", 5, goodBody), service.ErrNotReviewable},
		{"sku of another product", caller(buyer), "BAG-1", input("ORD-1", "SKU-JKT-M", 5, goodBody), service.ErrNotReviewable},
		{"unknown order", caller(buyer), "BAG-1", input("ORD-X", "SKU-BAG-BLK", 5, goodBody), service.ErrNotReviewable},
		{"service account", service.ReviewCaller{CustomerID: "svc", AccountType: "service", Bearer: "x"}, "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, goodBody), service.ErrCustomerRequired},
		{"rating 0", caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 0, goodBody), domain.ErrInvalidReview},
		{"short body", caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, "좋아요"), domain.ErrInvalidReview},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newReviewFixture(t)
			_, err := f.svc.Create(context.Background(), c.caller, c.product, c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestCreateReview_FitOnlyForClothing(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	in := input("ORD-1", "SKU-BAG-BLK", 5, goodBody)
	in.Fit = &domain.ReviewFit{Result: domain.FitTrueToSize}
	if _, err := f.svc.Create(ctx, caller(buyer), "BAG-1", in); !errors.Is(err, domain.ErrInvalidReview) {
		t.Fatalf("fit on a bag: got %v", err)
	}

	in = input("ORD-1", "SKU-JKT-M", 4, goodBody)
	in.Fit = &domain.ReviewFit{Result: domain.FitSmall, HeightCm: 175, UsualSize: "M"}
	if _, err := f.svc.Create(ctx, caller(buyer), "JKT-1", in); err != nil {
		t.Fatalf("fit on clothing: %v", err)
	}
	out, _ := f.svc.ProductReviews(ctx, "JKT-1", domain.ReviewQuery{})
	if out.Summary.Fit == nil || out.Summary.Fit.Small != 1 {
		t.Fatalf("fit summary: %+v", out.Summary.Fit)
	}
}

func TestCreateReview_OncePerLine(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	first, err := f.svc.Create(ctx, caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, goodBody))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Create(ctx, caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 3, goodBody))
	var exists *ports.ReviewExistsError
	if !errors.As(err, &exists) || exists.ReviewID != first.ID {
		t.Fatalf("second review: got %v", err)
	}
}

func TestCreateReview_OrderUnavailable(t *testing.T) {
	f := newReviewFixture(t)
	f.orders.err = ports.ErrReferenceUnavailable
	_, err := f.svc.Create(context.Background(), caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, goodBody))
	if !errors.Is(err, ports.ErrReferenceUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateReview_ValidatesBeforeCallingOrder(t *testing.T) {
	f := newReviewFixture(t)
	_, _ = f.svc.Create(context.Background(), caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 9, goodBody))
	if f.orders.calls != 0 {
		t.Fatalf("an invalid review reached order %d times", f.orders.calls)
	}
}

func TestReviewPhotos_UploadAttachReplace(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 3; i++ {
		p, err := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/png")
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		ids = append(ids, p.ID)
	}
	if len(f.objects.objects) != 6 {
		t.Fatalf("want a full and a thumb object per photo, got %d", len(f.objects.objects))
	}
	if _, err := f.svc.UploadPhoto(ctx, caller(buyer), []byte("not an image"), "image/png"); !errors.Is(err, service.ErrInvalidReviewPhoto) {
		t.Fatalf("garbage upload: %v", err)
	}
	if _, err := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/gif"); !errors.Is(err, service.ErrInvalidReviewPhoto) {
		t.Fatalf("gif upload: %v", err)
	}

	in := input("ORD-1", "SKU-BAG-BLK", 5, goodBody)
	in.PhotoIDs = &[]string{ids[1], ids[0]}
	r, err := f.svc.Create(ctx, caller(buyer), "BAG-1", in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Photos) != 2 || r.Photos[0].ID != ids[1] || r.Photos[0].URL == "" {
		t.Fatalf("photos not attached in order: %+v", r.Photos)
	}

	// Another shopper cannot take the third upload.
	in2 := input("ORD-2", "SKU-BAG-BLK", 4, goodBody)
	in2.PhotoIDs = &[]string{ids[2]}
	if _, err := f.svc.Create(ctx, caller(stranger), "BAG-1", in2); !errors.Is(err, ports.ErrReviewPhotoUnusable) {
		t.Fatalf("someone else's photo: %v", err)
	}

	// Replacing the set deletes the dropped photo's objects.
	updated, err := f.svc.Update(ctx, caller(buyer), r.ID, domain.ReviewInput{PhotoIDs: &[]string{ids[2]}})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Photos) != 1 || updated.Photos[0].ID != ids[2] || updated.EditedAt == nil {
		t.Fatalf("after replace: %+v", updated)
	}
	if len(f.objects.deleted) != 4 {
		t.Fatalf("want 2 photos × 2 objects deleted, got %v", f.objects.deleted)
	}

	out, _ := f.svc.ProductReviewPhotos(ctx, "BAG-1", 0, "")
	if len(out.Photos) != 1 {
		t.Fatalf("photo strip: %+v", out.Photos)
	}
}

func TestReviewPhotos_AtMostTen(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 11; i++ {
		p, err := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/png")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	in := input("ORD-1", "SKU-BAG-BLK", 5, goodBody)
	in.PhotoIDs = &ids
	if _, err := f.svc.Create(ctx, caller(buyer), "BAG-1", in); !errors.Is(err, domain.ErrInvalidReview) {
		t.Fatalf("11 photos: %v", err)
	}
	ten := ids[:10]
	in.PhotoIDs = &ten
	if _, err := f.svc.Create(ctx, caller(buyer), "BAG-1", in); err != nil {
		t.Fatalf("10 photos: %v", err)
	}
}

func TestUpdateAndDelete_OnlyTheAuthor(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	r, err := f.svc.Create(ctx, caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 5, goodBody))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Update(ctx, caller(stranger), r.ID, domain.ReviewInput{Rating: ptr(1)}); !errors.Is(err, ports.ErrReviewNotFound) {
		t.Fatalf("stranger edit: %v", err)
	}
	if err := f.svc.Delete(ctx, caller(stranger), r.ID); !errors.Is(err, ports.ErrReviewNotFound) {
		t.Fatalf("stranger delete: %v", err)
	}
	updated, err := f.svc.Update(ctx, caller(buyer), r.ID, domain.ReviewInput{Rating: ptr(3)})
	if err != nil || updated.Rating != 3 || updated.Body != goodBody {
		t.Fatalf("author edit: %+v %v", updated, err)
	}
	if err := f.svc.Delete(ctx, caller(buyer), r.ID); err != nil {
		t.Fatal(err)
	}
	out, _ := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{})
	if out.Summary.Count != 0 || out.Summary.Average != nil {
		t.Fatalf("summary after delete: %+v", out.Summary)
	}
	// The line can be reviewed again.
	if _, err := f.svc.Create(ctx, caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 4, goodBody)); err != nil {
		t.Fatalf("re-create: %v", err)
	}
}

func TestHide_LeavesSummaryAndLocksEdits(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	r, _ := f.svc.Create(ctx, caller(buyer), "BAG-1", input("ORD-1", "SKU-BAG-BLK", 1, goodBody))
	if _, err := f.svc.SetHidden(ctx, r.ID, true, "", "mgr-1"); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("hide without reason: %v", err)
	}
	hidden, err := f.svc.SetHidden(ctx, r.ID, true, "개인정보 노출", "mgr-1")
	if err != nil || !hidden.Hidden || hidden.HiddenBy != "mgr-1" {
		t.Fatalf("hide: %+v %v", hidden, err)
	}
	out, _ := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{})
	if out.Summary.Count != 0 || len(out.Reviews) != 0 {
		t.Fatalf("hidden review still public: %+v", out)
	}
	mine, _ := f.svc.Mine(ctx, caller(buyer), "")
	if len(mine) != 1 || !mine[0].Hidden || mine[0].HiddenReason != "개인정보 노출" || mine[0].HiddenBy != "" {
		t.Fatalf("author view of hidden review: %+v", mine)
	}
	if _, err := f.svc.Update(ctx, caller(buyer), r.ID, domain.ReviewInput{Rating: ptr(5)}); !errors.Is(err, service.ErrReviewHidden) {
		t.Fatalf("edit hidden: %v", err)
	}
	if _, err := f.svc.SetHidden(ctx, r.ID, false, "", "mgr-1"); err != nil {
		t.Fatal(err)
	}
	out, _ = f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{})
	if out.Summary.Count != 1 {
		t.Fatalf("unhide: %+v", out.Summary)
	}
}

func TestUserDeleted_RemovesReviewsRatingsAndPhotos(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	p, _ := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/png")
	orphan, _ := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/png")
	in := input("ORD-1", "SKU-BAG-BLK", 5, goodBody)
	in.PhotoIDs = &[]string{p.ID}
	if _, err := f.svc.Create(ctx, caller(buyer), "BAG-1", in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, caller(stranger), "BAG-1", input("ORD-2", "SKU-BAG-BLK", 3, goodBody)); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(events.UserDeletedEvent{EventType: events.UserDeleted, UserID: buyer})
	if err := f.svc.HandleUserDeleted(ctx, events.UserDeleted, payload); err != nil {
		t.Fatal(err)
	}
	out, _ := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{})
	if out.Summary.Count != 1 || *out.Summary.Average != 3 || out.Summary.PhotoCount != 0 {
		t.Fatalf("summary keeps the deleted account: %+v", out.Summary)
	}
	if mine, _ := f.svc.Mine(ctx, caller(buyer), ""); len(mine) != 0 {
		t.Fatalf("deleted account still has reviews")
	}
	for _, key := range []string{"reviews/" + p.ID + ".jpg", "reviews/" + orphan.ID + ".w400.jpg"} {
		if _, ok := f.objects.objects[key]; ok {
			t.Fatalf("object %s survived account deletion", key)
		}
	}
}

func TestSweepOrphanPhotos(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	old, _ := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/png")
	f.now = f.now.Add(25 * time.Hour)
	fresh, _ := f.svc.UploadPhoto(ctx, caller(buyer), pngBytes(t), "image/png")
	if err := f.svc.SweepOrphanPhotos(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.objects.objects["reviews/"+old.ID+".jpg"]; ok {
		t.Fatalf("old orphan kept")
	}
	if _, ok := f.objects.objects["reviews/"+fresh.ID+".jpg"]; !ok {
		t.Fatalf("fresh upload swept")
	}
}

func TestProductReviews_SortFilterPage(t *testing.T) {
	f := newReviewFixture(t)
	ctx := context.Background()
	f.orders.orders["ORD-3"] = &ports.ReviewOrder{ID: "ORD-3", CustomerID: buyer, Status: "fulfilled", Lines: []ports.OrderLine{{SkuID: "SKU-BAG-BEI"}}}
	for _, c := range []struct {
		order, sku string
		rating     int
	}{{"ORD-1", "SKU-BAG-BLK", 2}, {"ORD-3", "SKU-BAG-BEI", 5}, {"ORD-OLD", "SKU-BAG-BEI", 4}} {
		if _, err := f.svc.Create(ctx, caller(buyer), "BAG-1", input(c.order, c.sku, c.rating, goodBody)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct ULID timestamps
	}
	out, err := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{Sort: domain.ReviewSortRatingDesc, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Reviews) != 2 || out.Reviews[0].Rating != 5 || out.Reviews[1].Rating != 4 || out.NextCursor == "" {
		t.Fatalf("page 1: %+v", out)
	}
	next, err := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{Sort: domain.ReviewSortRatingDesc, Limit: 2, Cursor: out.NextCursor})
	if err != nil || len(next.Reviews) != 1 || next.Reviews[0].Rating != 2 || next.NextCursor != "" {
		t.Fatalf("page 2: %+v %v", next, err)
	}
	only, _ := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{Rating: 4})
	if len(only.Reviews) != 1 || only.Summary.Count != 3 {
		t.Fatalf("rating filter: %+v", only)
	}
	if _, err := f.svc.ProductReviews(ctx, "BAG-1", domain.ReviewQuery{Sort: "best"}); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("bad sort: %v", err)
	}
	if _, err := f.svc.ProductReviews(ctx, "DRAFT-1", domain.ReviewQuery{}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("draft product: %v", err)
	}
}
