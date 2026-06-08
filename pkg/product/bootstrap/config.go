package bootstrap

// Config holds the configuration required to wire the product search service.
type Config struct {
	DatabaseConnString string
	JWTSecret          string
}
