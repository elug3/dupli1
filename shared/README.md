# Dupli1 shared libraries

Reusable Go modules consumed by Dupli1 microservices. Prefer small packages with minimal dependencies so services can import only what they need (`permissions` / `settings` / `money` stay dependency-free; `authjwt` pulls JWT + `singleflight`).

## Module

```
github.com/elug3/dupli1/shared
```

## Packages

### `pkg/authjwt`

Shared access-token validation (JWKS RS256 or HMAC HS256), claims helpers, and request-context plumbing. JWKS refresh uses `singleflight` so concurrent unknown-`kid` / cold-cache requests share one fetch.

```go
import "github.com/elug3/dupli1/shared/pkg/authjwt"

validator, err := authjwt.NewAccessTokenValidator(jwksURL, hmacSecret)
claims, err := validator.ValidateAccessToken(token)
ctx = authjwt.WithClaims(ctx, claims)
```

### `pkg/permissions`

Fine-grained permission constants, wildcard evaluation, legacy role expansion for DB migration, and named bundles. See [docs/permissions.md](../docs/permissions.md) for the full specification.

```go
import "github.com/elug3/dupli1/shared/pkg/permissions"

// Check a single permission (supports *, admin.*, product.* wildcards).
permissions.Has("product.create", []string{"product.*"}) // true

// Check any of several required permissions.
permissions.HasAny(held, "product.create", "product.update")

// Expand legacy RBAC roles when migrating stored user data.
permissions.ExpandLegacyRoles([]string{"product_manager"}) // ["coupon.*", "product.*"]

// Apply a named bundle preset before saving a user.
permissions.ExpandBundle("catalog_editor")
```

### `pkg/settings`

Shared non-secret settings response helpers for `GET /settings` and `GET /api/v1/<service>/settings` on every service. Builds a JSON payload with service name, auth mode, storage backend, feature flags, and dependency hostnames — never secrets or DSNs.

```go
import "github.com/elug3/dupli1/shared/pkg/settings"

resp := settings.NewResponse("order")
resp.Auth = settings.ConsumerAuth(jwksURL, jwtSecret)
resp.Storage = settings.StorageMode(dbURL)
mux.HandleFunc("/settings", settings.Handler(resp))
```

### Testing

```bash
cd shared && go test ./...
```

### Adding a dependency in a service

```bash
cd auth  # or product, order, …
go get github.com/elug3/dupli1/shared@latest
```

For local development in this monorepo (no published tag yet):

```bash
go mod edit -replace github.com/elug3/dupli1/shared=../shared
```

Remove the `replace` directive before release builds that pin a version tag.
