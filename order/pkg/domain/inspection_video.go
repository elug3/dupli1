package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// The inspection video (검수영상) is the film staff record of each item before
// it ships: the evidence for disputes and damage claims, and something the
// shopper can watch once the parcel is on its way. See
// docs/order-inspection-video.md.
var (
	// ErrInspectionVideoRequired refuses a ship while no video is attached.
	ErrInspectionVideoRequired = errors.New("inspection video is required before shipping")
	// ErrInspectionVideoLocked refuses to replace the video once the order is
	// fulfilled (receipt confirmed, auto-fulfilled or closed by a manager) or
	// canceled: from then on it is the record of what was sent.
	ErrInspectionVideoLocked = errors.New("inspection video can no longer be changed")
	// ErrInvalidInspectionVideo is an upload that is not an accepted video.
	ErrInvalidInspectionVideo = errors.New("invalid inspection video")
)

// InspectionVideo is the stored film's metadata. The object itself lives in
// object storage (MinIO / any S3 API) under ObjectKey, which never leaves the
// service: shoppers and staff stream it through
// GET /api/v1/orders/{id}/inspection-video.
type InspectionVideo struct {
	ObjectKey   string    `json:"-"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	UploadedBy  string    `json:"uploaded_by,omitempty"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// InspectionVideoContentTypes maps each accepted upload type to the file
// extension its object key carries.
var InspectionVideoContentTypes = map[string]string{
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
}

// NormalizeInspectionVideoContentType lower-cases the media type and drops
// parameters ("video/mp4; codecs=…" → "video/mp4"), then checks it is one we
// accept.
func NormalizeInspectionVideoContentType(contentType string) (string, error) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if _, ok := InspectionVideoContentTypes[mediaType]; !ok {
		return "", fmt.Errorf("%w: content type %q is not one of video/mp4, video/quicktime, video/webm", ErrInvalidInspectionVideo, contentType)
	}
	return mediaType, nil
}

// InspectionVideoEditable reports whether staff may attach or replace the
// video on an order in status. Filming happens once the order is paid, so a
// pending order has nothing to film yet; a fulfilled or canceled order is
// closed and its video locked.
func InspectionVideoEditable(status OrderStatus) error {
	switch status {
	case StatusPaid, StatusConfirmed, StatusInTransit, StatusDelivered, StatusDisputed:
		return nil
	case StatusFulfilled, StatusCanceled:
		return ErrInspectionVideoLocked
	default:
		return fmt.Errorf("%w: an inspection video can be attached once the order is paid", ErrInvalidTransition)
	}
}

// HasInspectionVideo reports whether a video is attached.
func (o *Order) HasInspectionVideo() bool {
	return o.InspectionVideo != nil && o.InspectionVideo.ObjectKey != ""
}

// InspectionVideoVisibleToCustomer reports whether the shopper may watch the
// video: from shipping on, which is when it means "this is what we sent".
func (o *Order) InspectionVideoVisibleToCustomer() bool {
	return o.HasInspectionVideo() && o.ShippedAt != nil
}
