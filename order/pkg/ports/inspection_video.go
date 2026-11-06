package ports

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// ErrInspectionVideoStorageUnavailable means no object storage is configured
// (DUPLI1_ORDER_S3_ENDPOINT unset) or it could not be reached.
var ErrInspectionVideoStorageUnavailable = errors.New("inspection video storage unavailable")

// InspectionVideoObject is a stored video opened for reading. It seeks, so
// the handler can answer Range requests (video players seek and resume).
type InspectionVideoObject interface {
	io.ReadSeekCloser
	Size() int64
	ModTime() time.Time
}

// InspectionVideoStore holds the films themselves, in object storage (MinIO
// or any S3 API). The bucket is private: nothing in it is ever addressed by
// the browser directly.
type InspectionVideoStore interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Open(ctx context.Context, key string) (InspectionVideoObject, error)
	Delete(ctx context.Context, key string) error
}

// InspectionVideoRepository records which video belongs to an order. Kept
// apart from Repository.Save so a whole-order save can never drop or
// overwrite the video.
type InspectionVideoRepository interface {
	// SetInspectionVideo attaches video to the order, replacing any earlier
	// one, and returns the replaced object's key ("" when there was none).
	// It locks the order row and re-checks domain.InspectionVideoEditable
	// against its current status, so a concurrent fulfil cannot slip a
	// replacement past the lock. ErrNotFound when the order does not exist.
	SetInspectionVideo(ctx context.Context, orderID string, video domain.InspectionVideo) (previousKey string, err error)
}
