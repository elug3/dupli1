package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	auth "github.com/elug3/dupli1/auth/pkg"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Options configures the dupli1-auth executable.
type Options = auth.ServerOptions

func newRootCmd() *cobra.Command {
	root := newServeCmd()
	root.Use = "dupli1-auth"
	root.Short = "Auth server for the Dupli1 platform"
	root.AddCommand(newResetCmd())
	return root
}

func newServeCmd() *cobra.Command {
	opts := auth.NewServerOptions()
	applyEnv(opts)

	host, port, _ := splitAddr(opts.Addr)

	var (
		addrFlag           string
		hostFlag           = host
		portFlag           = port
		publicAddr         = opts.PublicAddr
		dbURL              = opts.DBURL
		redisURL           = opts.RedisURL
		natsURL            = opts.NATSURL
		jwtPrivateKeyFile  = opts.JWTPrivateKeyFile
		jwtKeyID           = opts.JWTKeyID
		readTimeoutSec     = int(opts.ReadTimeout / time.Second)
		writeTimeoutSec    = int(opts.WriteTimeout / time.Second)
		idleTimeoutSec     = int(opts.IdleTimeout / time.Second)
		shutdownTimeoutSec = int(opts.ShutdownTimeout / time.Second)
		tokenExpiry        = opts.TokenExpiry.String()
		refreshTokenExpiry = opts.RefreshTokenExpiry.String()
		corsOrigins        = strings.Join(opts.CORSOrigins, ",")
		logOutput          = opts.LogOutput
		logLevel           = opts.LogLevel
		ownerEmail         = opts.OwnerEmail
		ownerPassword      = opts.OwnerPassword
		webServiceEmail    = opts.WebServiceEmail
		webServicePassword = opts.WebServicePassword
	)

	cmd := &cobra.Command{
		Short:         "Start the HTTP server (default command)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			buildOpts(opts, flags, addrFlag, hostFlag, portFlag, publicAddr, dbURL, redisURL, natsURL,
				jwtPrivateKeyFile, jwtKeyID,
				readTimeoutSec, writeTimeoutSec, idleTimeoutSec, shutdownTimeoutSec, tokenExpiry, refreshTokenExpiry, corsOrigins,
				logOutput, logLevel, ownerEmail, ownerPassword, webServiceEmail, webServicePassword)

			if err := opts.Validate(); err != nil {
				return err
			}

			srv, err := auth.NewServer(*opts)
			if err != nil {
				fmt.Println("stopping")
				return err
			}

			interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			runErr := make(chan error, 1)
			go func() { runErr <- srv.Run() }()

			select {
			case err := <-runErr:
				return err
			case <-interrupt.Done():
			}

			srv.StopAndWait()
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&hostFlag, "host", hostFlag, "Server host address")
	f.IntVar(&portFlag, "port", portFlag, "Server port number")
	f.StringVar(&addrFlag, "addr", "", "Server listen address (overrides host/port)")
	f.StringVar(&publicAddr, "public-addr", publicAddr, "Publicly reachable base URL")
	f.StringVar(&dbURL, "db", dbURL, "Database connection URL")
	f.StringVar(&redisURL, "redis", redisURL, "Redis connection URL")
	f.StringVar(&natsURL, "nats", natsURL, "NATS connection URL")
	f.StringVar(&jwtPrivateKeyFile, "jwt-private-key-file", jwtPrivateKeyFile, "Path to PEM-encoded RSA private key for RS256/JWKS mode")
	f.StringVar(&jwtKeyID, "jwt-key-id", jwtKeyID, "Key ID (kid) to use in the JWKS document")
	f.IntVar(&readTimeoutSec, "read-timeout", readTimeoutSec, "Read timeout in seconds")
	f.IntVar(&writeTimeoutSec, "write-timeout", writeTimeoutSec, "Write timeout in seconds")
	f.IntVar(&idleTimeoutSec, "idle-timeout", idleTimeoutSec, "Idle timeout in seconds")
	f.IntVar(&shutdownTimeoutSec, "shutdown-timeout", shutdownTimeoutSec, "Graceful shutdown timeout in seconds")
	f.StringVar(&tokenExpiry, "token-expiry", tokenExpiry, "Access token lifetime")
	f.StringVar(&refreshTokenExpiry, "refresh-token-expiry", refreshTokenExpiry, "Refresh token lifetime")
	f.StringVar(&opts.CookieName, "cookie-name", opts.CookieName, "Session cookie name")
	f.BoolVar(&opts.CookieSecure, "cookie-secure", opts.CookieSecure, "Set Secure flag on session cookies")
	f.BoolVar(&opts.CookieHTTPOnly, "cookie-http-only", opts.CookieHTTPOnly, "Set HttpOnly flag on session cookies")
	f.StringVar(&corsOrigins, "cors-origins", corsOrigins, "Comma-separated CORS allowed origins")
	f.IntVar(&opts.MaxConns, "max-conns", opts.MaxConns, "Maximum concurrent connections")
	f.BoolVar(&opts.Debug, "debug", opts.Debug, "Enable debug mode")
	f.StringVar(&logOutput, "log-output", logOutput, "Log output format: json or text")
	f.StringVar(&logLevel, "log-level", logLevel, "Log level: debug, info, warn, error")
	f.StringVar(&ownerEmail, "owner-email", ownerEmail, "Email for the initial owner account (seeded on first startup)")
	f.StringVar(&ownerPassword, "owner-password", ownerPassword, "Password for the initial owner account")
	f.StringVar(&webServiceEmail, "web-service-email", webServiceEmail, "Email for the dupli1-web service account (seeded on first startup)")
	f.StringVar(&webServicePassword, "web-service-password", webServicePassword, "Password for the dupli1-web service account")

	return cmd
}

