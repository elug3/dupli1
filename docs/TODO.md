# TODO

## Product API

- [x] **Parent + variants** — implemented; see [product-variants-plan.md](product-variants-plan.md). Remaining: inventory `inStock` enrichment on PDP, drop legacy parent `color`/`stock`/`imageUrls` columns, merge pre-existing duplicate color products.
- [ ] **Auth-aware `GET /api/v1/products/{id}`** — managers should see drafts/cost on PDP without a separate `/manage` path (optional Bearer, same pattern as list search).

### Found in review (2026-07-08, size/color variants)

- [ ] **`product/pkg/handler` test package doesn't build** — `access_control_test.go`, `legacy_path_test.go`, and `route_conflict_test.go` still reference API removed by the parent+variants rewrite (`RouteManageProduct`, `RouteSearchBags`, `RouteProductImage`, `ListProductsHandler`, `GetProductHandler`, `domain.Product.Cost`). `go test ./...` fails to build the package, so none of its tests run — including the color/size coverage in `handler_test.go` (`TestCreateVariant`, `TestSearchProductsByColor`, etc.). Fix or delete the stale tests.
- [ ] **`UpdateVariant` silently clears omitted fields** — `PUT /api/v1/products/{id}/variants/{sku}` does a full-column overwrite in both `infra/pg/variant_store.go` and `infra/memory/product_store.go`, with no merge against the existing row and no default for empty `status` (unlike `CreateVariant`). A partial body (e.g. price-only) silently wipes `color`/`size` and can drop `status` to `""`, which removes the variant from active/public filtering (PDP, search, `GetPublicVariant`). Needs either merge-on-update semantics or an enforced full-body contract plus a status default.
- [ ] **Variant SKU auto-naming (`optionCode`) differs between stores** — `infra/pg/variant_store.go` pads short color/size codes to 3 characters with `X`; `infra/memory/product_store.go` does not. Cosmetic (SKUs stay unique either way), but tests against the in-memory store don't reflect the SKU shape Postgres actually produces. Worth deduplicating.
