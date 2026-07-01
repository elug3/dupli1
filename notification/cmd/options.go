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
		addr               string
		natsURL            = opts.NATSURL
		telegramToken      = opts.TelegramToken
		orderChatID        = opts.OrderChatID
		productChatID      = opts.ProductChatID
		readTimeoutSec     = int(opts.ReadTimeout / time.Second)
		writeTimeoutSec    = int(opts.WriteTimeout / time.Second)
		idleTimeoutSec     = int(opts.IdleTimeout / time.Second)
		shutdownTimeoutSec = int(opts.ShutdownTimeout / time.Second)
	)

	fs.StringVar(&host, "host", host, "Server host address")
	fs.IntVar(&port, "port", port, "Server port number")
	fs.StringVar(&addr, "addr", "", "Server listen address (overrides host/port)")
	fs.StringVar(&natsURL, "nats-url", natsURL, "NATS server URL")
	fs.StringVar(&telegramToken, "telegram-token", telegramToken, "Telegram bot token")
	fs.StringVar(&orderChatID, "telegram-order-chat-id", orderChatID, "Telegram chat ID for order manager alerts")
	fs.StringVar(&productChatID, "telegram-product-chat-id", productChatID, "Telegram chat ID for product manager alerts")
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
	opts.TelegramToken = telegramToken
	opts.OrderChatID = orderChatID
	opts.ProductChatID = productChatID
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
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		opts.TelegramToken = v
	}
	if v := os.Getenv("TELEGRAM_ORDER_CHAT_ID"); v != "" {
		opts.OrderChatID = v
	}
	if v := os.Getenv("TELEGRAM_PRODUCT_CHAT_ID"); v != "" {
		opts.ProductChatID = v
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
