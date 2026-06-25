package product

// SearchServerOptions configures the read-only product search server (customer-facing)
type SearchServerOptions struct {
	Host               string
	Port               int
	DatabaseConnString string
	NATSURL            string
	JWTSecret          string
	ReadTimeout        int // in seconds
	WriteTimeout       int // in seconds
}

var DefaultSearchServerOptions = SearchServerOptions{
	Host:               "localhost",
	Port:               8080,
	DatabaseConnString: "postgresql://localhost/products",
	ReadTimeout:        15,
	WriteTimeout:       15,
}

// ServerOptions configures the full product server (admin/manager)
type ServerOptions struct {
	Host               string
	Port               int
	DatabaseConnString string
	JWTSecret          string
	ReadTimeout        int // in seconds
	WriteTimeout       int // in seconds
	S3Endpoint         string
	S3AccessKey        string
	S3SecretKey        string
	S3Bucket           string
}

var DefaultServerOptions = ServerOptions{
	Host:               "localhost",
	Port:               8080,
	DatabaseConnString: "postgresql://localhost/products",
	ReadTimeout:        300, // 5 min — large image uploads need time
	WriteTimeout:       300, // 5 min — includes S3 write time
	S3Bucket:           "product-images",
}

func NewSearchServerOptions() *SearchServerOptions {
	opts := DefaultSearchServerOptions
	return &opts
}

func NewServerOptions() *ServerOptions {
	opts := DefaultServerOptions
	return &opts
}
