package main

import (
	"flag"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"

	support "github.com/elug3/dupli1/support/pkg"
)

type Options = support.ServerOptions

func ConfigureOptions(fs *flag.FlagSet, args []string) (Options, error) {
	opts := support.NewServerOptions()
	applyEnv(opts)

	host, port, err := splitAddr(opts.Addr)
	if err != nil {
		return Options{}, err
	}

	var (
		addr               string
		readTimeoutSec     = int(opts.ReadTimeout / time.Second)
		writeTimeoutSec    = int(opts.WriteTimeout / time.Second)
		idleTimeoutSec     = int(opts.IdleTimeout / time.Second)
		shutdownTimeoutSec = int(opts.ShutdownTimeout / time.Second)
	)

	fs.StringVar(&host, "host", host, "Server host address")
	fs.IntVar(&port, "port", port, "Server port number")
	fs.StringVar(&addr, "addr", "", "Server listen address (overrides host/port)")
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
	opts.ReadTimeout = time.Duration(readTimeoutSec) * time.Second
	opts.WriteTimeout = time.Duration(writeTimeoutSec) * time.Second
	opts.IdleTimeout = time.Duration(idleTimeoutSec) * time.Second
	opts.ShutdownTimeout = time.Duration(shutdownTimeoutSec) * time.Second

	return *opts, nil
}

// applyEnv reads the service's environment.
//
// The Telegram variables are deliberately TELEGRAM_SUPPORT_* rather than the
// ops bot's TELEGRAM_*: the two bots have separate tokens and separate webhook
// secrets, and a shared name is exactly how a deploy ends up pointing both at
// one bot — which Telegram resolves by giving one of them a 409.
func applyEnv(opts *support.ServerOptions) {
	if v := os.Getenv("DUPLI1_SUPPORT_ADDR"); v != "" {
		opts.Addr = v
	}
	if v := os.Getenv("DUPLI1_SUPPORT_DB"); v != "" {
		opts.DatabaseConnString = v
	}
	if v := os.Getenv("TELEGRAM_SUPPORT_BOT_TOKEN"); v != "" {
		opts.TelegramToken = v
	}
	if v := os.Getenv("TELEGRAM_SUPPORT_WEBHOOK_URL"); v != "" {
		opts.TelegramWebhookURL = v
	}
	if v := os.Getenv("TELEGRAM_SUPPORT_WEBHOOK_SECRET"); v != "" {
		opts.TelegramWebhookSecret = v
	}
	// Local dev and end-to-end tests only: point the bot at a mock Bot API so a
	// scripted consultation never messages a real person. Unset in production.
	if v := os.Getenv("TELEGRAM_SUPPORT_API_BASE"); v != "" {
		opts.TelegramAPIBase = v
	}
	if v := os.Getenv("DUPLI1_SUPPORT_NATS_URL"); v != "" {
		opts.NATSURL = v
	} else if v := os.Getenv("NATS_URL"); v != "" {
		opts.NATSURL = v
	}
	if v := os.Getenv("MANAGE_WEB_URL"); v != "" {
		opts.ManageWebURL = strings.TrimRight(v, "/")
	}
	// Service hours are config, not code, so a seasonal change is a deploy
	// variable. Holidays are deliberately absent — staff simply do not answer
	// on them, and the copy names the window rather than a day.
	if v := os.Getenv("DUPLI1_SUPPORT_HOURS_TZ"); v != "" {
		if loc, err := time.LoadLocation(v); err == nil {
			opts.BusinessHours.Location = loc
		} else {
			log.Printf("DUPLI1_SUPPORT_HOURS_TZ=%q is not a known timezone — keeping %s", v, opts.BusinessHours.Location)
		}
	}
	if hour, min, ok := parseClock(os.Getenv("DUPLI1_SUPPORT_HOURS_OPEN")); ok {
		opts.BusinessHours.OpenHour, opts.BusinessHours.OpenMin = hour, min
	}
	if hour, min, ok := parseClock(os.Getenv("DUPLI1_SUPPORT_HOURS_CLOSE")); ok {
		opts.BusinessHours.CloseHour, opts.BusinessHours.CloseMin = hour, min
	}
	if v := os.Getenv("DUPLI1_SUPPORT_HOURS_DAYS"); v != "" {
		if days := domain.ParseWeekdays(v); days != nil {
			opts.BusinessHours.Weekdays = days
		} else {
			log.Printf("DUPLI1_SUPPORT_HOURS_DAYS=%q is not readable — keeping Monday–Friday", v)
		}
	}
	// Retention in whole days, because that is how the policy is written.
	// A value of 0 disables the purge and is logged as such; an unreadable one
	// leaves the default rather than silently keeping transcripts forever.
	if v := os.Getenv("DUPLI1_SUPPORT_MESSAGE_RETENTION_DAYS"); v != "" {
		days, err := strconv.Atoi(strings.TrimSpace(v))
		switch {
		case err != nil || days < 0:
			log.Printf("DUPLI1_SUPPORT_MESSAGE_RETENTION_DAYS=%q is not a whole number of days — keeping the default", v)
		case days == 0:
			opts.MessageRetention = -1
		default:
			opts.MessageRetention = time.Duration(days) * 24 * time.Hour
		}
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		opts.JWTSecret = v
	}
	if v := os.Getenv("AUTH_JWKS_URL"); v != "" {
		opts.JWKSURL = v
	}
}

// parseClock reads "HH:MM". An unreadable value leaves the hours alone rather
// than silently moving them.
func parseClock(value string) (hour, min int, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	hourText, minText, found := strings.Cut(value, ":")
	if !found {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(strings.TrimSpace(hourText))
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	min, err = strconv.Atoi(strings.TrimSpace(minText))
	if err != nil || min < 0 || min > 59 {
		return 0, 0, false
	}
	return hour, min, true
}

func splitAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if addr == "" {
			return "", 8089, nil
		}
		return "", 0, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
