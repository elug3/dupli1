package order

import "time"

// DefaultShippingFeeWon is the flat per-order delivery charge in whole KRW
// applied when DUPLI1_ORDER_SHIPPING_FEE_WON (and the deprecated
// DUPLI1_ORDER_SHIPPING_FEE_KRW / DUPLI1_ORDER_SHIPPING_FEE_CENTS aliases) are not set.
const DefaultShippingFeeWon int64 = 30000

type ServerOptions struct {
	Addr string

	// GatewayURL is the internal nginx gateway base (preferred for product stock/promotions).
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

	// ShippingFeeWon is the flat delivery charge added to every order, in
	// whole KRW. Set DUPLI1_ORDER_SHIPPING_FEE_WON to override (deprecated
	// aliases: DUPLI1_ORDER_SHIPPING_FEE_KRW, DUPLI1_ORDER_SHIPPING_FEE_CENTS). An explicit 0 means free
	// delivery.
	ShippingFeeWon int64

	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr: ":8083",
		// GatewayURL must stay empty unless set via DUPLI1_GATEWAY_URL / -gateway-url.
		// A localhost default would shadow DUPLI1_PRODUCT_URL (preferred in older ECS
		// task defs) and make order call itself → 404 → checkout "unavailable items".
		// Local Compose sets DUPLI1_GATEWAY_URL; bare `go run` can still use ProductURL.
		ProductURL: "http://localhost:8081",
		// Flat delivery charge in whole KRW (30,000 KRW).
		ShippingFeeWon: DefaultShippingFeeWon,
		ReadTimeout:    5 * time.Second,
		// WriteTimeout covers paid cancel, which waits on NANO through payment.
		WriteTimeout:    25 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
