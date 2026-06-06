# Schick Architecture Guide

This document defines the mandatory architecture and directory structure for all services in Schick.

Every service MUST follow this structure.

## Philosophy

Schick uses:

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

Every service must follow this exact structure.

```
service-name/

├── server.go
├── config.go
├── errors.go
│
├── domain/
├── service/
├── ports/
├── infra/
├── handler/
└── runtime/
```

### Example:

```
auth/

├── server.go
├── config.go
├── errors.go
│
├── domain/
│   └── user.go
│
├── service/
│   └── login.go
│
├── ports/
│   ├── repository.go
│   └── token.go
│
├── infra/
│   ├── postgres/
│   ├── redis/
│   └── jwt/
│
├── handler/
│   └── http.go
│
└── runtime/
    └── bootstrap.go
```

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

## Runtime Rules

Runtime wires dependencies together.

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

Reusable components belong in:

```
pkg/shared/
```

### Examples:

- pkg/shared/db
- pkg/shared/logger
- pkg/shared/cache
- pkg/shared/config

Business logic must never be placed in shared.

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
