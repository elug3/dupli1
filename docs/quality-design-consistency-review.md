# Review: the other services against the product API v2 plan

**Status:** Review (2026-11-04). Findings and recommendations only; no code changed. Open items are tracked in [TODO.md](TODO.md) § Design consistency.
**Measured against:** [product-api-v2-plan.md](product-api-v2-plan.md) §1 (what drifted in product) and §6.1 (the v2 conventions).
**Related:** [v1.1-release-plan.md](v1.1-release-plan.md) Slice 1 (error envelope), [api.md](api.md) § Common error shape, [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) §6, [product-structure-final-review.md](product-structure-final-review.md), [quality-performance-review.md](quality-performance-review.md).

---

## 1. Summary

The v2 plan names what drifted in product and sets conventions for a clean API. Measured against the same list, auth, order, cart, payment, notification, support, profile and `shared/` show five things:

1. **Most v2 conventions are already the house style.** Every other service speaks snake_case JSON, whole-won `int64` money named `*_won`, and `time.Time`. Product v1 is the outlier, and v2 brings it in line. That settles D1 (§3.1).
2. **Four v2 conventions exist nowhere yet:** a structured error body, cursor pagination, strict decoding with merge-patch semantics, and `ETag` / `If-Match`. Built inside product, each becomes one more dialect, and v2's error body already conflicts with the system-wide target in v1.1 Slice 1 (§3.2). Agree each shape once and put the helpers in `shared/`.
3. **v2 misses contracts outside product:** the `product.*` events, a third `productclient` consumer (support), and the parent product ids held by wishlist, recommendations, view counts, promotion conditions and support (§4). One of these contracts has a live bug: product drops the `sku_id` that order sends with every reservation (§4.1).
4. **Some of product's §1 problems recur elsewhere, in smaller form.** profile's address `PATCH` can't clear a field (the `MergeUpdate` defect), order's entity carries computed fields, and three admin lists are unbounded or silently truncated (§3).
5. **Two product docs still describe the pre-v2 design**, and v2 contradicts itself in two places (§5).

---

## 2. Scorecard

✓ matches v2 · ~ partly · ✗ has the problem v2 §1 lists for product · — no such surface

| Check (v2 §) | auth | order | cart | payment | notification | support | profile | product v1 |
|---|---|---|---|---|---|---|---|---|
| snake_case JSON (D1) | ✓ | ✓ | ✓ | ✓ ¹ | ✓ | ✓ | ✓ | ✗ mixed |
| Money: `int64`, `*_won` (§4) | — | ~ ² | ✓ | ~ ² | ~ ³ | ✓ | — | ✗ `float64` |
| Time: `time.Time`, RFC 3339 (§4) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ strings |
| Typed enums (§4) | ~ | ✓ | — | ~ | ~ | ~ | — | ✗ |
| Entity ≠ view; JSON tags off `domain` (§4) | ✓ | ✗ | ✓ | ~ | ~ | ✓ | ~ | ✗ |
| `PATCH` can keep, set and clear (§6.1) | ✓ | — | — | — | ✓ | ~ ⁴ | ✗ | ✗ |
| Unknown and read-only fields refused (§6.1) | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| Error body carries a machine code (§6.1) | ~ | ~ | ✗ | ✗ | ✗ | ~ | ✗ | ✗ |
| Lists bounded and pageable (§6.1) | ✗ | ✗ | — | — | — | ✗ | — | ~ offset |
| `handler` / `service` import no infra (CLAUDE.md) | ✓ | ✓ | ✓ | ✗ | ✗ | ✓ | ✓ | ✓ |

1. payment's camelCase keys are NANO's own wire format (`compOrderNo`, `tranNo`), not dupli1's API.
2. The fields are `*_won`, but Go parameters and locals still say `*KRW`: `amountKRW`, `discountKRW`, `shippingFeeKRW`, `remainingKRW` in order, `amountKRW` in `payment/pkg/domain/payment.go`. CLAUDE.md asks for `*_won` in Go identifiers too.
3. notification receives the `product.*` price as a float and converts it with `money.FromProductPrice` (§4.2).
4. `PATCH /api/v1/support/me/product-questions/{id}` replaces the whole question, so omitting `fit` clears it.

