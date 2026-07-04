# TODO

## Product API

- [ ] **Restore admin single-product read** — `GET /api/v1/products/{id}/manage` was removed for REST consistency. Public `GET /api/v1/products/{id}` only returns active products with cost redacted. Managers currently rely on authenticated `GET /api/v1/products` (optional Bearer token with `product_manager` / `admin` / `owner`) to see drafts and cost. Prefer an auth-aware `GET /api/v1/products/{id}` that returns the full product for managers instead of reintroducing `/manage`.
