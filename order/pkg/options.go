package order

import "time"

type ServerOptions struct {
	Addr            string
	InventoryURL    string
	JWTSecret       string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8083",
		InventoryURL:    "http://localhost:8082",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
