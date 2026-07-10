# TODO

## Product API

- [x] **Parent + variants** — implemented; see [product-variants-plan.md](product-variants-plan.md). Remaining: inventory `inStock` enrichment on PDP, drop legacy parent `color`/`stock`/`imageUrls` columns, merge pre-existing duplicate color products.
- [ ] **Auth-aware `GET /api/v1/products/{id}`** — managers should see drafts/cost on PDP without a separate `/manage` path (optional Bearer, same pattern as list search).

### Found in review (2026-07-08, size/color variants)

- [x] **`product/pkg/handler` test package doesn't build** — fixed as part of the fine-grained-permissions rewrite (`access_control_test.go` was rewritten against the current API); `go test ./...` builds and passes, including `TestCreateVariant` and `TestSearchProductsByColor`.
- [x] **`UpdateVariant` silently clears omitted fields** — fixed with merge-on-update semantics: `domain.Variant.MergeUpdate` (`product/pkg/domain/enrich.go`) applies only the non-zero-value fields from the request onto the existing row, and `ProductSearchService.UpdateVariant` fetches the existing variant and merges before writing. A price-only `PUT` now keeps `color`/`size`/`status`/`imageUrls` untouched instead of blanking them; status specifically keeps its current value rather than resetting to `"active"`, so an update can't accidentally reactivate a deliberately-archived variant. The store layer (`infra/pg/variant_store.go`, `infra/memory/product_store.go`) is unchanged — it still writes whatever full struct it's given, which is now always the merged one.
- [x] **Variant SKU auto-naming (`optionCode`) differs between stores** — fixed by extracting `domain.OptionCode`/`domain.BuildVariantSKUBase` as shared helpers (`product/pkg/domain/skuid.go`) used by both `infra/pg/variant_store.go` and `infra/memory/product_store.go`; no longer possible to diverge.

### Found while implementing SkuID + inventory merge (2026-07-10)

- [ ] **Frontend repos (`dupli1-web`, `dupli1-manage-web`) not yet migrated to `skuId`** — PDP variant JSON exposes both `sku` and `skuId`; storefront and admin clients still only send/read `sku`. See the "SkuId migration" section in [frontend-product-variants-migration.md](frontend-product-variants-migration.md).
