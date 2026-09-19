// Package support is the customer-facing Telegram consultation bot.
// See docs/support-telegram-bot.md.
package support

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/elug3/dupli1/support/pkg/bootstrap"
)

type Server struct {
	opts     ServerOptions
	http     *http.Server
	app      *bootstrap.App
	stopped  chan struct{}
	stopOnce sync.Once
}

func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Addr == "" {
		return nil, fmt.Errorf("Addr is required")
	}

	app, err := bootstrap.Bootstrap(bootstrap.Config{
		Addr:                  opts.Addr,
		DatabaseConnString:    opts.DatabaseConnString,
		TelegramToken:         opts.TelegramToken,
		TelegramWebhookURL:    opts.TelegramWebhookURL,
		TelegramWebhookSecret: opts.TelegramWebhookSecret,
		TelegramAPIBase:       opts.TelegramAPIBase,
		NATSURL:               opts.NATSURL,
		ManageWebURL:          opts.ManageWebURL,
		BusinessHours:         opts.BusinessHours,
		MessageRetention:      opts.MessageRetention,
		JWTSecret:             opts.JWTSecret,
		JWKSURL:               opts.JWKSURL,
		ReadTimeout:           opts.ReadTimeout,
		WriteTimeout:          opts.WriteTimeout,
		IdleTimeout:           opts.IdleTimeout,
	})
	if err != nil {
		return nil, err
	}

	return &Server{
		opts:    opts,
		http:    app.HTTP,
		app:     app,
		stopped: make(chan struct{}),
	}, nil
}

func (s *Server) Run() error {
	fmt.Printf("Starting support server on %s\n", s.http.Addr)
	err := s.http.ListenAndServe()
	s.markStopped()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Stop() error {
	// Graceful shutdown timeout parent after SIGINT; not tied to a request.
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()

	fmt.Println("Gracefully stopping support server...")
	err := s.http.Shutdown(ctx)
	if closeErr := s.app.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (s *Server) Wait() { <-s.stopped }

func (s *Server) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}

func (s *Server) markStopped() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

func (s *Server) App() *bootstrap.App { return s.app }
