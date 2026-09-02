package nats

import (
	natsgo "github.com/nats-io/nats.go"

	"github.com/elug3/dupli1/shared/pkg/natspublisher"
)

// Publisher publishes JSON-encoded events to NATS subjects.
// Alias of the shared implementation — see shared/pkg/natspublisher.
type Publisher = natspublisher.Publisher

// NewPublisher connects to NATS and returns an event publisher.
func NewPublisher(url string, opts ...natsgo.Option) (*Publisher, error) {
	return natspublisher.New(url, opts...)
}
