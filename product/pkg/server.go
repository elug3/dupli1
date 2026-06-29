package product

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/elug3/schick/product/pkg/bootstrap"
)

// ProductSearchServer is the read-only server for customers to search products.
type ProductSearchServer struct {
	opts     SearchServerOptions
	http     *http.Server
	app      *bootstrap.App
	mu       sync.RWMutex
	stopped  chan struct{}
	stopOnce sync.Once
}

// NewSearchServer creates and wires a new read-only product search server.
func NewSearchServer(opts SearchServerOptions) (*ProductSearchServer, error) {
	app, err := bootstrap.Bootstrap(context.Background(), bootstrap.Config{
		DatabaseConnString: opts.DatabaseConnString,
		JWTSecret:          opts.JWTSecret,
		JWKSURL:            opts.JWKSURL,
	})
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      app.Handler,
		ReadTimeout:  time.Duration(opts.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(opts.WriteTimeout) * time.Second,
	}

	return &ProductSearchServer{
		opts:    opts,
		http:    srv,
		app:     app,
		stopped: make(chan struct{}),
	}, nil
}

// Run starts the server and blocks until it stops.
func (s *ProductSearchServer) Run() error {
	s.mu.RLock()
	addr := s.http.Addr
	s.mu.RUnlock()

	fmt.Printf("Starting ProductSearchServer on %s\n", addr)
	err := s.http.ListenAndServe()
	s.markStopped()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully stops the server and releases infrastructure resources.
func (s *ProductSearchServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.http == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Gracefully stopping ProductSearchServer...")
	err := s.http.Shutdown(ctx)
	if closeErr := s.app.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (s *ProductSearchServer) markStopped() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

// Wait blocks until the server has stopped.
func (s *ProductSearchServer) Wait() {
	<-s.stopped
}

// StopAndWait gracefully stops the server and waits for it to close.
func (s *ProductSearchServer) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}

// ProductServer is the full-featured server for admin/manager operations.
type ProductServer struct {
	opts     ServerOptions
	server   *http.Server
	app      *bootstrap.App
	mu       sync.RWMutex
	stopped  chan struct{}
	stopOnce sync.Once
}

// NewServer creates and wires a new full-featured product server for admin/manager use.
func NewServer(opts ServerOptions) (*ProductServer, error) {
	app, err := bootstrap.Bootstrap(context.Background(), bootstrap.Config{
		DatabaseConnString: opts.DatabaseConnString,
		JWTSecret:          opts.JWTSecret,
		JWKSURL:            opts.JWKSURL,
		S3Endpoint:         opts.S3Endpoint,
		S3AccessKey:        opts.S3AccessKey,
		S3SecretKey:        opts.S3SecretKey,
		S3Bucket:           opts.S3Bucket,
	})
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	return &ProductServer{
		opts: opts,
		app:  app,
		server: &http.Server{
			Addr:         addr,
			Handler:      app.Handler,
			ReadTimeout:  time.Duration(opts.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(opts.WriteTimeout) * time.Second,
		},
		stopped: make(chan struct{}),
	}, nil
}

func (s *ProductServer) Run() error {
	s.mu.RLock()
	addr := s.server.Addr
	s.mu.RUnlock()

	fmt.Printf("Starting ProductServer on %s\n", addr)
	err := s.server.ListenAndServe()
	s.markStopped()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *ProductServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Gracefully stopping ProductServer...")
	err := s.server.Shutdown(ctx)
	if closeErr := s.app.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (s *ProductServer) markStopped() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

func (s *ProductServer) Wait() {
	<-s.stopped
}

func (s *ProductServer) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}
