package payment

import "time"

type ServerOptions struct {
	Addr               string
	OrderURL           string
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
	PublicBaseURL      string
	// NANO Solution certified payment (인증결제). When API key + shop are set,
	// credit_card uses NANO instead of being unavailable (use method=bypass otherwise).
	NanoBaseURL     string
	NanoVer         string
	NanoShopCode    string
	NanoLoginID     string
	NanoAPIKey      string
	NanoSuccessURL  string
	NanoFailureURL  string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8087",
		OrderURL:        "http://localhost:8083",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    30 * time.Second, // NANO checkout bridge may wait on upstream
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
