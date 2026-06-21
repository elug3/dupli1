# AGENTS.md

Guidance for AI agents working in the Schick repository.

## Repository status

Schick is a Go microservice backend for a fashion bag marketplace. The repo contains:

- Six HTTP services under `cmd/schick-*`
- Service packages under `pkg/*` (auth, product, inventory, order, notification)
- Docker Compose for local development
- Terraform and GitHub Actions for AWS ECS deployment

See [docs/current-state.md](docs/current-state.md) for the authoritative snapshot of what is implemented today.

## Cursor Cloud specific instructions

### Prerequisites (pre-installed on the VM)

| Tool | Version / notes |
|------|-----------------|
| Go | 1.22+ (`/usr/bin/go`) — workspace uses Go 1.26 |
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

Gateway: `https://localhost` (self-signed cert; use `curl -k` or trust `certs/server.crt`).

Direct service ports (bypassing proxy):

| Service | Port |
|---------|------|
| `schick-auth` | 8080 |
| `schick-product` | 8081 |
| `schick-inventory` | 8082 |
| `schick-order` | 8083 |
| `schick-notification` | 8084 |
| `schick-proxy` | 80 / 443 |

### Running a single service (without Docker)

```bash
# From repo root — uses go.work
cd pkg/auth && go test ./...
cd pkg/product && go test ./...

# Auth server
cd cmd/schick-auth && go run . -help

# Product server
cd cmd/schick-product && go run . -help
```

### Testing

Run tests per module (root `go test ./...` does not work because the root `go.mod` is a workspace stub):

```bash
cd pkg/auth && go test ./...
cd pkg/product && go test ./...
cd pkg/inventory && go test ./...
cd pkg/order && go test ./...
```

### Gotchas

- **Module paths differ:** auth/inventory/order use `github.com/elug3/schick/pkg/...`; product uses `github.com/schick/pkg/product`.
- **Inventory and order** use in-memory stores today (no Postgres).
- **Notification** is a health-only stub; no outbound messaging yet.
- **Redis and NATS** are optional for auth (session cache and event publishing); not wired in Docker Compose by default.
- **SMTP, payment, and OAuth providers** are external; no local mocks are bundled.
