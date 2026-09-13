package ports

// OrderEventSink receives order changes for fan-out to live clients (SSE).
// Implemented by pkg/stream.Hub; nil-safe use is the caller's responsibility.
type OrderEventSink interface {
	// PublishOrderEvent delivers an enriched order snapshot. id positions the
	// event for reconnect replay; eventType is the SSE event name.
	PublishOrderEvent(id, eventType string, payload []byte)
	// PublishReset asks live clients to reload from REST because the service
	// could not build an authoritative snapshot.
	PublishReset(id string)
}
