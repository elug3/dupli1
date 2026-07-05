package main

import (
	"flag"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/elug3/dupli1/cart/pkg"
)

type Options = cart.ServerOptions

func ConfigureOptions(fs *flag.FlagSet, args []string) (Options, error) {
	opts := cart.NewServerOptions()
	applyEnv(opts)

	host, port, err := splitAddr(opts.Addr)
	if err != nil {
		return Options{}, err
	}

	var (
		addr               string
		productURL         = opts.ProductURL
		inventoryURL       = opts.InventoryURL
		readTimeoutSec     = int(opts.ReadTimeout / time.Second)
		writeTimeoutSec    = int(opts.WriteTimeout / time.Second)
		idleTimeoutSec     = int(opts.IdleTimeout / time.Second)
		shutdownTimeoutSec = int(opts.ShutdownTimeout / time.Second)
	)

	fs.StringVar(&host, "host", host, "Server host address")
	fs.IntVar(&port, "port", port, "Server port number")
	fs.StringVar(&addr, "addr", "", "Server listen address (overrides host/port)")
	fs.StringVar(&productURL, "product-url", productURL, "Product service base URL")
	fs.StringVar(&inventoryURL, "inventory-url", inventoryURL, "Inventory service base URL")
	fs.IntVar(&readTimeoutSec, "read-timeout", readTimeoutSec, "Read timeout in seconds")
	fs.IntVar(&writeTimeoutSec, "write-timeout", writeTimeoutSec, "Write timeout in seconds")
	fs.IntVar(&idleTimeoutSec, "idle-timeout", idleTimeoutSec, "Idle timeout in seconds")
	fs.IntVar(&shutdownTimeoutSec, "shutdown-timeout", shutdownTimeoutSec, "Graceful shutdown timeout in seconds")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if addr != "" {
		opts.Addr = addr
	} else {
		opts.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	}
	opts.ProductURL = productURL
	opts.InventoryURL = inventoryURL
	opts.ReadTimeout = time.Duration(readTimeoutSec) * time.Second
	opts.WriteTimeout = time.Duration(writeTimeoutSec) * time.Second
	opts.IdleTimeout = time.Duration(idleTimeoutSec) * time.Second
	opts.ShutdownTimeout = time.Duration(shutdownTimeoutSec) * time.Second

	return *opts, nil
}

func applyEnv(opts *cart.ServerOptions) {
	if v := os.Getenv("DUPLI1_CART_ADDR"); v != "" {
		opts.Addr = v
	}
	if v := os.Getenv("DUPLI1_PRODUCT_URL"); v != "" {
		opts.ProductURL = v
	}
	if v := os.Getenv("DUPLI1_INVENTORY_URL"); v != "" {
		opts.InventoryURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		opts.JWTSecret = v
	}
	if v := os.Getenv("AUTH_JWKS_URL"); v != "" {
		opts.JWKSURL = v
	}
	if v := os.Getenv("DUPLI1_CART_DB"); v != "" {
		opts.DatabaseConnString = v
	} else if v := os.Getenv("DB_URL"); v != "" {
		opts.DatabaseConnString = v
	}
}

func splitAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if addr == "" {
			return "", 8086, nil
		}
		return "", 0, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
