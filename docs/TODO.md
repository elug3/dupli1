# TODO

## Product API

- [x] **Parent + variants** — implemented; see [product-variants-plan.md](product-variants-plan.md). Remaining: inventory `inStock` enrichment on PDP, drop legacy parent `color`/`stock`/`imageUrls` columns, merge pre-existing duplicate color products.
- [ ] **Auth-aware `GET /api/v1/products/{id}`** — managers should see drafts/cost on PDP without a separate `/manage` path (optional Bearer, same pattern as list search).

### Found in review (2026-07-08, size/color variants)

- [x] **`product/pkg/handler` test package doesn't build** — fixed as part of the fine-grained-permissions rewrite (`access_control_test.go` was rewritten against the current API); `go test ./...` builds and passes, including `TestCreateVariant` and `TestSearchProductsByColor`.
- [ ] **`UpdateVariant` silently clears omitted fields** — `PUT /api/v1/products/{id}/variants/{sku}` does a full-column overwrite in both `infra/pg/variant_store.go` and `infra/memory/product_store.go`, with no merge against the existing row and no default for empty `status` (unlike `CreateVariant`). A partial body (e.g. price-only) silently wipes `color`/`size` and can drop `status` to `""`, which removes the variant from active/public filtering (PDP, search, `GetPublicVariant`). Needs either merge-on-update semantics or an enforced full-body contract plus a status default. Still open as of the SkuID/inventory-merge work — that work only changed the `RETURNING`/scan clause to also fetch `sku_id`, not the overwrite semantics.
- [x] **Variant SKU auto-naming (`optionCode`) differs between stores** — fixed by extracting `domain.OptionCode`/`domain.BuildVariantSKUBase` as shared helpers (`product/pkg/domain/skuid.go`) used by both `infra/pg/variant_store.go` and `infra/memory/product_store.go`; no longer possible to diverge.

### Found while implementing SkuID + inventory merge (2026-07-10)

- [ ] **Frontend repos (`dupli1-web`, `dupli1-manage-web`) not yet migrated to `skuId`** — PDP variant JSON exposes both `sku` and `skuId`; storefront and admin clients still only send/read `sku`. See the "SkuId migration" section in [frontend-product-variants-migration.md](frontend-product-variants-migration.md).
