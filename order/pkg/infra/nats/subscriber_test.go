package nats

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
)

func startServer(t *testing.T) string {
	t.Helper()
	s, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server not ready")
	}
	t.Cleanup(s.Shutdown)
	return s.ClientURL()
}

// Two replicas: a queue-group Subscribe hands each message to one of them,
// SubscribeAll to both — which the live order stream relies on, since a
// console's stream is held by whichever replica it reached.
func TestSubscribeAllReachesEveryReplica(t *testing.T) {
	url := startServer(t)
	var queued, broadcast atomic.Int32
	for i := 0; i < 2; i++ {
		sub, err := NewSubscriber(url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(sub.Close)
		if err := sub.Subscribe(t.Context(), "order.created", func(context.Context, string, []byte) error {
			queued.Add(1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := sub.SubscribeAll(t.Context(), "order.*", func(context.Context, string, []byte) error {
			broadcast.Add(1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := sub.conn.Flush(); err != nil {
			t.Fatal(err)
		}
	}

	pub, err := natsgo.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	if err := pub.Publish("order.created", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	_ = pub.Flush()

	deadline := time.Now().Add(3 * time.Second)
	for broadcast.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // let any extra queued delivery land
	if broadcast.Load() != 2 {
		t.Fatalf("broadcast deliveries = %d, want 2 (one per replica)", broadcast.Load())
	}
	if queued.Load() != 1 {
		t.Fatalf("queue-group deliveries = %d, want 1", queued.Load())
	}
}
