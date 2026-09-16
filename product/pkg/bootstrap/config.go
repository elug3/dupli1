package bootstrap

// Config holds the configuration required to wire the product search service.
type Config struct {
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
	S3Endpoint         string
	S3PublicEndpoint   string
	S3AccessKey        string
	S3SecretKey        string
	S3Bucket           string
	// RedisURL shares the promotional-code rate-limit window across tasks.
	// Unset falls back to a per-process window, so local dev needs no
	// infrastructure — see pkg/infra/ratelimit.
	RedisURL string
}
