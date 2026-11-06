package memory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/elug3/dupli1/order/pkg/ports"
)

// VideoStore is an in-process ports.InspectionVideoStore for tests. Bootstrap
// never falls back to it: a video kept only in memory would vanish on the
// next deploy while the order still pointed at it.
type VideoStore struct {
	mu      sync.Mutex
	objects map[string]videoObject
}

type videoObject struct {
	data        []byte
	contentType string
	modTime     time.Time
}

func NewVideoStore() *VideoStore {
	return &VideoStore{objects: make(map[string]videoObject)}
}

func (s *VideoStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return fmt.Errorf("video store: read %d bytes, want %d", len(data), size)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = videoObject{data: data, contentType: contentType, modTime: time.Now().UTC()}
	return nil
}

func (s *VideoStore) Open(ctx context.Context, key string) (ports.InspectionVideoObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("inspection video object %s: %w", key, ports.ErrNotFound)
	}
	return &openVideo{Reader: bytes.NewReader(obj.data), size: int64(len(obj.data)), modTime: obj.modTime}, nil
}

func (s *VideoStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

// Keys lists the stored object keys (test helper).
func (s *VideoStore) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	return keys
}

type openVideo struct {
	*bytes.Reader
	size    int64
	modTime time.Time
}

func (o *openVideo) Close() error       { return nil }
func (o *openVideo) Size() int64        { return o.size }
func (o *openVideo) ModTime() time.Time { return o.modTime }
