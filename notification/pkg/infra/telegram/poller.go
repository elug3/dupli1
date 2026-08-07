package telegram

import (
	"context"
	"log"
	"time"
)

const (
	pollTimeoutSec   = 30
	pollErrorBackoff = 5 * time.Second
)

// DrainUpdates fetches and processes pending updates once (used after webhook setup).
func DrainUpdates(ctx context.Context, client *Client, processor *UpdateProcessor) error {
	if client == nil || !client.Enabled() || processor == nil {
		return nil
	}
	var offset int64
	for {
		updates, err := client.GetUpdates(ctx, offset, 0)
		if err != nil {
			return err
		}
		if len(updates) == 0 {
			return nil
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := processor.Handle(ctx, update); err != nil {
				log.Printf("telegram drain update: %v", err)
			}
		}
	}
}

// RunPoller long-polls Telegram when webhook mode is not configured.
func RunPoller(ctx context.Context, client *Client, processor *UpdateProcessor) {
	if client == nil || !client.Enabled() || processor == nil {
		return
	}

	pollClient := *client
	pollHTTP := *client.httpClient
	pollHTTP.Timeout = pollTimeoutSec*time.Second + 10*time.Second
	pollClient.httpClient = &pollHTTP

	if err := pollClient.DeleteWebhook(ctx); err != nil {
		log.Printf("telegram deleteWebhook: %v", err)
	} else {
		log.Println("telegram polling mode started")
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			log.Println("telegram poller stopped")
			return
		default:
		}

		updates, err := pollClient.GetUpdates(ctx, offset, pollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram getUpdates: %v", err)
			sleep(ctx, pollErrorBackoff)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := processor.Handle(ctx, update); err != nil {
				log.Printf("telegram handle update: %v", err)
			}
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
