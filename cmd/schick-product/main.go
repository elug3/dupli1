package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/schick/pkg/product"
)

var usageStr = `
Usage: schick-product [OPTIONS]

A product server application that serves product data over HTTP.

Options:
  -host string
      Server host address (default: localhost)
  -port int
      Server port number (default: 8080)
  -db string
      Database connection string (default: postgresql://localhost/products)
  -nats string
      NATS connection URL
  -read-timeout int
      Read timeout in seconds (default: 15)
  -write-timeout int
      Write timeout in seconds (default: 15)
  -help
      Show this help message

Examples:
  schick-product
  schick-product -port 9000 -host 0.0.0.0
  schick-product -db postgresql://prod-db/products -nats nats://localhost:4222 -port 3000
`

func main() {
	fs := flag.NewFlagSet("schick", flag.ExitOnError)
	opts, err := ConfigureOptions(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ConfigureOptions: %v\n", err)
		os.Exit(1)
	}

	srv, err := product.NewSearchServer(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewSearchServer: %v\n", err)
		os.Exit(1)
	}

	interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var runErr chan error
	go func() {
		err := srv.Run()
		if err != nil {
			runErr <- err
		}
	}()

	// Wait interrupt signal or server error

	select {
	case <-runErr:
	case <-interrupt.Done():
	}

	srv.StopAndWait()
}
