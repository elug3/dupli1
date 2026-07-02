package nats

import (
	"context"
	"testing"
	"time"
)

func TestFlushContextUsesExistingDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	flushCtx, flushCancel := flushContext(ctx)
	defer flushCancel()

	got, ok := flushCtx.Deadline()
	if !ok {
		t.Fatal("expected deadline on flush context")
	}
	if !got.Equal(deadline) {
		t.Fatalf("want deadline %v, got %v", deadline, got)
	}
}

func TestFlushContextAddsDeadlineWhenMissing(t *testing.T) {
	ctx := context.Background()

	flushCtx, cancel := flushContext(ctx)
	defer cancel()

	_, ok := flushCtx.Deadline()
	if !ok {
		t.Fatal("expected flush context to have a deadline")
	}
}
