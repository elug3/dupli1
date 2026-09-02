package natspublisher

import (
	"context"
	"testing"
	"time"
)

func TestFlushContextUsesExistingDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
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
	// test harness: no HTTP request context
	ctx := t.Context()

	flushCtx, cancel := flushContext(ctx)
	defer cancel()

	_, ok := flushCtx.Deadline()
	if !ok {
		t.Fatal("expected flush context to have a deadline")
	}
}

func TestPublish_NilPublisherIsNoop(t *testing.T) {
	var p *Publisher
	if err := p.Publish(context.Background(), "subject", map[string]string{"a": "b"}); err != nil {
		t.Fatalf("Publish on nil *Publisher: %v", err)
	}
}

func TestPublish_NilConnIsNoopRegardlessOfContext(t *testing.T) {
	// A Publisher with no live connection (NATS not configured) is a
	// deliberate no-op even for an already-cancelled context — it never
	// gets far enough to check ctx.Err().
	p := &Publisher{conn: nil}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := p.Publish(ctx, "subject", struct{}{}); err != nil {
		t.Fatalf("Publish with nil conn: %v", err)
	}
}
