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

- [ ] **Enable `awsvpcTrunking` for the ECS instance role** — account default is enabled, but instance-role principal still needs it (root/admin) so ASG can shrink below ~5×`t3.large`.
- [ ] **Align `dupli1-web` / `dupli1-manage-web` CI task defs with live Terraform** — workflows still use Fargate/`awsvpc`/`web-container`; live storefront is EC2 `bridge` / family `dupli1-web` / container `web`.
- [ ] **Prefer OIDC for backend CI** — replace long-lived `AWS_ACCESS_KEY_ID` secrets with `github-actions-deploy-role` (frontends already use OIDC).
- [ ] **HTTP→HTTPS redirect on ALB `:80` default action** — Terraform models redirect; live still serves HTTP for health/clients (API rule intact).
