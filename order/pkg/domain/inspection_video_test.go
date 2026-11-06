package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func TestInspectionVideoEditable(t *testing.T) {
	for status, want := range map[domain.OrderStatus]error{
		domain.StatusPending:   domain.ErrInvalidTransition,
		domain.StatusPaid:      nil,
		domain.StatusConfirmed: nil,
		domain.StatusInTransit: nil,
		domain.StatusDelivered: nil,
		domain.StatusDisputed:  nil,
		domain.StatusFulfilled: domain.ErrInspectionVideoLocked,
		domain.StatusCanceled:  domain.ErrInspectionVideoLocked,
	} {
		err := domain.InspectionVideoEditable(status)
		if (want == nil && err != nil) || (want != nil && !errors.Is(err, want)) {
			t.Errorf("%s: error = %v, want %v", status, err, want)
		}
	}
}

func TestNormalizeInspectionVideoContentType(t *testing.T) {
	for in, want := range map[string]string{
		"video/mp4":              "video/mp4",
		"Video/QuickTime":        "video/quicktime",
		"video/webm; codecs=vp9": "video/webm",
	} {
		got, err := domain.NormalizeInspectionVideoContentType(in)
		if err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "image/jpeg", "application/octet-stream", "video/x-msvideo"} {
		if _, err := domain.NormalizeInspectionVideoContentType(in); !errors.Is(err, domain.ErrInvalidInspectionVideo) {
			t.Errorf("%q: error = %v, want ErrInvalidInspectionVideo", in, err)
		}
	}
}

func TestInspectionVideoVisibleToCustomerOnlyOnceShipped(t *testing.T) {
	o := &domain.Order{InspectionVideo: &domain.InspectionVideo{ObjectKey: "k"}}
	if o.InspectionVideoVisibleToCustomer() {
		t.Fatal("visible before shipping")
	}
	now := time.Now()
	o.ShippedAt = &now
	if !o.InspectionVideoVisibleToCustomer() {
		t.Fatal("not visible after shipping")
	}
	o.InspectionVideo = nil
	if o.InspectionVideoVisibleToCustomer() {
		t.Fatal("visible with no video")
	}
}
