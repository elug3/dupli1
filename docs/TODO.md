# TODO

## Quality / performance (reviewed 2026-07-15)

Full write-up: [quality-performance-review.md](quality-performance-review.md).

### Fixed in review PR

- [x] Checkout `DELETE …/items/by-sku-id/{id}` ownership ABAC
- [x] Payment `CompletePayment` republishes `payment.succeeded` after prior publish failure
- [x] Gate `simulate-success` behind dev-only flag (Stripe unset)
- [x] Product search pagination (`limit`/`offset`) + filter indexes
- [x] Order list / expiry batch item load + pending `payment_due_at` index
- [x] Cart enrichment parallelized (bounded concurrency)

### Still open (priority)

- [ ] **Product images CDN** — apply CloudFront + OAC Terraform; rewrite existing `imageUrls` hosts if needed ([product-images-browser-access.md](product-images-browser-access.md))
- [ ] **Server-side order/checkout pricing** — ignore client `unit_price_cents`; resolve from product
- [ ] Inventory service token refresh in order bootstrap
- [ ] NATS handler errors / outbox for payment→order events
- [ ] Batch cart/product APIs (`?sku_ids=`); Redis catalog cache
- [ ] Plumb request `context` through product PG stores; sanitize 500 responses
- [ ] Consolidate duplicated `authjwt` into `shared/`

## Product API

- [x] **Parent + variants** — implemented; see [product-variants-plan.md](product-variants-plan.md). Remaining: inventory `inStock` enrichment on PDP, drop legacy parent `color`/`stock`/`imageUrls` columns, merge pre-existing duplicate color products.
- [ ] **Auth-aware `GET /api/v1/products/{id}`** — managers should see drafts/cost on PDP without a separate `/manage` path (optional Bearer, same pattern as list search).
- [ ] **Guest session cookie + unique product view counter** — plan in [product-guest-views-plan.md](product-guest-views-plan.md). Browser `dupli1_guest` cookie; exact unique views per parent product on public PDP.

### Found in review (2026-07-08, size/color variants)

- [x] **`product/pkg/handler` test package doesn't build** — fixed as part of the fine-grained-permissions rewrite (`access_control_test.go` was rewritten against the current API); `go test ./...` builds and passes, including `TestCreateVariant` and `TestSearchProductsByColor`.
- [x] **`UpdateVariant` silently clears omitted fields** — fixed with merge-on-update semantics: `domain.Variant.MergeUpdate` (`product/pkg/domain/enrich.go`) applies only the non-zero-value fields from the request onto the existing row, and `ProductSearchService.UpdateVariant` fetches the existing variant and merges before writing. A price-only `PUT` now keeps `color`/`size`/`status`/`imageUrls` untouched instead of blanking them; status specifically keeps its current value rather than resetting to `"active"`, so an update can't accidentally reactivate a deliberately-archived variant. The store layer (`infra/pg/variant_store.go`, `infra/memory/product_store.go`) is unchanged — it still writes whatever full struct it's given, which is now always the merged one.
- [x] **Variant SKU auto-naming (`optionCode`) differs between stores** — fixed by extracting `domain.OptionCode`/`domain.BuildVariantSKUBase` as shared helpers (`product/pkg/domain/skuid.go`) used by both `infra/pg/variant_store.go` and `infra/memory/product_store.go`; no longer possible to diverge.
- [x] **Luxury SKU naming system** — `Brand_Style_Color[_Edition]_Size` with master tables (`brands`, `colors`, `sizes`, `sku_editions`); see [product-sku-system.md](product-sku-system.md).
- [x] **SKU master-data runtime CRUD** — Phase A+B+C: styles table, FKs, catalog APIs (`/api/v1/catalog/...`), `product.master.read|write`, ULID product `id`, strict master codes on product/variant create, read-name enrichment; see [product-sku-master-data-plan.md](product-sku-master-data-plan.md). Phase D (admin UI) remains.

### Found while implementing SkuID + inventory merge (2026-07-10)

- [ ] **Frontend repos (`dupli1-web`, `dupli1-manage-web`) not yet migrated to `skuId`** — PDP variant JSON exposes both `sku` and `skuId`; storefront and admin clients still only send/read `sku`. See the "SkuId migration" section in [frontend-product-variants-migration.md](frontend-product-variants-migration.md).

## AWS deployment readiness (reviewed 2026-07-13)

Architecture is suitable (ECS on EC2 + ALB + RDS + Terraform + GitHub Actions). See [deployment-aws.md](deployment-aws.md).

### Working today (live ALB)

- [x] Gateway health, auth, product catalog
- [x] Storefront (`dupli1-web`) and admin (`dupli1-manage-web`) ECS services
- [x] Redis, NATS, NAT, Secrets Manager (auth/product DB URLs), Cloud Map `dupli1.local`
- [x] **Cart + payment on ECS** — ECR repos, Cloud Map (`cart` / `payment`), task defs, RDS DBs + secrets, nginx upstreams; APIs return 401 without auth (not 502)
- [x] **Order stabilized** — listens on `:8080`, `DUPLI1_ORDER_DB` from Secrets Manager; ASG sized for awsvpc ENI limits
- [x] **HTTPS on ALB** — ACM cert + `:443` listener; `/api/*` + `/gateway/*` → proxy
- [x] **Route53 → current ALB** — `dupli1.com` / `www` alias `dupli1-production-alb`
- [x] **JWT_SECRET in Secrets Manager** — no longer plain env default in task defs
- [x] **Orphan `dupli1-inventory` Fargate service removed**
- [x] **Docs updated** — [deployment-aws.md](deployment-aws.md) lists cart/payment/frontends/RDS DBs

### Remaining

- [ ] **Manager settings API** — sketch in [manager-settings-api.md](manager-settings-api.md) (`GET|PATCH /api/v1/settings/{section}`).
- [x] **Enable `awsvpcTrunking` for the ECS instance role** — confirmed live on container instances (2026-07-14); ASG Terraform defaults lowered to 2/1/4.
- [ ] **Align `dupli1-web` / `dupli1-manage-web` CI task defs with live Terraform** — workflows still use Fargate/`awsvpc`/`web-container`; live storefront is EC2 `bridge` / family `dupli1-web` / container `web`.
- [ ] **Prefer OIDC for backend CI** — replace long-lived `AWS_ACCESS_KEY_ID` secrets with `github-actions-deploy-role` (frontends already use OIDC).
- [ ] **HTTP→HTTPS redirect on ALB `:80` default action** — Terraform models redirect; live still serves HTTP for health/clients (API rule intact).
- [ ] **Apply cost cleanup** — follow [aws-cost-reduction-plan.md](aws-cost-reduction-plan.md) (Phases 1–2); script: `infra/scripts/cleanup-aws-orphans.sh`. Evidence: [aws-cost-optimization.md](aws-cost-optimization.md).