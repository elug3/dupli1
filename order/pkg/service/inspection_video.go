package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

// DefaultInspectionVideoMaxBytes caps one upload when
// DUPLI1_ORDER_INSPECTION_VIDEO_MAX_BYTES is not set. 100 MB is what the
// Cloudflare tunnel in front of manage.dupli1.com lets through in one request.
const DefaultInspectionVideoMaxBytes int64 = 100_000_000

// WithInspectionVideos sets where inspection videos are stored and how large
// one may be (maxBytes <= 0 keeps the default).
func (s *Service) WithInspectionVideos(store ports.InspectionVideoStore, maxBytes int64) *Service {
	s.videos = store
	if maxBytes > 0 {
		s.videoMaxBytes = maxBytes
	}
	return s
}

// InspectionVideoMaxBytes is the largest upload accepted.
func (s *Service) InspectionVideoMaxBytes() int64 {
	if s.videoMaxBytes > 0 {
		return s.videoMaxBytes
	}
	return DefaultInspectionVideoMaxBytes
}

// UploadInspectionVideoInput is one upload: body holds exactly SizeBytes.
type UploadInspectionVideoInput struct {
	OrderID     string
	UploadedBy  string
	ContentType string
	SizeBytes   int64
	Body        io.Reader
}

// UploadInspectionVideo stores a staff member's inspection film and attaches
// it to the order, replacing any earlier one. It is refused before payment
// and once the order is fulfilled or canceled. The new object is written
// under a fresh key first and the order points at it only after that
// succeeds, so a failed upload never leaves the order without its old video.
func (s *Service) UploadInspectionVideo(ctx context.Context, in UploadInspectionVideoInput) (*domain.Order, error) {
	if s.videos == nil {
		return nil, ports.ErrInspectionVideoStorageUnavailable
	}
	orderID := strings.TrimSpace(in.OrderID)
	uploadedBy := strings.TrimSpace(in.UploadedBy)
	contentType, err := domain.NormalizeInspectionVideoContentType(in.ContentType)
	if err != nil {
		return nil, err
	}
	if in.SizeBytes <= 0 {
		return nil, fmt.Errorf("%w: the video is empty", domain.ErrInvalidInspectionVideo)
	}
	if in.SizeBytes > s.InspectionVideoMaxBytes() {
		return nil, fmt.Errorf("%w: the video is %d bytes; the limit is %d", domain.ErrInvalidInspectionVideo, in.SizeBytes, s.InspectionVideoMaxBytes())
	}

	order, err := s.repo.Get(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// Checked again under the row lock in SetInspectionVideo; this one only
	// saves streaming a whole video into storage for nothing.
	if err := domain.InspectionVideoEditable(order.Status); err != nil {
		return nil, err
	}

	now := s.now().UTC()
	key := fmt.Sprintf("orders/%s/inspection-%d%s", order.ID, now.UnixNano(), domain.InspectionVideoContentTypes[contentType])
	if err := s.videos.Put(ctx, key, in.Body, in.SizeBytes, contentType); err != nil {
		return nil, err
	}

	previous, err := s.repo.SetInspectionVideo(ctx, order.ID, domain.InspectionVideo{
		ObjectKey:   key,
		ContentType: contentType,
		SizeBytes:   in.SizeBytes,
		UploadedBy:  uploadedBy,
		UploadedAt:  now,
	})
	if err != nil {
		// The order did not take it (closed meanwhile, or the database
		// failed): the object is orphaned, so drop it.
		s.deleteVideoObject(context.WithoutCancel(ctx), key)
		return nil, err
	}
	if previous != "" && previous != key {
		s.deleteVideoObject(context.WithoutCancel(ctx), previous)
	}
	return s.GetOrder(ctx, order.ID)
}

func (s *Service) deleteVideoObject(ctx context.Context, key string) {
	if err := s.videos.Delete(ctx, key); err != nil {
		log.Printf("order: inspection video %s left in storage: %v", key, err)
	}
}

// InspectionVideoAccess says who is asking to watch.
type InspectionVideoAccess struct {
	UserID string
	// Staff readers (order.read.all and up) see the video as soon as it is
	// attached; the order's own customer only from shipping on.
	Staff bool
}

// ErrInspectionVideoForbidden is a customer asking for someone else's order.
var ErrInspectionVideoForbidden = errors.New("forbidden: you do not own this order")

// OpenInspectionVideo returns the order's video for streaming. The caller
// closes the object. ErrNotFound when there is none (yet) for this reader.
func (s *Service) OpenInspectionVideo(ctx context.Context, orderID string, access InspectionVideoAccess) (*domain.InspectionVideo, ports.InspectionVideoObject, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return nil, nil, err
	}
	if !access.Staff {
		if order.CustomerID != access.UserID {
			return nil, nil, ErrInspectionVideoForbidden
		}
		if !order.InspectionVideoVisibleToCustomer() {
			return nil, nil, fmt.Errorf("inspection video: %w", ports.ErrNotFound)
		}
	}
	if !order.HasInspectionVideo() {
		return nil, nil, fmt.Errorf("inspection video: %w", ports.ErrNotFound)
	}
	if s.videos == nil {
		return nil, nil, ports.ErrInspectionVideoStorageUnavailable
	}
	obj, err := s.videos.Open(ctx, order.InspectionVideo.ObjectKey)
	if err != nil {
		return nil, nil, err
	}
	video := *order.InspectionVideo
	return &video, obj, nil
}
