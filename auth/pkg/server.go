package auth

import (
	"context"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/elug3/schick/auth/pkg/bootstrap"
	"github.com/rs/zerolog"
)

// Server represents the auth service HTTP server.
type Server struct {
	opts     ServerOptions
	http     *http.Server
	app      *bootstrap.App
	log      zerolog.Logger
	mu       sync.RWMutex
	stopped  chan struct{}
	stopOnce sync.Once
}

// NewServer creates a new auth server.
func NewServer(opts ServerOptions) (*Server, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	log := newLogger(opts.LogOutput, opts.LogLevel)
	log.Info().Msg("Starting auth server...")

	app, err := bootstrap.Bootstrap(context.Background(), bootstrap.Config{
		DBURL:              opts.DBURL,
		RedisURL:           opts.RedisURL,
		NATSURL:            opts.NATSURL,
		TokenSigningKey:    opts.TokenSigningKey,
		TokenExpiry:        opts.TokenExpiry,
		RefreshTokenExpiry: opts.RefreshTokenExpiry,
		Debug:              opts.Debug,
		MaxConns:           opts.MaxConns,
		Logger:        log,
		OwnerEmail:    opts.OwnerEmail,
		OwnerPassword: opts.OwnerPassword,
	})
	if err != nil {
		return nil, err
	}

	srv := &http.Server{
		Addr:         opts.Addr,
		Handler:      app.Engine,
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
		IdleTimeout:  opts.IdleTimeout,
	}

	return &Server{
		opts:    opts,
		http:    srv,
		app:     app,
		log:     log,
		stopped: make(chan struct{}),
	}, nil
}

// Run starts the server and blocks until it stops or returns an error.
// Uses TLS when both TLSCertFile and TLSKeyFile are set in ServerOptions.
func (s *Server) Run() error {
	s.mu.RLock()
	httpSrv := s.http
	certFile := s.opts.TLSCertFile
	keyFile := s.opts.TLSKeyFile
	s.mu.RUnlock()

	var err error
	if certFile != "" && keyFile != "" {
		err = httpSrv.ListenAndServeTLS(certFile, keyFile)
	} else {
		err = httpSrv.ListenAndServe()
	}
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

	s.log.Info().Msg("stopping")
	err := s.http.Shutdown(ctx)
	if closeErr := s.app.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
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

func newLogger(output, level string) zerolog.Logger {
	var w io.Writer
	if output == "json" {
		w = os.Stdout
	} else {
		w = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    true,
			PartsOrder: []string{zerolog.MessageFieldName},
		}
	}

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(w).With().Timestamp().Logger().Level(lvl)
}
