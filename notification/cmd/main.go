package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	notification "github.com/elug3/dupli1/notification/pkg"
)

var usageStr = `
Usage: dupli1-notification [OPTIONS]

A notification server application that owns outbound customer and admin messaging APIs.

Options:
  -host string
      Server host address
  -port int
      Server port number
  -addr string
      Server listen address (overrides host/port)
  -help
      Show this help message
`

func main() {
	fs := flag.NewFlagSet("dupli1-notification", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usageStr)
	}

	opts, err := ConfigureOptions(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ConfigureOptions: %v\n", err)
		os.Exit(1)
	}

	srv, err := notification.NewServer(opts)
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
