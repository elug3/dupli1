// Package sentrymon reports a service's panics, 5xx responses and log lines to
// Sentry. Everything is driven by environment variables and is a no-op when
// SENTRY_DSN is unset, so local runs and tests need no Sentry at all.
//
//	SENTRY_DSN                 project DSN (sentry.io or self-hosted); unset = off
//	SENTRY_ENVIRONMENT         e.g. production, staging (default "development")
//	SENTRY_RELEASE             release name, e.g. the image tag
//	SENTRY_TRACES_SAMPLE_RATE  0..1 share of requests traced (default 0)
//	SENTRY_LOGS                "false" stops forwarding log lines (default on)
//
// Request bodies, cookies and sensitive headers are never sent
// (SendDefaultPII stays false).
package sentrymon

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

const flushTimeout = 2 * time.Second

var (
	enabled bool
	logger  sentry.Logger

	// transport replaces the network transport in tests.
	transport sentry.Transport
)

// Enabled reports whether Init connected to Sentry.
func Enabled() bool { return enabled }

// Init configures Sentry for service and, unless SENTRY_LOGS=false, mirrors
// the standard library logger to Sentry Logs while still writing to stderr.
// The returned func flushes buffered events; defer it from main. Init never
// fails the process: a bad DSN is logged and Sentry stays off.
func Init(service string) (flush func()) {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if dsn == "" {
		return func() {}
	}

	env := strings.TrimSpace(os.Getenv("SENTRY_ENVIRONMENT"))
	if env == "" {
		env = "development"
	}
	rate := 0.0
	if v := strings.TrimSpace(os.Getenv("SENTRY_TRACES_SAMPLE_RATE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			rate = f
		} else {
			log.Printf("sentry: ignoring SENTRY_TRACES_SAMPLE_RATE=%q (want 0..1)", v)
		}
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Release:          strings.TrimSpace(os.Getenv("SENTRY_RELEASE")),
		ServerName:       service,
		AttachStacktrace: true,
		EnableTracing:    rate > 0,
		TracesSampleRate: rate,
		Transport:        transport,
	})
	if err != nil {
		log.Printf("sentry: disabled, init failed: %v", err)
		return func() {}
	}
	sentry.ConfigureScope(func(s *sentry.Scope) { s.SetTag("service", service) })
	logger = sentry.NewLogger(context.Background())
	enabled = true

	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SENTRY_LOGS")), "false") {
		log.SetOutput(io.MultiWriter(log.Writer(), LogWriter()))
	}
	log.Printf("sentry: reporting as %s (environment %s)", service, env)
	return func() { sentry.Flush(flushTimeout) }
}

// Handler wraps h so a panic in a request is reported (and re-panicked, so
// net/http still logs it and closes the connection) and a 5xx response is
// reported as an error event. It returns h unchanged when Sentry is off.
func Handler(h http.Handler) http.Handler {
	if !enabled {
		return h
	}
	report := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		if rec.status >= 500 {
			if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
				hub.WithScope(func(s *sentry.Scope) {
					s.SetLevel(sentry.LevelError)
					s.SetTag("http.status_code", strconv.Itoa(rec.status))
					s.SetFingerprint([]string{"http-5xx", routeOf(r), strconv.Itoa(rec.status)})
					hub.CaptureMessage(fmt.Sprintf("HTTP %d %s", rec.status, routeOf(r)))
				})
			}
		}
	})
	return sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle(report)
}

// routeOf names the request as "METHOD /path", preferring the ServeMux
// pattern so /orders/{id} groups as one issue.
func routeOf(r *http.Request) string {
	switch {
	case r.Pattern == "":
		return r.Method + " " + r.URL.Path
	case strings.Contains(r.Pattern, " "):
		return r.Pattern
	default:
		return r.Method + " " + r.Pattern
	}
}

// LogWriter returns an io.Writer that sends each line written to it to Sentry
// Logs, at error level when the line reads like a failure and info otherwise.
// It is a no-op when Sentry is off.
func LogWriter() io.Writer { return logWriter{} }

type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	if !enabled {
		return len(p), nil
	}
	msg := strings.TrimRight(string(p), "\n")
	Emit(levelOf(msg), msg)
	return len(p), nil
}

// Level is a log severity for Emit.
type Level int

const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
)

// Emit sends one log line to Sentry Logs. It is a no-op when Sentry is off.
func Emit(level Level, msg string) {
	if !enabled || msg == "" {
		return
	}
	switch level {
	case LevelError:
		logger.Error().Emit(msg)
	case LevelWarn:
		logger.Warn().Emit(msg)
	default:
		logger.Info().Emit(msg)
	}
}

func levelOf(msg string) Level {
	m := strings.ToLower(msg)
	for _, w := range []string{"panic", "fatal", "error", "fail"} {
		if strings.Contains(m, w) {
			return LevelError
		}
	}
	if strings.Contains(m, "warn") {
		return LevelWarn
	}
	return LevelInfo
}

// statusRecorder remembers the status code while keeping streaming (SSE)
// working: it forwards Flush and exposes Unwrap for http.ResponseController.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
