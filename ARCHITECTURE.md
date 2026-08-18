# Dupli1 Architecture Guide

This document defines the mandatory architecture and directory structure for all services in Dupli1.

Every service MUST follow this structure.

## Philosophy

Dupli1 uses:

- Domain Driven Design (DDD)
- Hexagonal Architecture
- Ports and Adapters
- Dependency Inversion

Business logic must never depend on infrastructure.

Dependencies must flow inward.

```
Handler
    ↓
Service
    ↓
Ports
    ↓
Domain

Infra implements Ports
```

---

## Service Layout

Every service lives in its own module directory with `cmd/` (entrypoint) and `pkg/` (hexagonal layers):

```
service-name/
├── cmd/                      # CLI / process entrypoint
│   └── main.go
├── go.mod
└── pkg/
    ├── server.go             # HTTP server wiring (location may vary)
    ├── domain/
    ├── service/
    ├── ports/
    ├── infra/
    ├── handler/
    └── bootstrap/
```

### Example:

```
auth/
├── cmd/
│   └── main.go
├── go.mod
└── pkg/
    ├── server.go
    ├── options.go
    ├── domain/
    │   └── user.go
    ├── service/
    │   └── service.go
    ├── ports/
    │   ├── repository.go
    │   └── token.go
    ├── infra/
    │   ├── postgres/
    │   ├── redis/
    │   └── jwt/
    ├── handler/
    │   └── handler.go
    └── bootstrap/
        ├── bootstrap.go
        └── config.go
```

Note: `config.go` and `errors.go` may live in `bootstrap/` or a dedicated `autherrors/` package. Existing services use `options.go` under `pkg/` and `bootstrap/config.go` for wiring config. See [docs/service-layout.md](docs/service-layout.md).

---

## Domain Rules

Domain contains business entities and business rules.

### Allowed:

- structs
- value objects
- domain methods

### Forbidden:

- SQL
- Redis
- HTTP
- JWT
- gRPC
- External SDKs

### Example:

```go
type User struct {
    ID string
    Email string
}
```

---

## Service Rules

Service contains use cases.

### Examples:

- Login
- Register
- CreateOrder
- UpdateInventory

Service may depend only on:

- domain
- ports

Service MUST NOT depend on:

- infra
- handler

---

## Ports Rules

Ports define required interfaces.

### Example:

```go
type UserRepository interface {
    FindByEmail(
        ctx context.Context,
        email string,
    ) (*domain.User, error)
}
```

Ports contain no implementation.

---

## Infra Rules

Infra implements Ports.

### Examples:

- infra/postgres
- infra/redis
- infra/jwt
- infra/email

Infra may depend on external libraries.

Infra MUST NOT contain business logic.

---

## Handler Rules

Handlers translate external requests.

### Examples:

- HTTP
- gRPC
- CLI

Handlers:

- Validate input
- Call service
- Return response

Handlers MUST NOT contain business logic.

---

## Bootstrap Rules

Bootstrap wires dependencies together.

### Responsibilities:

- Create database connections
- Create repositories
- Create services
- Create handlers
- Start server

### Example:

```go
repo := postgres.NewUserRepository(db)
svc := service.New(repo)
h := handler.New(svc)
```

---

## Shared Code

Reusable components belong in the **`shared/`** module (`github.com/elug3/dupli1/shared`):

```
shared/
├── go.mod
└── pkg/
    ├── permissions/    # Fine-grained permission constants and helpers
    ├── settings/       # Shared GET /settings response helpers
    └── authjwt/        # JWKS / JWT validation helpers (where extracted)
```

Local services typically `replace` the module to `../shared`. Business logic must never be placed in shared.

---

## Dependency Rules

### Allowed:

- handler -> service
- service -> ports
- infra -> ports
- service -> domain

### Forbidden:

- domain -> infra
- domain -> handler
- service -> handler
- service -> postgres
- service -> redis

---

## AI Agent Instructions

When generating new code:

1. Follow the directory structure exactly.
2. Never place business logic in handlers.
3. Never place business logic in infra.
4. Define interfaces in ports.
5. Implement interfaces in infra.
6. Keep domain independent from infrastructure.
7. Use dependency injection.
8. Avoid global state.
9. Write unit tests for services.
10. Maintain backward compatibility whenever possible.
