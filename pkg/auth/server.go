package auth

import (
	"context"
	"net/http"
)

// Server represents the auth service HTTP server.
type Server struct {
	http *http.Server
}

// NewServer creates a new auth server.
func NewServer(opts ServerOptions) *Server {
	// validate provided options; panic on invalid config since signature doesn't return error
	if err := opts.Validate(); err != nil {
		panic(err)
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

	return &Server{http: srv}
}

// Start starts the server.
func (s *Server) Start() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
