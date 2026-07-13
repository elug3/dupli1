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

Architecture is suitable (ECS on EC2 + ALB + RDS + Terraform + GitHub Actions). Live account already serves auth, product, storefront, and manage-web. Full marketplace checkout is **not** production-ready yet. See [deployment-aws.md](deployment-aws.md) and [`infra/terraform/`](../infra/terraform/README.md).

### Working today (live ALB)

- [x] Gateway health, auth, product catalog
- [x] Storefront (`dupli1-web`) and admin (`dupli1-manage-web`) ECS services
- [x] Redis, NATS, NAT, Secrets Manager (auth/product DB URLs), Cloud Map `dupli1.local`

### Blockers — cart / payment / order

- [ ] **Create ECR repos `dupli1-cart` and `dupli1-payment`** — `.github/workflows/aws.yml` builds them, but push fails (`RepositoryNotFoundException`); `main` AWS workflow is red.
- [ ] **Add Terraform ECS services + Cloud Map for cart and payment** — `infra/terraform/ecs_services.tf` has auth/product/order/notification/proxy/redis/nats only; no cart/payment task defs or services.
- [ ] **Wire RDS DB secrets for order, cart, payment** — Secrets Manager has auth + product URLs only. Order task has **no** `DUPLI1_ORDER_DB` (in-memory fallback in prod). Create secrets and inject like auth/product; ensure `orders` / `cart` / `payments` DBs exist (`infra/scripts/create-rds-databases.sh`).
- [ ] **Fix ECS nginx cart/payment upstreams** — Live proxy logs: cart/payments resolve to `web.dupli1.local` → 502. Repo `api/nginx.ecs.conf` omits cart/payment; `api/nginx.ecs.conf.template` has them. Align deployed gateway with real Cloud Map names (`cart` / `payment` or `dupli1-cart` / `dupli1-payment`).
- [ ] **Stabilize order service** — `/api/v1/orders` → 502 (connection refused). ECS events show `RESOURCE:ENI` placement failures; pending tasks while capacity is exhausted.

### Frontend CI / task-definition mismatch

- [ ] **Align `dupli1-web` deploy with live Terraform service** — Workflow (`.aws/task-definition.json`) uses family `dupli1-web-task`, container `web-container`, `awsvpc`; live service is family `dupli1-web`, container `web`, `bridge`. Deploy fails: `Network Configuration must be provided when networkMode 'awsvpc' is specified.`
- [ ] **Align `dupli1-manage-web` the same way** — Same pattern (OIDC role + task-def file vs Terraform-managed `dupli1-manage-web`).

### Networking / DNS / TLS

- [ ] **Point Route53 at the current ALB** — `dupli1.com` / `www` still alias `dupli1-prod-alb-...`; live stack ALB is `dupli1-production-alb-...`.
- [ ] **Add HTTPS listener (ACM)** — Cert for `dupli1.com` is issued; ALB only has HTTP:80.

### Capacity / cost / cleanup

- [ ] **Fix ENI / ASG drift** — ASG desired≈2 but multiple `t3.large` instances (some DRAINING); awsvpc tasks fail with `RESOURCE:ENI`. Reclaim drained instances or raise capacity / reduce awsvpc task density (web already uses bridge to save ENIs).
- [ ] **Delete orphan `dupli1-inventory` Fargate service** — desired 0; inventory merged into product.
- [ ] **Rotate placeholder `JWT_SECRET`** — still `dupli1-prod-jwt-change-me` in task defs; move to Secrets Manager.

### Docs / security hygiene

- [ ] **Update [deployment-aws.md](deployment-aws.md)** — Still lists inventory as a service; omits cart/payment and frontends; RDS DB list incomplete vs `create-rds-databases.sh`.
- [ ] **Stop using long-lived admin IAM user keys for agents/CI** — Prefer the frontends' OIDC role (`github-actions-deploy-role`); rotate any keys that were shared outside Secrets Manager.
