package payment

import "time"

type ServerOptions struct {
	Addr                string
	OrderURL            string
	DatabaseConnString  string
	JWTSecret           string
	JWKSURL             string
	NATSURL             string
	StripeSecretKey     string
	StripeWebhookSecret string
	StripeSuccessURL    string
	StripeCancelURL     string
	PublicBaseURL       string
	AllowDevSimulate    bool // PAYMENT_ALLOW_DEV_SIMULATE — local/dev only
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	ShutdownTimeout     time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8087",
		OrderURL:        "http://localhost:8083",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