No service uses optimistic locking yet (§3.6), so D3 has no row.

---

## 3. v2 conventions across the services

### 3.1 JSON casing (D1)

Distinct JSON keys declared in each module, test files excluded:

| Module | Keys | camelCase | Where the camelCase comes from |
|---|---|---|---|
| auth | 40 | 0 | |
| order | 97 | 1 | `order/cmd/backfill-sku-id`, decoding product's `skuId` |
| cart | 16 | 0 | |
| payment | 65 | 20 | NANO PG wire types |
| notification | 16 | 0 | |
| support | 62 | 0 | |
| profile | 18 | 0 | |
| shared | 102 | 4 | `productclient`, decoding product v1 |
| product | 163 | 39 | Catalog types: `products.go`, size chart, dimensions, taxonomy, wishlist |

Product v1 is not uniformly camelCase, as D1 describes it. Promotions, reports and the reservation body are snake_case, and two types mix both styles: `StockItem` has `skuId` beside `updated_at` (`product/pkg/domain/inventory.go:8`), and the reservation request has `order_id` beside `skuId` (`product/pkg/handler/inventory_handler.go:122`). The promotion condition attributes stored in `promotions.conditions` mix them too: `line.brandCode`, `line.skuId` and `line.productId` sit next to `line.unit_price_won` and `line.on_sale` (`product/pkg/domain/promotion_conditions.go:55`).

**snake_case is the only choice for v2 that converges.** The condition attribute names are D1's stored-data side and need their own decision: accept both spellings, or migrate the JSONB.

### 3.2 Error body: six live shapes, two targets

| Shape | Where |
|---|---|
| `{"error": "<message>"}` | auth (most routes), notification, support inbox and Telegram routes |
| `{"error": "<message>", "code": "<string>"}` | auth login refusal (`auth/pkg/handler/handler.go:123`); support web chat and product questions (`respondCode`, `support/pkg/handler/web.go:228`) |
| `{"error": "<message>", "code": <HTTP status>}` | product, order, cart, payment, profile |
| The same, plus `"reason": "<string>"` (and `sub_reason`, `method`, `unavailable_items`) | order: promotion refusals, a disabled payment method, unavailable lines (`order/pkg/handler/http.go:519`) |
| `{"error": "<code>"}` | auth API-key exchange (`auth/pkg/handler/api_key.go:53`) |
| `{"error": "<code>", "message": "<message>"}` | auth API-key create (`auth/pkg/handler/api_key.go:102`) |

Two documents set a target, and they disagree:

- **v1.1 Slice 1** (every service): flat, `{"error": "<message>", "code": "auth.session.expired"}`.
- **v2 §6.1** (product v2 only): nested, `{"error": {"code": "unknown_master", "message": "…", "field": "color_code"}}`.

The flat shape is already live in auth and support with snake_case codes (`account_type_not_allowed`, `customer_required`), and order's `reason` values are codes of the same kind. Each target has a hazard. `code` is an integer in five services today, so v1.1's target changes the type of an existing key. v2's nested object changes the type of `error`, so product v1 and v2 would disagree for as long as both are served (D2).

v2 §1 says product errors carry no code. They carry `code` set to the HTTP status (`product/pkg/handler/handler.go:57`), which is the same integer-versus-string hazard.

The status code also varies. A write that names something that doesn't exist answers `404` in product v1 (unknown master), `400` in order (unknown SKU, `order/pkg/handler/http.go:565`), and `422` in order (unavailable lines) and in v2.

**Recommendation.** Use one envelope in every service: v1.1's flat shape plus v2's optional `field`, with snake_case codes.

