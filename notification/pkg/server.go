package notification

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/elug3/dupli1/notification/pkg/bootstrap"
)

type ServerOptions struct {
	Addr            string
	NATSURL         string
	TelegramToken   string
	OrderChatID     string
	ProductChatID   string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8084",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}

type Server struct {
	opts     ServerOptions
	app      *bootstrap.App
	stopped  chan struct{}
	stopOnce sync.Once
}

func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Addr == "" {
		return nil, fmt.Errorf("Addr is required")
	}

	app, err := bootstrap.Bootstrap(bootstrap.Config{
		Addr:            opts.Addr,
		NATSURL:         opts.NATSURL,
		TelegramToken:   opts.TelegramToken,
		OrderChatID:     opts.OrderChatID,
		ProductChatID:   opts.ProductChatID,
		ReadTimeout:     opts.ReadTimeout,
		WriteTimeout:    opts.WriteTimeout,
		IdleTimeout:     opts.IdleTimeout,
		ShutdownTimeout: opts.ShutdownTimeout,
	})
	if err != nil {
		return nil, err
	}

	return &Server{
		opts:    opts,
		app:     app,
		stopped: make(chan struct{}),
	}, nil
}

func (s *Server) Run() error {
	fmt.Printf("Starting notification server on %s\n", s.app.HTTP.Addr)
	err := s.app.HTTP.ListenAndServe()
	s.markStopped()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()

	fmt.Println("Gracefully stopping notification server...")
	err := s.app.HTTP.Shutdown(ctx)
	if closeErr := s.app.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (s *Server) Wait() {
	<-s.stopped
}

func (s *Server) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}

func (s *Server) markStopped() {
	s.stopOnce.Do(func() {
		close(s.stopped)
	})
}
