package main

import (
	"flag"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/elug3/dupli1/payment/pkg"
)

type Options = payment.ServerOptions

func ConfigureOptions(fs *flag.FlagSet, args []string) (Options, error) {
	opts := payment.NewServerOptions()
	applyEnv(opts)

	host, port, err := splitAddr(opts.Addr)
	if err != nil {
		return Options{}, err
	}

	var (
		addr                string
		orderURL            = opts.OrderURL
		readTimeoutSec      = int(opts.ReadTimeout / time.Second)
		writeTimeoutSec     = int(opts.WriteTimeout / time.Second)
		idleTimeoutSec      = int(opts.IdleTimeout / time.Second)
		shutdownTimeoutSec  = int(opts.ShutdownTimeout / time.Second)
	)

	fs.StringVar(&host, "host", host, "Server host address")
	fs.IntVar(&port, "port", port, "Server port number")
	fs.StringVar(&addr, "addr", "", "Server listen address (overrides host/port)")
	fs.StringVar(&orderURL, "order-url", orderURL, "Order service base URL")
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
	opts.OrderURL = orderURL
	opts.ReadTimeout = time.Duration(readTimeoutSec) * time.Second
	opts.WriteTimeout = time.Duration(writeTimeoutSec) * time.Second
	opts.IdleTimeout = time.Duration(idleTimeoutSec) * time.Second
	opts.ShutdownTimeout = time.Duration(shutdownTimeoutSec) * time.Second

	return *opts, nil
}

func applyEnv(opts *payment.ServerOptions) {
	if v := os.Getenv("DUPLI1_PAYMENT_ADDR"); v != "" {
		opts.Addr = v
	}
	if v := os.Getenv("DUPLI1_ORDER_URL"); v != "" {
		opts.OrderURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		opts.JWTSecret = v
	}
	if v := os.Getenv("AUTH_JWKS_URL"); v != "" {
		opts.JWKSURL = v
	}
	if v := os.Getenv("NATS_URL"); v != "" {
		opts.NATSURL = v
	}
	if v := os.Getenv("STRIPE_SECRET_KEY"); v != "" {
		opts.StripeSecretKey = v
	}
	if v := os.Getenv("STRIPE_WEBHOOK_SECRET"); v != "" {
		opts.StripeWebhookSecret = v
	}
	if v := os.Getenv("STRIPE_SUCCESS_URL"); v != "" {
		opts.StripeSuccessURL = v
	}
	if v := os.Getenv("STRIPE_CANCEL_URL"); v != "" {
		opts.StripeCancelURL = v
	}
	if v := os.Getenv("DUPLI1_PAYMENT_PUBLIC_URL"); v != "" {
		opts.PublicBaseURL = v
	}
	if v := os.Getenv("DUPLI1_PAYMENT_DB"); v != "" {
		opts.DatabaseConnString = v
	} else if v := os.Getenv("DB_URL"); v != "" {
		opts.DatabaseConnString = v
	}
}

func splitAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if addr == "" {
			return "", 8087, nil
		}
		return "", 0, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