```json
{ "error": "color RDX does not exist", "code": "unknown_master", "field": "color_code" }
```

Build it once as the shared helper that v1.1 Slice 1 step 2 already calls for (stdlib and Gin), and have v2 use it instead of the nested object. Order's `reason` becomes `code`; keep `reason` for one release. Before `code` changes from a number to a string, check that `dupli1-web` and `dupli1-manage-web` read the HTTP status rather than the body's `code`. Answer `422` whenever a body names something that doesn't exist, and keep `404` for the resource in the path.

### 3.3 Pagination

| List | Today |
|---|---|
| `GET /api/v1/orders`, all orders (`order.read.all`) | Every order, no `LIMIT` (`order/pkg/infra/pg/repository.go:795`); `total` is `len(orders)` |
| `GET /api/v1/auth/users` | Every user (`auth/pkg/infra/postgres/user_repository.go:86`) |
| `GET /api/v1/support/inquiries` | The newest 100 that match. The handler never reads `limit`, and there is no cursor or offset, so older inquiries can't be listed (`support/pkg/infra/postgres/inquiry_repository.go:58`) |
| Support product-question queue | `limit`, no cursor |
| `GET /api/v1/products` | `limit` / `offset`, max 100, with `total` |
| v2 | Cursor, no offsets |

**Recommendation.** Put v2's cursor helper in `shared/`: an opaque cursor over the sort key and `id`, `limit` up to 100, and `next_cursor`. Then use it for the three admin lists above.

v2 itself needs two more answers:
- v1 search sorts by counters that keep moving (`views`, `sold`, `wishlist`), and a keyset cursor over a moving key can skip or repeat a card. Say whether that drift is accepted.
- Category pages show item counts today. Say whether style lists return a `total`.

### 3.4 Request decoding and partial updates

No service refuses unknown or read-only fields: none uses `DisallowUnknownFields` or Gin's `EnableDecoderDisallowUnknownFields`. For example, `POST /api/v1/orders` with `"status": "paid", "total_won": 1, "bogus": true` answers `201` and ignores all three fields. That is v2 §1's first row, in order.

profile's `PATCH /api/v1/profile/me/addresses/{id}` uses `""` to mean "keep" (`profile/pkg/service/service.go:204`), so `label`, `address_line2` and `pccc` can never be cleared once set. This is the `MergeUpdate` defect. A patch that sends all three as `""` leaves them unchanged, as a test confirmed. PCCC is a customs identity number, and a customer who wants it gone has to delete the whole address.

Other problems on the same profile routes:
- Create and patch share one `AddressInput` struct.
- A patch with `"is_default": false` is silently ignored.
- `PATCH …/profile` has pointer fields but can't clear `display_name` or `phone`: `""` fails validation and `null` means keep.

notification's subscription `PATCH` uses pointer fields, which is the right shape for flags.

**Recommendation.** Ship v2's strict decoder (`400 unknown_field`, naming the key) and merge-patch helper in `shared/`. Fix profile's address `PATCH` with them first: it is small and customers use it.

### 3.5 Entity, view and wire types

Three services already split these the way v2 Step 1 wants:
- auth: `domain.User` has no JSON tags; `userResponse` lives in the handler.
- support: views are built in the handler.
- cart: `StoredItem` is what's stored; `CartItem` is enriched for reads.

The others don't. order puts JSON tags on `domain.Order` and gives it seven computed fields, "populated by ApplyRefundPolicy, not stored" (`order/pkg/domain/order.go:141`). That is product's counters-among-entity-fields problem. payment hides stored fields with `json:"-"`, and notification and profile tag their domain types.

Typed enums exist for `OrderStatus`, `CheckoutSessionStatus` and `PaymentStatus`. Everything else is a plain string with constants:
- payment: method and provider
- auth: `account_type`
- notification: subscription status
- support: inquiry status, channel, and question type and status

