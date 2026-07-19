package order

import "time"

type ServerOptions struct {
	Addr string

	ProductURL string
	// InventoryURL is deprecated; prefer ProductURL (stock is served by product).
	InventoryURL string

	AuthURL              string
	OrderServiceEmail    string
	OrderServicePassword string
	StockBearerToken     string

	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8083",
		ProductURL:      "http://localhost:8081",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
