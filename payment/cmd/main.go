package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/elug3/dupli1/payment/pkg"
)

var usageStr = `
Usage: dupli1-payment [OPTIONS]

A payment server that creates Stripe Checkout sessions and publishes payment events.

Options:
  -host string
      Server host address
  -port int
      Server port number
  -addr string
      Server listen address (overrides host/port)
  -order-url string
      Order service base URL
  -help
      Show this help message
`

func main() {
	fs := flag.NewFlagSet("dupli1-payment", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usageStr)
	}

	opts, err := ConfigureOptions(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ConfigureOptions: %v\n", err)
		os.Exit(1)
	}

	srv, err := payment.NewServer(opts)
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