**Recommendation.** No rewrite. Apply v2 Step 1's rules to new code, and whenever a type is reshaped. Move order's computed policy fields into a view type now: they look stored and are not. When `money.Won` lands in `shared/pkg/money` (v2 §4), rename the `*KRW` identifiers in the same pass.

### 3.6 Optimistic locking

No service uses `ETag`, `If-Match` or a version column. Under D3, product would be first, which is the right place to start: a style edit that fans out to every color is the write where a lost update costs most. Keep the helper (an `ETag` from `updated_at`, `412` on mismatch) in `shared/` so the next service that needs it doesn't copy it.

### 3.7 Layering

Two services break the hexagonal rule:
- payment's service and handler import `payment/pkg/infra/checkout` for the NANO signature checks (`payment/pkg/service/service.go:13`, `payment/pkg/handler/http.go:17`).
- notification's handler and service import `pgx` and branch on `pgx.ErrNoRows` (`notification/pkg/handler/http.go:23`, `notification/pkg/service/telegram_subscriptions.go:11`). The handler also imports `infra/telegram`.

These need the same fix v2 Step 1 gives JSON tags. notification needs a not-found sentinel in `ports`. payment's NANO checks are pure functions and belong behind a port or in `domain`.

---

## 4. Contracts the v2 plan does not cover

### 4.1 Reservations drop `sku_id` (live bug)

order sends each reservation line as `{"sku_id", "sku", "quantity"}` (`order/pkg/ports/stock.go:19`). product decodes `skuId` (`product/pkg/handler/inventory_handler.go:126`). `encoding/json` matches keys case-insensitively but not across `_`, so product never sees the id and reserves by the human `sku`. Decoding the exact body order sends confirmed this. It works today only because order always sends `sku` as well, and a variant's `sku` never changes.

v2 keeps inventory on v1 and makes the ULID the identity, so fix this before v2:
- Accept `sku_id` on the reservation body, and keep `skuId` for one release.
- Add a contract test that decodes order's real request in product.

### 4.2 `product.*` events

`events.Product` (`shared/pkg/events/events.go:189`) carries three fields that conflict with v2:
- `product_id` is the parent.
- `brand` is a copy of the name, the stale-name problem v2 §1 lists for the row.
- `price` is a `float64`, the only money key on the bus not named `*_won`. notification formats it through `money.FromProductPrice` (`notification/pkg/service/dispatcher.go:491`), which v2 §4 expects to retire.

v2 needs to say three things about these events:
- What `product_id` names after the flatten: the style or the sellable product.
- Whether a style edit that fans out to N products publishes one event or N.
- When `price` becomes `price_won`.

The flat plan's §6 only says the payloads "gain `styleCode`".

### 4.3 A third `productclient` consumer

support's gateway reader (`support/pkg/infra/gateway/gateway.go:15`) uses `shared/pkg/productclient` to resolve web-chat reference cards and product questions. v2 §7 and CLAUDE.md list only cart and order. support needs to move to `/api/v2/products` with them.

### 4.4 Parent ids outside the catalog

v2 §6.3 keeps wishlist, recommendations, promotions and visits on v1 because "a v2 product id is a v1 `skuId`". That holds for inventory. The surfaces below take the **parent** id instead, and the v2 Style resource doesn't expose one:

| Where | Problem |
|---|---|
| Wishlist, `/api/v1/products/{id}/wishlist`; the list returns parents | A v2 client has no id to send |
| Recommendations, `/api/v1/products/{id}/recommendations` | Same |
| PDP view count, recorded as a side effect of `GET /api/v1/products/{id}` (`product/pkg/handler/handler.go:320`) | v2 doesn't say whether its PDP read counts a view. If it does, server-side readers inflate the count; support already avoids that route for this reason |
| Promotion predicates on `line.productId`, stored in `promotions.conditions` | After the flatten, the name reads as the sellable id, but the stored values are parent ids |
| support product questions: the `product_id` column and the `?product_id=` filter | Parent ids stored in another database, resolvable only while v1's legacy-id lookup lives (D2) |
| `product_id` on cart lines | Means the parent. The flat plan's Phase 6 renames the `sku_id` columns in cart and order to `product_id`, which would give the word two meanings across services |

