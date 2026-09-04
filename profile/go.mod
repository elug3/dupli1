module github.com/elug3/dupli1/profile

go 1.26.3

require (
	github.com/elug3/dupli1/shared v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.12.3
	github.com/nats-io/nats.go v1.52.0
)

require (
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

replace github.com/elug3/dupli1/shared => ../shared
