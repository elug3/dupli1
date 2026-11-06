package service_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

func newVideoSvc(t *testing.T) (*service.Service, *fakeStock, *memory.VideoStore) {
	t.Helper()
	stock := &fakeStock{reservationID: "res-1"}
	videos := memory.NewVideoStore()
	svc := newSvc(stock, &fakeProduct{defaultKRW: 5000}).WithInspectionVideos(videos, 1000)
	return svc, stock, videos
}

func createOrder(t *testing.T, svc *service.Service, customerID string) *domain.Order {
	t.Helper()
	order, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID: customerID,
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	return order
}

func confirmedOrder(t *testing.T, svc *service.Service, customerID string) *domain.Order {
	t.Helper()
	order := createOrder(t, svc, customerID)
	if _, err := svc.MarkOrderPaid(t.Context(), order.ID, "pay-1", order.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	confirmed, err := svc.ConfirmOrder(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("ConfirmOrder: %v", err)
	}
	return confirmed
}

func upload(svc *service.Service, orderID, contentType, body string) (*domain.Order, error) {
	return svc.UploadInspectionVideo(context.Background(), service.UploadInspectionVideoInput{
		OrderID: orderID, UploadedBy: "staff-1", ContentType: contentType,
		SizeBytes: int64(len(body)), Body: strings.NewReader(body),
	})
}

// Shipping is refused until staff attach the 검수영상, and the refusal comes
// before the stock commit, which cannot be undone.
func TestShipOrderRequiresInspectionVideo(t *testing.T) {
	svc, stock, _ := newVideoSvc(t)
	order := confirmedOrder(t, svc, "c-1")

	_, err := svc.ShipOrder(t.Context(), order.ID, "staff-1", testShipTracking())
	if !errors.Is(err, domain.ErrInspectionVideoRequired) {
		t.Fatalf("ShipOrder error = %v, want ErrInspectionVideoRequired", err)
	}
	if stock.committed != "" {
		t.Fatalf("stock committed %q on a refused ship", stock.committed)
	}

	if _, err := upload(svc, order.ID, "video/mp4", "film"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	shipped, err := svc.ShipOrder(t.Context(), order.ID, "staff-1", testShipTracking())
	if err != nil {
		t.Fatalf("ShipOrder after upload: %v", err)
	}
	if shipped.Status != domain.StatusInTransit {
		t.Fatalf("status = %s, want in_transit", shipped.Status)
	}
}

func TestUploadInspectionVideoReplacesEarlierOne(t *testing.T) {
	svc, _, videos := newVideoSvc(t)
	order := confirmedOrder(t, svc, "c-1")

	first, err := upload(svc, order.ID, "video/quicktime", "first")
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if first.InspectionVideo == nil || first.InspectionVideo.ContentType != "video/quicktime" || first.InspectionVideo.SizeBytes != 5 {
		t.Fatalf("first video = %+v", first.InspectionVideo)
	}
	if first.InspectionVideo.UploadedBy != "staff-1" || first.InspectionVideo.UploadedAt.IsZero() {
		t.Fatalf("first video uploader = %+v", first.InspectionVideo)
	}

	second, err := upload(svc, order.ID, "video/mp4; codecs=avc1", "second!")
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if second.InspectionVideo.ContentType != "video/mp4" || second.InspectionVideo.SizeBytes != 7 {
		t.Fatalf("second video = %+v", second.InspectionVideo)
	}
	keys := videos.Keys()
	if len(keys) != 1 || !strings.HasSuffix(keys[0], ".mp4") {
		t.Fatalf("stored keys = %v, want only the replacement", keys)
	}

	_, obj, err := svc.OpenInspectionVideo(t.Context(), order.ID, service.InspectionVideoAccess{Staff: true})
	if err != nil {
		t.Fatalf("OpenInspectionVideo: %v", err)
	}
	defer obj.Close()
	if got, _ := io.ReadAll(obj); string(got) != "second!" {
		t.Fatalf("video body = %q, want the replacement", got)
	}
}

// Once the shopper confirms receipt the video is the record of what was sent.
func TestInspectionVideoLockedOnceFulfilled(t *testing.T) {
	svc, _, _ := newVideoSvc(t)
	order := confirmedOrder(t, svc, "c-1")
	if _, err := upload(svc, order.ID, "video/mp4", "film"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.ShipOrder(t.Context(), order.ID, "staff-1", testShipTracking()); err != nil {
		t.Fatalf("ShipOrder: %v", err)
	}
	if _, err := svc.DeliverOrder(t.Context(), order.ID, "driver-1"); err != nil {
		t.Fatalf("DeliverOrder: %v", err)
	}
	delivered, err := upload(svc, order.ID, "video/mp4", "retake")
	if err != nil {
		t.Fatalf("replace while delivered: %v", err)
	}
	if delivered.InspectionVideoLocked {
		t.Fatal("inspection_video_locked = true on a delivered order")
	}

	fulfilled, err := svc.ConfirmReceipt(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("ConfirmReceipt: %v", err)
	}
	if !fulfilled.InspectionVideoLocked {
		t.Fatal("inspection_video_locked = false on a fulfilled order")
	}
	if fulfilled.InspectionVideo == nil || fulfilled.InspectionVideo.SizeBytes != 6 {
		t.Fatalf("fulfilled order lost its video: %+v", fulfilled.InspectionVideo)
	}
	if _, err := upload(svc, order.ID, "video/mp4", "late"); !errors.Is(err, domain.ErrInspectionVideoLocked) {
		t.Fatalf("upload after receipt error = %v, want ErrInspectionVideoLocked", err)
	}
}

func TestUploadInspectionVideoRefusals(t *testing.T) {
	svc, _, videos := newVideoSvc(t)
	pending := createOrder(t, svc, "c-1")
	if _, err := upload(svc, pending.ID, "video/mp4", "film"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("pending order: error = %v, want ErrInvalidTransition", err)
	}

	order := confirmedOrder(t, svc, "c-2")
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"not a video", "image/jpeg", "jpeg"},
		{"empty", "video/mp4", ""},
		{"over the limit", "video/mp4", strings.Repeat("x", 1001)},
	} {
		if _, err := upload(svc, order.ID, tc.contentType, tc.body); !errors.Is(err, domain.ErrInvalidInspectionVideo) {
			t.Errorf("%s: error = %v, want ErrInvalidInspectionVideo", tc.name, err)
		}
	}
	if keys := videos.Keys(); len(keys) != 0 {
		t.Fatalf("refused uploads stored %v", keys)
	}

	if _, err := upload(svc, "ord_missing", "video/mp4", "film"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("missing order: error = %v, want ErrNotFound", err)
	}
}

func TestUploadInspectionVideoWithoutStorage(t *testing.T) {
	svc := newSvc(&fakeStock{reservationID: "res-1"}, &fakeProduct{defaultKRW: 5000})
	order := confirmedOrder(t, svc, "c-1")
	if _, err := upload(svc, order.ID, "video/mp4", "film"); !errors.Is(err, ports.ErrInspectionVideoStorageUnavailable) {
		t.Fatalf("error = %v, want ErrInspectionVideoStorageUnavailable", err)
	}
}

// Staff watch it as soon as it is attached; the shopper from shipping on, and
// only on their own order.
func TestOpenInspectionVideoAccess(t *testing.T) {
	svc, _, _ := newVideoSvc(t)
	order := confirmedOrder(t, svc, "c-1")
	if _, err := upload(svc, order.ID, "video/mp4", "film"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	owner := service.InspectionVideoAccess{UserID: "c-1"}

	if _, _, err := svc.OpenInspectionVideo(t.Context(), order.ID, owner); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("owner before shipping: error = %v, want ErrNotFound", err)
	}
	_, obj, err := svc.OpenInspectionVideo(t.Context(), order.ID, service.InspectionVideoAccess{UserID: "staff-1", Staff: true})
	if err != nil {
		t.Fatalf("staff before shipping: %v", err)
	}
	obj.Close()

	if _, err := svc.ShipOrder(t.Context(), order.ID, "staff-1", testShipTracking()); err != nil {
		t.Fatalf("ShipOrder: %v", err)
	}
	video, obj, err := svc.OpenInspectionVideo(t.Context(), order.ID, owner)
	if err != nil {
		t.Fatalf("owner after shipping: %v", err)
	}
	obj.Close()
	if video.ContentType != "video/mp4" {
		t.Fatalf("content type = %q", video.ContentType)
	}
	if _, _, err := svc.OpenInspectionVideo(t.Context(), order.ID, service.InspectionVideoAccess{UserID: "c-2"}); !errors.Is(err, service.ErrInspectionVideoForbidden) {
		t.Fatalf("another customer: error = %v, want ErrInspectionVideoForbidden", err)
	}
}
