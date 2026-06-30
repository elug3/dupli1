package bootstrap

// Config holds the configuration required to wire the product search service.
type Config struct {
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
	S3Endpoint         string
	S3AccessKey        string
	S3SecretKey        string
	S3Bucket           string
}
