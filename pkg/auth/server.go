package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// Server represents the auth service HTTP server.
type Server struct {
	opts      ServerOptions
	http      *http.Server
	mu        sync.RWMutex
	stopped   chan struct{}
	stopOnce  sync.Once
}

// NewServer creates a new auth server.
func NewServer(opts ServerOptions) (*Server, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         opts.Addr,
		Handler:      mux,
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
		IdleTimeout:  opts.IdleTimeout,
	}

	return &Server{
		opts:    opts,
		http:    srv,
		stopped: make(chan struct{}),
	}, nil
}

// Run starts the server and blocks until it stops or returns an error.
func (s *Server) Run() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("Starting auth server on %s\n", s.http.Addr)
	err := s.http.ListenAndServe()
	s.markStopped()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully stops the server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.http == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()

	fmt.Println("Gracefully stopping auth server...")
	return s.http.Shutdown(ctx)
}

func (s *Server) markStopped() {
	s.stopOnce.Do(func() {
		close(s.stopped)
	})
}

// Wait blocks until the server has stopped.
func (s *Server) Wait() {
	<-s.stopped
}

// StopAndWait gracefully stops the server and waits for it to close.
func (s *Server) StopAndWait() {
	_ = s.Stop()
	s.Wait()
}
