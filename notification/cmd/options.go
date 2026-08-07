package main

import (
	"flag"
	"net"
	"os"
	"strconv"
	"time"

	notification "github.com/elug3/dupli1/notification/pkg"
)

type Options = notification.ServerOptions

func ConfigureOptions(fs *flag.FlagSet, args []string) (Options, error) {
	opts := notification.NewServerOptions()
	applyEnv(opts)

	host, port, err := splitAddr(opts.Addr)
	if err != nil {
		return Options{}, err
	}

	var (
		addr                  string
		natsURL               = opts.NATSURL
		databaseConnString    = opts.DatabaseConnString
		jwtSecret             = opts.JWTSecret
		jwksURL               = opts.JWKSURL
		telegramToken         = opts.TelegramToken
		telegramWebhookURL    = opts.TelegramWebhookURL
		telegramWebhookSecret = opts.TelegramWebhookSecret
		allowedUserIDs        = opts.AllowedUserIDs
		orderChatID           = opts.OrderChatID
		productChatID         = opts.ProductChatID
		manageWebURL          = opts.ManageWebURL
		readTimeoutSec        = int(opts.ReadTimeout / time.Second)
		writeTimeoutSec       = int(opts.WriteTimeout / time.Second)
		idleTimeoutSec        = int(opts.IdleTimeout / time.Second)
		shutdownTimeoutSec    = int(opts.ShutdownTimeout / time.Second)
	)

	fs.StringVar(&host, "host", host, "Server host address")
	fs.IntVar(&port, "port", port, "Server port number")
	fs.StringVar(&addr, "addr", "", "Server listen address (overrides host/port)")
	fs.StringVar(&natsURL, "nats-url", natsURL, "NATS server URL")
	fs.StringVar(&databaseConnString, "db-url", databaseConnString, "PostgreSQL connection string")
	fs.StringVar(&jwtSecret, "jwt-secret", jwtSecret, "JWT secret (dev HS256 fallback)")
	fs.StringVar(&jwksURL, "jwks-url", jwksURL, "Auth JWKS URL for RS256 access tokens")
	fs.StringVar(&telegramToken, "telegram-token", telegramToken, "Telegram bot token")
	fs.StringVar(&telegramWebhookURL, "telegram-webhook-url", telegramWebhookURL, "Public Telegram webhook URL")
	fs.StringVar(&telegramWebhookSecret, "telegram-webhook-secret", telegramWebhookSecret, "Telegram webhook secret token")
	fs.StringVar(&allowedUserIDs, "telegram-allowed-user-ids", allowedUserIDs, "Comma-separated Telegram user IDs allowed to use bot commands")
	fs.StringVar(&orderChatID, "telegram-order-chat-id", orderChatID, "Telegram chat ID for order manager alerts")
	fs.StringVar(&productChatID, "telegram-product-chat-id", productChatID, "Telegram chat ID for product manager alerts")
	fs.StringVar(&manageWebURL, "manage-web-url", manageWebURL, "Base URL for manage-web order links in Telegram alerts")
	fs.IntVar(&readTimeoutSec, "read-timeout", readTimeoutSec, "Read timeout in seconds")
	fs.IntVar(&writeTimeoutSec, "write-timeout", writeTimeoutSec, "Write timeout in seconds")
	fs.IntVar(&idleTimeoutSec, "idle-timeout", idleTimeoutSec, "Idle timeout in seconds")
	fs.IntVar(&shutdownTimeoutSec, "shutdown-timeout", shutdownTimeoutSec, "Graceful shutdown timeout in seconds")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if addr != "" {
		opts.Addr = addr
	} else {
		opts.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	}
	opts.NATSURL = natsURL
	opts.DatabaseConnString = databaseConnString
	opts.JWTSecret = jwtSecret
	opts.JWKSURL = jwksURL
	opts.TelegramToken = telegramToken
	opts.TelegramWebhookURL = telegramWebhookURL
	opts.TelegramWebhookSecret = telegramWebhookSecret
	opts.AllowedUserIDs = allowedUserIDs
	opts.OrderChatID = orderChatID
	opts.ProductChatID = productChatID
	opts.ManageWebURL = manageWebURL
	opts.ReadTimeout = time.Duration(readTimeoutSec) * time.Second
	opts.WriteTimeout = time.Duration(writeTimeoutSec) * time.Second
	opts.IdleTimeout = time.Duration(idleTimeoutSec) * time.Second
	opts.ShutdownTimeout = time.Duration(shutdownTimeoutSec) * time.Second

	return *opts, nil
}

func applyEnv(opts *notification.ServerOptions) {
	if v := os.Getenv("DUPLI1_NOTIFICATION_ADDR"); v != "" {
		opts.Addr = v
	}
	if v := os.Getenv("DUPLI1_NOTIFICATION_NATS_URL"); v != "" {
		opts.NATSURL = v
	} else if v := os.Getenv("NATS_URL"); v != "" {
		opts.NATSURL = v
	}
	if v := os.Getenv("DUPLI1_NOTIFICATION_DB"); v != "" {
		opts.DatabaseConnString = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		opts.JWTSecret = v
	}
	if v := os.Getenv("AUTH_JWKS_URL"); v != "" {
		opts.JWKSURL = v
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		opts.TelegramToken = v
	}
	if v := os.Getenv("TELEGRAM_WEBHOOK_URL"); v != "" {
		opts.TelegramWebhookURL = v
	}
	if v := os.Getenv("TELEGRAM_WEBHOOK_SECRET"); v != "" {
		opts.TelegramWebhookSecret = v
	}
	if v := os.Getenv("TELEGRAM_ALLOWED_USER_IDS"); v != "" {
		opts.AllowedUserIDs = v
	}
	if v := os.Getenv("TELEGRAM_ORDER_CHAT_ID"); v != "" {
		opts.OrderChatID = v
	}
	if v := os.Getenv("TELEGRAM_PRODUCT_CHAT_ID"); v != "" {
		opts.ProductChatID = v
	}
	if v := os.Getenv("MANAGE_WEB_URL"); v != "" {
		opts.ManageWebURL = v
	}
}

func splitAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if addr == "" {
			return "", 8084, nil
		}
		return "", 0, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
