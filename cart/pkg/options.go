package cart

import "time"

type ServerOptions struct {
	Addr               string
	ProductURL         string
	InventoryURL       string
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8086",
		ProductURL:      "http://localhost:8081",
		InventoryURL:    "http://localhost:8082",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
