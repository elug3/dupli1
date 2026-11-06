package pg

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

// The video sits in its own table: a whole-order save (every status change
// goes through one) must not drop it, and the status lock is re-checked
// under the row lock.
func TestSetInspectionVideo(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_inspection_video_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := t.Context()
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

	order, err := domain.NewOrder("ord-video-1", "cust-1", "res-video-1", []domain.OrderItem{{
		SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}
	video := domain.InspectionVideo{ObjectKey: "orders/ord-video-1/a.mp4", ContentType: "video/mp4", SizeBytes: 10, UploadedBy: "staff-1", UploadedAt: now}

	if _, err := repo.SetInspectionVideo(ctx, order.ID, video); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("pending order: error = %v, want ErrInvalidTransition", err)
	}
	if _, err := repo.SetInspectionVideo(ctx, "ord-missing", video); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("missing order: error = %v, want ErrNotFound", err)
	}

	if err := order.MarkPaid("pay-1", order.TotalWon, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := order.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save confirmed: %v", err)
	}
	previous, err := repo.SetInspectionVideo(ctx, order.ID, video)
	if err != nil || previous != "" {
		t.Fatalf("first SetInspectionVideo = %q, %v", previous, err)
	}
	replacement := video
	replacement.ObjectKey = "orders/ord-video-1/b.mov"
	replacement.ContentType = "video/quicktime"
	previous, err = repo.SetInspectionVideo(ctx, order.ID, replacement)
	if err != nil || previous != video.ObjectKey {
		t.Fatalf("replacement SetInspectionVideo = %q, %v; want previous %q", previous, err, video.ObjectKey)
	}

	// A status change saves the whole order row; the video must survive it.
	order.InspectionVideo = nil
	if err := order.Fulfill(now); err == nil {
		t.Fatal("test setup: a confirmed order should not fulfil directly")
	}
	order.Status = domain.StatusFulfilled
	order.UpdatedAt = now.Add(time.Hour)
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save fulfilled: %v", err)
	}
	loaded, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.InspectionVideo == nil || loaded.InspectionVideo.ObjectKey != replacement.ObjectKey ||
		loaded.InspectionVideo.ContentType != "video/quicktime" || loaded.InspectionVideo.UploadedBy != "staff-1" ||
		!loaded.InspectionVideo.UploadedAt.Equal(now) {
		t.Fatalf("loaded video = %+v", loaded.InspectionVideo)
	}
	all, err := repo.ListByCustomer(ctx, "cust-1")
	if err != nil || len(all) != 1 || all[0].InspectionVideo == nil {
		t.Fatalf("ListByCustomer video = %+v, %v", all, err)
	}
	everything, err := repo.ListAll(ctx)
	if err != nil || len(everything) != 1 || everything[0].InspectionVideo == nil {
		t.Fatalf("ListAll video = %+v, %v", everything, err)
	}

	if _, err := repo.SetInspectionVideo(ctx, order.ID, video); !errors.Is(err, domain.ErrInspectionVideoLocked) {
		t.Fatalf("fulfilled order: error = %v, want ErrInspectionVideoLocked", err)
	}
}
