package product

// SearchServerOptions configures the read-only product search server (customer-facing)
type SearchServerOptions struct {
	Host               string
	Port               int
	DatabaseConnString string
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
	ReadTimeout        int // in seconds
	WriteTimeout       int // in seconds
}

var DefaultServerOptions = ServerOptions{
	Host:               "localhost",
	Port:               8080,
	DatabaseConnString: "postgresql://localhost/products",
	ReadTimeout:        15,
	WriteTimeout:       15,
}

func NewServerOptions() *ServerOptions {
	opts := DefaultServerOptions
	return &opts
}
