# Dupli1 shared libraries

Reusable Go modules consumed by Dupli1 microservices. Each package is intentionally small and dependency-free so services can import only what they need.

## Module

```
github.com/elug3/dupli1/shared
```

## Packages

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
