package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

type stubOrders struct{}

// Every order is fulfilled, owned by cust-1 and holds SKU-1.
func (stubOrders) Order(_ context.Context, bearer, id string) (*ports.ReviewOrder, error) {
	if id == "ORD-OTHER" {
		return nil, ports.ErrOrderNotReadable
	}
	return &ports.ReviewOrder{ID: id, CustomerID: "cust-1", Status: "fulfilled", Lines: []ports.OrderLine{{SkuID: "SKU-1"}}}, nil
}

type stubProfiles struct{}

func (stubProfiles) DisplayName(context.Context, string) (string, error) { return "박서준", nil }

type stubObjects struct{}

func (stubObjects) Upload(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	_, _ = io.Copy(io.Discard, r)
	return "https://img.test/" + key, nil
}
func (stubObjects) PublicURL(key string) string          { return "https://img.test/" + key }
func (stubObjects) Delete(context.Context, string) error { return nil }

func newReviewMux(t *testing.T) *http.ServeMux {
	t.Helper()
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BAG-1", Name: "Tote", Category: "bags", Status: "active"}}
	store.Variants = []domain.Variant{{SkuID: "SKU-1", SKU: "DUP_TOTE_BLK_OS", ProductID: "BAG-1", Color: "Black", Status: "active"}}
	reviews := service.NewReviewService(memory.NewReviewStore(store), store).
		WithObjects(stubObjects{}).
		WithGateway(stubOrders{}, stubProfiles{})
	h := handler.NewHandler(service.NewProductSearchService(store, nil), nil, nil, nil).WithReviewService(reviews)

	validator := authjwt.NewHMACValidator(accessControlSecret)
	mux := http.NewServeMux()
	h.RegisterReviewRoutes(mux, handler.ReviewMiddleware{
		Auth:      func(next http.Handler) http.Handler { return middleware.RequireAuth(validator, next) },
		Moderator: middleware.RequireAnyPermission(permissions.ProductReviewModerate),
	})
	return mux
}

func decodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return out
}

func TestReviewRoutes_WriteReadModerate(t *testing.T) {
	mux := newReviewMux(t)
	shopper := makeAccessToken(t, "cust-1", nil)
	moderator := makeAccessToken(t, "mgr-1", []string{permissions.ProductReviewModerate})
	body := map[string]any{"orderId": "ORD-1", "skuId": "SKU-1", "rating": 4, "body": "튼튼하고 수납이 넉넉해요."}

	if w := serve(t, mux, http.MethodPost, "/api/v1/products/BAG-1/reviews", "", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous create: %d", w.Code)
	}
	w := serve(t, mux, http.MethodPost, "/api/v1/products/BAG-1/reviews", shopper, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := decodeMap(t, w)
	if created["author"] != "박**" || created["orderId"] != "ORD-1" {
		t.Fatalf("created: %v", created)
	}
	id := created["id"].(string)

	w = serve(t, mux, http.MethodPost, "/api/v1/products/BAG-1/reviews", shopper, body)
	if got := decodeMap(t, w); w.Code != http.StatusConflict || got["reason"] != "review_exists" || got["reviewId"] != id {
		t.Fatalf("duplicate: %d %v", w.Code, got)
	}
	other := map[string]any{"orderId": "ORD-OTHER", "skuId": "SKU-1", "rating": 4, "body": "튼튼하고 수납이 넉넉해요."}
	w = serve(t, mux, http.MethodPost, "/api/v1/products/BAG-1/reviews", shopper, other)
	if got := decodeMap(t, w); w.Code != http.StatusForbidden || got["reason"] != "not_reviewable" {
		t.Fatalf("someone else's order: %d %v", w.Code, got)
	}
	bad := map[string]any{"orderId": "ORD-1", "skuId": "SKU-1", "rating": 4, "body": "짧아요"}
	w = serve(t, mux, http.MethodPost, "/api/v1/products/BAG-1/reviews", shopper, bad)
	if got := decodeMap(t, w); w.Code != http.StatusBadRequest || got["reason"] != "invalid_review" {
		t.Fatalf("short body: %d %v", w.Code, got)
	}

	w = serve(t, mux, http.MethodGet, "/api/v1/products/BAG-1/reviews?sort=rating_desc", "", nil)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("public list: %d %v", w.Code, w.Header())
	}
	list := decodeMap(t, w)
	if list["summary"].(map[string]any)["count"].(float64) != 1 {
		t.Fatalf("summary: %v", list["summary"])
	}
	if _, leaked := list["reviews"].([]any)[0].(map[string]any)["orderId"]; leaked {
		t.Fatalf("public review carries orderId")
	}
	if w := serve(t, mux, http.MethodGet, "/api/v1/products/NOPE/reviews", "", nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown product: %d", w.Code)
	}

	if w := serve(t, mux, http.MethodGet, "/api/v1/products/reviews/me?orderId=ORD-1", shopper, nil); w.Code != http.StatusOK || len(decodeMap(t, w)["reviews"].([]any)) != 1 {
		t.Fatalf("mine: %d %s", w.Code, w.Body.String())
	}

	if w := serve(t, mux, http.MethodGet, "/api/v1/products/reviews", shopper, nil); w.Code != http.StatusForbidden {
		t.Fatalf("shopper on moderation list: %d", w.Code)
	}
	if w := serve(t, mux, http.MethodPost, "/api/v1/products/reviews/by-id/"+id+"/hide", moderator, map[string]any{"reason": "광고"}); w.Code != http.StatusOK {
		t.Fatalf("hide: %d %s", w.Code, w.Body.String())
	}
	w = serve(t, mux, http.MethodGet, "/api/v1/products/reviews?status=hidden", moderator, nil)
	hidden := decodeMap(t, w)["reviews"].([]any)
	if w.Code != http.StatusOK || len(hidden) != 1 || hidden[0].(map[string]any)["customerId"] != "cust-1" {
		t.Fatalf("moderation list: %d %v", w.Code, hidden)
	}
	w = serve(t, mux, http.MethodPatch, "/api/v1/products/reviews/by-id/"+id, shopper, map[string]any{"rating": 5})
	if got := decodeMap(t, w); w.Code != http.StatusConflict || got["reason"] != "review_hidden" {
		t.Fatalf("edit hidden: %d %v", w.Code, got)
	}
	if w := serve(t, mux, http.MethodDelete, "/api/v1/products/reviews/by-id/"+id, shopper, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
}

func TestReviewRoutes_PhotoUpload(t *testing.T) {
	mux := newReviewMux(t)
	shopper := makeAccessToken(t, "cust-1", nil)

	var img bytes.Buffer
	_ = png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	upload := func(contentType string, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		header := make(map[string][]string)
		header["Content-Disposition"] = []string{`form-data; name="photo"; filename="a.png"`}
		header["Content-Type"] = []string{contentType}
		part, _ := mw.CreatePart(header)
		_, _ = part.Write(data)
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products/reviews/photos", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", bearer(shopper))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	w := upload("image/png", img.Bytes())
	if w.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	got := decodeMap(t, w)
	if got["photoId"] == "" || got["thumbUrl"] == "" {
		t.Fatalf("upload response: %v", got)
	}
	if w := upload("image/png", []byte("nope")); w.Code != http.StatusBadRequest || decodeMap(t, w)["reason"] != "invalid_photo" {
		t.Fatalf("garbage: %d %s", w.Code, w.Body.String())
	}
}
