package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeStore struct {
	pending   []Message
	published []int64
	attempts  map[int64]string
	failMark  bool
}

func newFakeStore(msgs ...Message) *fakeStore {
	return &fakeStore{pending: msgs, attempts: map[int64]string{}}
}

func (s *fakeStore) ListPendingOutbox(_ context.Context, limit int) ([]Message, error) {
	var out []Message
	for _, m := range s.pending {
		if len(out) >= limit {
			break
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *fakeStore) MarkOutboxPublished(_ context.Context, id int64) error {
	if s.failMark {
		return errors.New("mark published failed")
	}
	s.published = append(s.published, id)
	s.pending = removeMessage(s.pending, id)
	return nil
}

func (s *fakeStore) RecordOutboxAttempt(_ context.Context, id int64, errMsg string) error {
	s.attempts[id] = errMsg
	return nil
}

func removeMessage(msgs []Message, id int64) []Message {
	out := msgs[:0]
	for _, m := range msgs {
		if m.ID != id {
			out = append(out, m)
		}
	}
	return out
}

type fakePublisher struct {
	failSubjects map[string]bool
	published    []struct {
		subject string
		payload json.RawMessage
	}
}

func (p *fakePublisher) Publish(_ context.Context, subject string, event any) error {
	if p.failSubjects[subject] {
		return errors.New("publish failed: " + subject)
	}
	raw, ok := event.(json.RawMessage)
	if !ok {
		return errors.New("expected json.RawMessage event")
	}
	p.published = append(p.published, struct {
		subject string
		payload json.RawMessage
	}{subject, raw})
	return nil
}

func TestDrain_NoPublisherMarksAllPublished(t *testing.T) {
	store := newFakeStore(
		Message{ID: 1, Subject: "a.created", Payload: []byte(`{}`)},
		Message{ID: 2, Subject: "b.created", Payload: []byte(`{}`)},
	)
	d := NewDrainer(store, nil, "test outbox drain")

	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(store.published) != 2 {
		t.Fatalf("published = %v, want 2 rows marked", store.published)
	}
}

func TestDrain_PublishesAndMarksPublished(t *testing.T) {
	store := newFakeStore(Message{ID: 1, Subject: "order.created", Payload: []byte(`{"id":"ord_1"}`)})
	pub := &fakePublisher{failSubjects: map[string]bool{}}
	d := NewDrainer(store, pub, "test outbox drain")

	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(pub.published) != 1 || pub.published[0].subject != "order.created" {
		t.Fatalf("published = %+v, want one order.created", pub.published)
	}
	if string(pub.published[0].payload) != `{"id":"ord_1"}` {
		t.Fatalf("payload = %s, want passthrough of stored bytes", pub.published[0].payload)
	}
	if len(store.published) != 1 || store.published[0] != 1 {
		t.Fatalf("store.published = %v, want [1]", store.published)
	}
}

func TestDrain_RecordsAttemptOnPublishFailureAndContinues(t *testing.T) {
	store := newFakeStore(
		Message{ID: 1, Subject: "order.created", Payload: []byte(`{}`)},
		Message{ID: 2, Subject: "order.paid", Payload: []byte(`{}`)},
	)
	pub := &fakePublisher{failSubjects: map[string]bool{"order.created": true}}
	d := NewDrainer(store, pub, "test outbox drain")

	err := d.Drain(context.Background())
	if err == nil {
		t.Fatal("expected Drain to return the first publish error")
	}
	if store.attempts[1] == "" {
		t.Fatal("expected failed message to have a recorded attempt")
	}
	if len(store.published) != 1 || store.published[0] != 2 {
		t.Fatalf("published = %v, want only id 2 (the message that succeeded)", store.published)
	}
}

func TestDrain_NoPendingMessagesIsNoop(t *testing.T) {
	store := newFakeStore()
	pub := &fakePublisher{}
	d := NewDrainer(store, pub, "test outbox drain")

	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(pub.published) != 0 {
		t.Fatalf("published = %v, want none", pub.published)
	}
}

func TestTryDrain_DoesNotPanicOnError(t *testing.T) {
	store := newFakeStore(Message{ID: 1, Subject: "x", Payload: []byte(`{}`)})
	pub := &fakePublisher{failSubjects: map[string]bool{"x": true}}
	d := NewDrainer(store, pub, "test outbox drain")

	// Must not panic or block; errors are logged, not returned.
	d.TryDrain(context.Background())
}
