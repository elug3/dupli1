package order

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/elug3/dupli1/order/pkg/bootstrap"
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
	if opts.InventoryURL == "" {
		return nil, fmt.Errorf("InventoryURL is required")
	}

	app, err := bootstrap.Bootstrap(bootstrap.Config{
		InventoryURL:             opts.InventoryURL,
		ProductURL:               opts.ProductURL,
		AuthURL:                  opts.AuthURL,
		InventoryServiceEmail:    opts.InventoryServiceEmail,
		InventoryServicePassword: opts.InventoryServicePassword,
		InventoryBearerToken:     opts.InventoryBearerToken,
		DatabaseConnString:       opts.DatabaseConnString,
		JWTSecret:                opts.JWTSecret,
		JWKSURL:                  opts.JWKSURL,
		NATSURL:                  opts.NATSURL,
		HTTPClient:               bootstrap.DefaultHTTPClient(),
	})
	if err != nil {
		return nil, err
	}
	httpSrv := &http.Server{
		Addr:         opts.Addr,
		Handler:      app.Router,
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
		IdleTimeout:  opts.IdleTimeout,
	}

	return &Server{
		opts:    opts,
		http:    httpSrv,
		app:     app,
		stopped: make(chan struct{}),
	}, nil
}

func (s *Server) Run() error {
	fmt.Printf("Starting order server on %s\n", s.http.Addr)
	err := s.http.ListenAndServe()
	s.markStopped()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()

	fmt.Println("Gracefully stopping order server...")
	err := s.http.Shutdown(ctx)
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

func (s *Server) App() *bootstrap.App {
	return s.app
}
