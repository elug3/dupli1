package payment

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/elug3/dupli1/payment/pkg/bootstrap"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
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
	if opts.OrderURL == "" {
		return nil, fmt.Errorf("OrderURL is required")
	}

	app, err := bootstrap.Bootstrap(bootstrap.Config{
		OrderURL:           opts.OrderURL,
		DatabaseConnString: opts.DatabaseConnString,
		JWTSecret:          opts.JWTSecret,
		JWKSURL:            opts.JWKSURL,
		NATSURL:            opts.NATSURL,
		PublicBaseURL:      opts.PublicBaseURL,
		AllowDevSimulate:   opts.AllowDevSimulate,
		Nano: checkout.NanoConfig{
			BaseURL:       opts.NanoBaseURL,
			Ver:           opts.NanoVer,
			ShopCode:      opts.NanoShopCode,
			LoginID:       opts.NanoLoginID,
			APIKey:        opts.NanoAPIKey,
			PublicBaseURL: opts.PublicBaseURL,
			SuccessURL:    opts.NanoSuccessURL,
			FailureURL:    opts.NanoFailureURL,
		},
		HTTPClient: bootstrap.DefaultHTTPClient(),
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
	fmt.Printf("Starting payment server on %s\n", s.http.Addr)
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

	fmt.Println("Gracefully stopping payment server...")
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
