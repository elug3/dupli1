// Package nats publishes support events to the platform bus.
package nats

import (
	"context"
	"time"

	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/natspublisher"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// InquiryPublisher announces escalations over NATS.
//
// Notification subscribes and fans out to the ops chats it already resolves,
// so this service never learns the ops bot's token or which chats staff watch.
type InquiryPublisher struct {
	publisher *natspublisher.Publisher
	manageURL string
	now       func() time.Time
}

func NewInquiryPublisher(publisher *natspublisher.Publisher, manageWebURL string) *InquiryPublisher {
	return &InquiryPublisher{publisher: publisher, manageURL: manageWebURL, now: time.Now}
}

func (p *InquiryPublisher) InquiryOpened(ctx context.Context, in ports.InquiryOpened) error {
	if p == nil || p.publisher == nil {
		return nil
	}
	now := p.now().UTC()
	return p.publisher.Publish(ctx, events.SupportInquiryOpened, events.SupportInquiry{
		InquiryID:    in.InquiryID,
		ChatID:       in.ChatID,
		Topic:        in.Topic,
		Language:     in.Language,
		Username:     in.Username,
		EntryContext: in.EntryContext,
		Excerpt:      in.Excerpt,
		ManageURL:    p.inquiryURL(in.InquiryID),
		AfterHours:   in.AfterHours,
		OpenedAt:     now,
		Occurred:     now,
	})
}

func (p *InquiryPublisher) inquiryURL(inquiryID string) string {
	if p.manageURL == "" || inquiryID == "" {
		return ""
	}
	return p.manageURL + "/support/inquiries/" + inquiryID
}