**Decide before Step 3:**
- Does a Style keep a stable id, for example the legacy parent ULID as `style.id`, or does everything key on `{brand_code}/{style_code}`?
- Do wishlist, recommendations and views get style-keyed routes in v2, or do the v1 routes accept style codes?
- Make v2 reads free of side effects, and record the PDP view with an explicit beacon, like `/visits`.
- Add a style attribute for promotion conditions, and migrate stored `line.productId` predicates to it.
- Outside product, `product_id` should mean only the sellable product. Rename parent references (the cart response, support) to `style_*` before any column becomes `product_id`.

---

## 5. Product docs that disagree with v2

| Doc | Says | v2 |
|---|---|---|
| [product-structure-final-review.md](product-structure-final-review.md) §4, status "authoritative" | A target `Product` struct on v1: camelCase, `float64` prices, string status and time, a copied `brand` name, counters on the entity | Replaced by v2 §6.2; v2 inherits only the §5 locks. Its status line now says so |
| The same doc, §5 listing lock | PDP picker from `siblings`, mirroring `variants` for one release | No `siblings` on v1; the v2 PDP reads `products` |
| [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md) | Keep `Product` / `Variant`; "`Product` is the stable cross-service noun" for the parent | Parent → `Style`, sellable → `Product`. The category-agnostic conclusion stands. Its status line now says so |
| [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) §6 | order's "`httpproduct` client"; support not listed; coupons "unaffected" | The client is `shared/pkg/productclient`; support and promotion conditions are affected (§4) |
| [product-api-v2-plan.md](product-api-v2-plan.md) §8, "A SKU's own size" | `dimensions_mm` (bags) "still stored once per style" | [product-sku-dimensions.md](product-sku-dimensions.md) and v2's own §6.3 keep dimensions per product |
| v2 §6.2, Style `name` | Names are joined from masters and never stored on the row | A style's name lives on the `styles` master and on today's `products.name`, which becomes `product_styles`. The flat plan keeps both tables, so v2 should name the canonical one, or the stale-name problem returns for styles |
| [api.md](api.md) § Common error shape | support sends only `{"error"}` | Web chat and product questions also send a string `code`. Now corrected |
| CLAUDE.md, the `productclient` row | Used by cart and order | Also support. Now corrected |
| [TODO.md](TODO.md), v1.0 closeout D/E | Frontends finish the v1 parent/variant migration: canonical paths, parent pricing, `skuId` | v2 Step 3 moves the same screens again. Decide whether the catalog screens wait for v2, leaving v1.0 with only the cart and checkout `skuId` work |

---

## 6. Recommended order

1. Fix §4.1 and add a contract test. It is small and independent of v2.
2. Agree one error envelope for v1.1 Slice 1 and v2 (§3.2), and add the shared helper.
3. Add the questions from §4 (style identity, events, view recording) and §3.3 (cursor drift, `total`) to v2 §3, and settle them before Step 3.
4. Build v2's cursor, strict-decode, merge-patch and `ETag` helpers in `shared/`. Their first users outside product: profile's address `PATCH`, the support inbox, and the order and auth admin lists.
5. As the code is touched: order's computed fields into a view type, the `*KRW` identifiers, and the payment and notification layering.

---

## 7. Also seen

- 23 Go files in auth, order, cart, payment and product are not `gofmt`-clean. The drift is whitespace and alignment only, some of it left by renames. CI runs only `go test`.
- Two Postgres drivers are in use: `pgx/v4` (cart, notification, order, payment, product) and `lib/pq` (auth, profile, support). This adds to the two id-sequence mechanisms already in TODO.
- auth names a user's id `user_id` in its responses; every other resource uses `id`.
