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

// RunPoller long-polls Telegram for updates and handles bot commands.
// It returns when ctx is cancelled.
func RunPoller(ctx context.Context, client *Client) {
	if client == nil || !client.Enabled() {
		return
	}

	pollClient := *client
	pollHTTP := *client.httpClient
	pollHTTP.Timeout = pollTimeoutSec*time.Second + 10*time.Second
	pollClient.httpClient = &pollHTTP

	if err := pollClient.DeleteWebhook(ctx); err != nil {
		log.Printf("telegram deleteWebhook: %v", err)
	} else {
		log.Println("telegram command poller started")
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			log.Println("telegram command poller stopped")
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
			if update.Message == nil {
				continue
			}
			if err := HandleMessage(ctx, client, update.Message); err != nil {
				log.Printf("telegram handle message: %v", err)
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
