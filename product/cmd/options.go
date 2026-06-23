package main

import (
	"flag"
	"os"

	"github.com/elug3/schick/product/pkg"
)

func ConfigureOptions(fs *flag.FlagSet, args []string) (product.SearchServerOptions, error) {
	opts := product.DefaultSearchServerOptions

	// Check environment variables first, then use defaults
	if v := os.Getenv("SCHICK_PRODUCT_DB_URL"); v != "" {
		opts.DatabaseConnString = v
	} else if v := os.Getenv("DB_URL"); v != "" {
		opts.DatabaseConnString = v
	}
	if v := os.Getenv("SCHICK_PRODUCT_NATS_URL"); v != "" {
		opts.NATSURL = v
	} else if v := os.Getenv("NATS_URL"); v != "" {
		opts.NATSURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		opts.JWTSecret = v
	}

	// Define command-line flags (these override environment variables)
	fs.StringVar(&opts.Host, "host", opts.Host, "Server host address")
	fs.IntVar(&opts.Port, "port", opts.Port, "Server port number")
	fs.StringVar(&opts.DatabaseConnString, "db", opts.DatabaseConnString, "Database connection string")
	fs.StringVar(&opts.NATSURL, "nats", opts.NATSURL, "NATS connection URL")
	fs.IntVar(&opts.ReadTimeout, "read-timeout", opts.ReadTimeout, "Read timeout in seconds")
	fs.IntVar(&opts.WriteTimeout, "write-timeout", opts.WriteTimeout, "Write timeout in seconds")

	// Parse command-line arguments
	err := fs.Parse(args)
	if err != nil {
		return opts, err
	}

	return opts, nil
}
