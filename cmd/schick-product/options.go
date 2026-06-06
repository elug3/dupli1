package main

import (
	"flag"

	"github.com/schick/pkg/product"
)

func ConfigureOptions(fs *flag.FlagSet, args []string) (product.SearchServerOptions, error) {
	opts := product.DefaultSearchServerOptions

	// Check environment variables first, then use defaults

	// Define command-line flags (these override environment variables)
	fs.StringVar(&opts.Host, "host", opts.Host, "Server host address")
	fs.IntVar(&opts.Port, "port", opts.Port, "Server port number")
	fs.StringVar(&opts.DatabaseConnString, "db", opts.DatabaseConnString, "Database connection string")
	fs.IntVar(&opts.ReadTimeout, "read-timeout", opts.ReadTimeout, "Read timeout in seconds")
	fs.IntVar(&opts.WriteTimeout, "write-timeout", opts.WriteTimeout, "Write timeout in seconds")

	// Parse command-line arguments
	err := fs.Parse(args)
	if err != nil {
		return opts, err
	}

	return opts, nil
}
