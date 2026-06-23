package product

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	natsinfra "github.com/elug3/schick/product/pkg/infra/nats"
	"github.com/elug3/schick/product/pkg/infra/pg"
	"github.com/elug3/schick/product/pkg/ports"
)

// ProductSearchServer is the read-only server for customers to search products
type ProductSearchServer struct {
	opts          SearchServerOptions
	server        *http.Server
	store         *pg.ProductSearchStore
	natsPublisher *natsinfra.Publisher
	mu            sync.RWMutex
	stopped       chan struct{}
}

// NewSearchServer creates and returns a new read-only product search server
func NewSearchServer(opts SearchServerOptions) (*ProductSearchServer, error) {
	mux := http.NewServeMux()
	addr := net.JoinHostPort(opts.Host, fmt.Sprintf("%d", opts.Port))

	store, err := pg.NewProductStore(opts.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	var eventPublisher ports.EventPublisher
	var natsPublisher *natsinfra.Publisher
	if opts.NATSURL != "" {
		natsPublisher, err = natsinfra.NewPublisher(opts.NATSURL)
		if err != nil {
			store.Close()
			return nil, err
		}
		eventPublisher = natsPublisher
	}

	service := NewProductSearchService(store, eventPublisher)
	handler := NewProductSearchHandler(service, opts.JWTSecret)
	handler.RegisterRoutes(mux)

	return &ProductSearchServer{
		opts: opts,
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  time.Duration(opts.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(opts.WriteTimeout) * time.Second,
		},
		store:         store,
		natsPublisher: natsPublisher,
		stopped:       make(chan struct{}),
	}, nil
}

// ProductServer is the full-featured server for admin/manager operations
type ProductServer struct {
	opts    ServerOptions
	server  *http.Server
	mu      sync.RWMutex
	stopped chan struct{}
}

// NewServer creates and returns a new full-featured product server for admin/manager use
func NewServer(opts ServerOptions) (*ProductServer, error) {
	mux := http.NewServeMux()
	addr := net.JoinHostPort(opts.Host, fmt.Sprintf("%d", opts.Port))

	return &ProductServer{
		opts: opts,
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  time.Duration(opts.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(opts.WriteTimeout) * time.Second,
		},
		stopped: make(chan struct{}),
	}, nil
}

// === ProductSearchServer Methods ===

// Run starts the search server and blocks until it's stopped
func (s *ProductSearchServer) Run() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("Starting ProductSearchServer on %s\n", s.server.Addr)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("ProductSearchServer error: %v\n", err)
		}
		close(s.stopped)
	}()

	return nil
}

// Stop gracefully stops the search server
func (s *ProductSearchServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	// Create a context with 30-second timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Gracefully stopping ProductSearchServer...")
	err := s.server.Shutdown(ctx)
	if s.natsPublisher != nil {
		s.natsPublisher.Close()
	}
	if s.store != nil {
		s.store.Close()
	}
	return err
}

// Wait blocks until the search server is closed
func (s *ProductSearchServer) Wait() {
	<-s.stopped
}

// StopAndWait gracefully stops the search server and waits for it to close
func (s *ProductSearchServer) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}

// IsReady checks if the search server is ready to serve requests
func (s *ProductSearchServer) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return false
	}

	// Try to connect to the server
	conn, err := net.DialTimeout("tcp", s.server.Addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// === ProductServer Methods ===

// Run starts the server and blocks until it's stopped
func (s *ProductServer) Run() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("Starting ProductServer on %s\n", s.server.Addr)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("ProductServer error: %v\n", err)
		}
		close(s.stopped)
	}()

	return nil
}

// Stop gracefully stops the server
func (s *ProductServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	// Create a context with 30-second timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Gracefully stopping ProductServer...")
	return s.server.Shutdown(ctx)
}

// Wait blocks until the server is closed
func (s *ProductServer) Wait() {
	<-s.stopped
}

// StopAndWait gracefully stops the server and waits for it to close
func (s *ProductServer) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}

// IsReady checks if the server is ready to serve requests
func (s *ProductServer) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server == nil {
		return false
	}

	// Try to connect to the server
	conn, err := net.DialTimeout("tcp", s.server.Addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