func buildOpts(
	opts *Options,
	flags *pflag.FlagSet,
	addrFlag string, hostFlag string, portFlag int,
	publicAddr, dbURL, redisURL, natsURL string,
	jwtPrivateKeyFile, jwtKeyID string,
	readTimeoutSec, writeTimeoutSec, idleTimeoutSec, shutdownTimeoutSec int,
	tokenExpiry, refreshTokenExpiry, corsOrigins string,
	logOutput, logLevel string,
	ownerEmail, ownerPassword string,
	webServiceEmail, webServicePassword string,
) {
	_ = flags

	if addrFlag != "" {
		opts.Addr = addrFlag
	} else {
		opts.Addr = net.JoinHostPort(hostFlag, strconv.Itoa(portFlag))
	}

	opts.PublicAddr = publicAddr
	opts.DBURL = dbURL
	opts.RedisURL = redisURL
	opts.NATSURL = natsURL
	opts.JWTPrivateKeyFile = jwtPrivateKeyFile
	opts.JWTKeyID = jwtKeyID

	opts.ReadTimeout = time.Duration(readTimeoutSec) * time.Second
	opts.WriteTimeout = time.Duration(writeTimeoutSec) * time.Second
	opts.IdleTimeout = time.Duration(idleTimeoutSec) * time.Second
	opts.ShutdownTimeout = time.Duration(shutdownTimeoutSec) * time.Second

	if tokenExpiry != "" {
		if d, err := time.ParseDuration(tokenExpiry); err == nil {
			opts.TokenExpiry = d
		}
	}
	if refreshTokenExpiry != "" {
		if d, err := time.ParseDuration(refreshTokenExpiry); err == nil {
			opts.RefreshTokenExpiry = d
		}
	}
	if corsOrigins != "" {
		opts.CORSOrigins = strings.Split(corsOrigins, ",")
	}
	opts.LogOutput = logOutput
	opts.LogLevel = logLevel
	opts.OwnerEmail = ownerEmail
	opts.OwnerPassword = ownerPassword
	opts.WebServiceEmail = webServiceEmail
	opts.WebServicePassword = webServicePassword
}

func applyEnv(opts *auth.ServerOptions) {
	if v := os.Getenv("DUPLI1_AUTH_ADDR"); v != "" {
		opts.Addr = v
	}
	if host := os.Getenv("SERVER_HOST"); host != "" {
		port := os.Getenv("SERVER_PORT")
		if port == "" {
			port = "8080"
		}
		opts.Addr = net.JoinHostPort(host, port)
	}
	if v := os.Getenv("DUPLI1_AUTH_PUBLIC_ADDR"); v != "" {
		opts.PublicAddr = v
	}
	if v := os.Getenv("DB_URL"); v != "" {
		opts.DBURL = v
	}
	if v := os.Getenv("REDIS_URL"); v != "" {
		opts.RedisURL = v
	}
	if v := os.Getenv("DUPLI1_AUTH_NATS_URL"); v != "" {
		opts.NATSURL = v
	} else if v := os.Getenv("NATS_URL"); v != "" {
		opts.NATSURL = v
	}
	if v := os.Getenv("JWT_PRIVATE_KEY_FILE"); v != "" {
		opts.JWTPrivateKeyFile = v
	}
	if v := os.Getenv("JWT_KEY_ID"); v != "" {
		opts.JWTKeyID = v
	}
	if v := os.Getenv("DUPLI1_AUTH_DEBUG"); v != "" {
		opts.Debug = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("OWNER_EMAIL"); v != "" {
		opts.OwnerEmail = v
	}
	if v := os.Getenv("OWNER_PASSWORD"); v != "" {
		opts.OwnerPassword = v
	}
	if v := os.Getenv("DUPLI1_WEB_SERVICE_EMAIL"); v != "" {
		opts.WebServiceEmail = v
	}
	if v := os.Getenv("DUPLI1_WEB_SERVICE_PASSWORD"); v != "" {
		opts.WebServicePassword = v
	}

	setDurationEnv(&opts.ReadTimeout, "DUPLI1_AUTH_READ_TIMEOUT")
	setDurationEnv(&opts.WriteTimeout, "DUPLI1_AUTH_WRITE_TIMEOUT")
	setDurationEnv(&opts.IdleTimeout, "DUPLI1_AUTH_IDLE_TIMEOUT")
	setDurationEnv(&opts.ShutdownTimeout, "DUPLI1_AUTH_SHUTDOWN_TIMEOUT")
	setDurationEnv(&opts.TokenExpiry, "JWT_EXPIRATION")
	setDurationEnv(&opts.RefreshTokenExpiry, "DUPLI1_AUTH_REFRESH_TOKEN_EXPIRY")
	if v := os.Getenv("DUPLI1_AUTH_LOG_OUTPUT"); v != "" {
		opts.LogOutput = v
	}
	if v := os.Getenv("DUPLI1_AUTH_LOG_LEVEL"); v != "" {
		opts.LogLevel = v
	}
}

func setDurationEnv(target *time.Duration, key string) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}

func splitAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if addr == "" {
			return "", 8080, nil
		}
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
