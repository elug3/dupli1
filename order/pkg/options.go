package order

import "time"

type ServerOptions struct {
	Addr string

	// GatewayURL is the internal nginx gateway base (preferred for product stock/coupons).
	// Example Compose: http://dupli1-proxy  Example ECS: http://proxy.dupli1.local
	GatewayURL string

	// ProductURL is a deprecated direct product base URL. Prefer GatewayURL.
	ProductURL string
	// InventoryURL is a deprecated alias for ProductURL.
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
		GatewayURL:      "http://localhost:8080",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
