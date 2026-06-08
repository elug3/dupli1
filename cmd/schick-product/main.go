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

A product search server that serves product catalog APIs over HTTP.

Options:
  -host string
      Server host address (default: localhost; also SERVER_HOST env)
  -port int
      Server port number (default: 8080; also SERVER_PORT env)
  -db string
      Database connection string (also DB_URL or SCHICK_PRODUCT_DB env)
  -read-timeout int
      Read timeout in seconds (also SCHICK_PRODUCT_READ_TIMEOUT env)
  -write-timeout int
      Write timeout in seconds (also SCHICK_PRODUCT_WRITE_TIMEOUT env)
  -help
      Show this help message

Environment variables:
  SERVER_HOST, SERVER_PORT, DB_URL, SCHICK_PRODUCT_DB,
  SCHICK_PRODUCT_READ_TIMEOUT, SCHICK_PRODUCT_WRITE_TIMEOUT

Examples:
  schick-product
  schick-product -port 9000 -host 0.0.0.0
  schick-product -db postgresql://prod-db/products -port 3000
  DB_URL=postgresql://prod-db/products schick-product
`

func main() {
	fs := flag.NewFlagSet("schick-product", flag.ExitOnError)
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
		fmt.Fprintf(os.Stderr, "NewSearchServer: %v\n", err)
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
