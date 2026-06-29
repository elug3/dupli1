# AGENTS.md

Guidance for AI agents working in the Schick repository.

## Repository status

Schick is a Go microservice backend for a fashion bag marketplace. The repo contains:

- Five HTTP services in `auth/`, `product/`, `inventory/`, `order/`, `notification/` (each with `cmd/` + `pkg/`)
- nginx gateway in `api/` (`schick-proxy` in Docker Compose)
- Docker Compose for local development
- Terraform and GitHub Actions for AWS ECS deployment

See [docs/current-state.md](docs/current-state.md) for the authoritative snapshot of what is implemented today. See [docs/service-layout.md](docs/service-layout.md) for directory and module layout.

## Cursor Cloud specific instructions

### Prerequisites (pre-installed on the VM)

| Tool | Version / notes |
|------|-----------------|
| Go | 1.22+ (`/usr/bin/go`) — modules target Go 1.26 |
| PostgreSQL | 16 — `localhost:5432` |
| Redis | 7 — `localhost:6379` |

### Starting infrastructure services

For tests or non-Docker workflows, start system services:

```bash
sudo service postgresql start
sudo service redis-server start
pg_isready -h localhost
redis-cli ping
```

Local development normally uses Docker Compose Postgres containers instead (see below).

### Database credentials (dev)

Docker Compose provides two Postgres instances:

| Service | Host port | Database | User | Password |
|---------|-----------|----------|------|----------|
| `postgres-auth` | 5432 | `schick_db` | `schick` | `schick_dev` |
| `postgres-product` | 5433 | `products` | `schick` | `schick_dev` |

Connection strings:

- Auth: `postgres://schick:schick_dev@localhost:5432/schick_db?sslmode=disable`
- Product: `postgres://schick:schick_dev@localhost:5433/products?sslmode=disable`

Production uses **Amazon RDS** — see [docs/deployment-aws.md](docs/deployment-aws.md) and [infra/terraform/README.md](infra/terraform/README.md).

### Running locally

```bash
cp .env.example .env   # set JWT_SECRET, OWNER_EMAIL, OWNER_PASSWORD
docker compose up --build
```

Gateway (HTTP): `http://localhost:8080` or `http://localhost` (port 80). TLS certs exist in `certs/` but are not wired into local nginx yet.

Direct service ports (bypass gateway):

| Service | Host port |
|---------|-----------|
| `schick-auth` | 18080 |
| `schick-product` | 8081 |
| `schick-inventory` | 8082 |
| `schick-order` | 8083 |
| `schick-notification` | 8084 |

### Running a single service (without Docker)

```bash
cd auth && go test ./...
cd product && go test ./...

# Auth server
cd auth/cmd && go run . -help

# Product server
cd product/cmd && go run . -help
```

### Testing

Run tests per service module (root `go test ./...` does not work — the root `go.mod` is a stub):

```bash
cd auth && go test ./...
cd product && go test ./...
cd inventory && go test ./...
cd order && go test ./...
```

### Gotchas

- **Module paths:** all services use `github.com/elug3/schick/<service>` (e.g. `github.com/elug3/schick/product`). There is no top-level `go.work`.
- **Auth token flow:** login returns only a `refresh_token`; call `POST /api/v1/auth/refresh` to obtain a short-lived access token in the `token` field.
- **Product JWT:** protected routes validate RS256 via `AUTH_JWKS_URL` (set in Compose to auth's JWKS endpoint).
- **Order JWT:** when `JWT_SECRET` is set, order/checkout require Bearer tokens validated with HMAC — not aligned with auth RS256 yet.
- **Inventory and order** use in-memory stores (no Postgres).
- **Notification** is a health-only stub; no outbound messaging yet.
- **Redis and NATS** are optional for auth (rate limits, session cache, events); Redis is wired in Compose.
- **SMTP, payment, and OAuth providers** are external; no local mocks are bundled.
