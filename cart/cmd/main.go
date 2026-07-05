package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/elug3/dupli1/cart/pkg"
)

var usageStr = `
Usage: dupli1-cart [OPTIONS]

A cart server application that serves shopping cart APIs over HTTP.

Options:
  -host string
      Server host address
  -port int
      Server port number
  -addr string
      Server listen address (overrides host/port)
  -product-url string
      Product service base URL
  -inventory-url string
      Inventory service base URL
  -help
      Show this help message
`

func main() {
	fs := flag.NewFlagSet("dupli1-cart", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usageStr)
	}

	opts, err := ConfigureOptions(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ConfigureOptions: %v\n", err)
		os.Exit(1)
	}

	srv, err := cart.NewServer(opts)
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
