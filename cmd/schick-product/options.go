package main

import (
	"flag"
	"os"
	"strconv"

	"github.com/schick/pkg/product"
)

func ConfigureOptions(fs *flag.FlagSet, args []string) (product.ServerOptions, error) {
	opts := product.DefaultServerOptions
	applyEnv(&opts)

	fs.StringVar(&opts.Host, "host", opts.Host, "Server host address")
	fs.IntVar(&opts.Port, "port", opts.Port, "Server port number")
	fs.StringVar(&opts.DatabaseConnString, "db", opts.DatabaseConnString, "Database connection string")
	fs.StringVar(&opts.JWTSecret, "jwt-secret", opts.JWTSecret, "JWT secret for validating access tokens (also JWT_SECRET env)")
	fs.IntVar(&opts.ReadTimeout, "read-timeout", opts.ReadTimeout, "Read timeout in seconds")
	fs.IntVar(&opts.WriteTimeout, "write-timeout", opts.WriteTimeout, "Write timeout in seconds")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}

	return opts, nil
}

func applyEnv(opts *product.ServerOptions) {
	if v := os.Getenv("SERVER_HOST"); v != "" {
		opts.Host = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Port = n
		}
	}
	if v := os.Getenv("DB_URL"); v != "" {
		opts.DatabaseConnString = v
	}
	// SCHICK_PRODUCT_DB takes precedence over the generic DB_URL
	if v := os.Getenv("SCHICK_PRODUCT_DB"); v != "" {
		opts.DatabaseConnString = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		opts.JWTSecret = v
	}
	setIntEnv(&opts.ReadTimeout, "SCHICK_PRODUCT_READ_TIMEOUT")
	setIntEnv(&opts.WriteTimeout, "SCHICK_PRODUCT_WRITE_TIMEOUT")
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		opts.S3Endpoint = v
	}
	if v := os.Getenv("S3_ACCESS_KEY"); v != "" {
		opts.S3AccessKey = v
	}
	if v := os.Getenv("S3_SECRET_KEY"); v != "" {
		opts.S3SecretKey = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		opts.S3Bucket = v
	}
}

func setIntEnv(target *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}
