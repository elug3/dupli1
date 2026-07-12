package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/elug3/dupli1/product/pkg"
)

var usageStr = `
Usage: dupli1-product [OPTIONS]

Product catalog server for bags, coupons, and image uploads.

Options:
  -host string
      Server host address (default: localhost; also SERVER_HOST env)
  -port int
      Server port number (default: 8080; also SERVER_PORT env)
  -db string
      Database connection string (also DUPLI1_PRODUCT_DB or DB_URL env)
  -jwt-secret string
      JWT secret for validating access tokens (also JWT_SECRET env)
  -read-timeout int
      Read timeout in seconds (also DUPLI1_PRODUCT_READ_TIMEOUT env)
  -write-timeout int
      Write timeout in seconds (also DUPLI1_PRODUCT_WRITE_TIMEOUT env)
  -help
      Show this help message

Environment variables:
  SERVER_HOST, SERVER_PORT, DB_URL, DUPLI1_PRODUCT_DB, JWT_SECRET,
  S3_ENDPOINT, S3_PUBLIC_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET,
  DUPLI1_PRODUCT_READ_TIMEOUT, DUPLI1_PRODUCT_WRITE_TIMEOUT

Examples:
  dupli1-product
  dupli1-product -port 9000 -host 0.0.0.0
  DUPLI1_PRODUCT_DB=postgres://dupli1:dupli1_dev@localhost:5433/products dupli1-product
`

func main() {
	fs := flag.NewFlagSet("dupli1-product", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usageStr)
	}

	opts, err := ConfigureOptions(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ConfigureOptions: %v\n", err)
		os.Exit(1)
	}

	srv, err := product.NewServer(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewServer: %v\n", err)
		os.Exit(1)
	}

	interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	runErr := make(chan error, 1)
	go func() {
		runErr <- srv.Run()
	}()

	select {
	case err := <-runErr:
		if err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		}
	case <-interrupt.Done():
	}

	srv.StopAndWait()
}
