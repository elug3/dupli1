# Plan: Product View Counter + Simple Recommendations

**Status:** Planning (not implemented).  
**Depends on:** [product-guest-views-plan.md](product-guest-views-plan.md) (guest cookie + unique `viewCount`).  
**Related:** [current-state.md](current-state.md), [api.md](api.md), [TODO.md](TODO.md).

## Goals

1. Ship the **unique PDP view counter** already designed in the guest-views plan (`dupli1_guest` + `product_views` + denormalized `products.view_count`).
2. Add a **simple, deterministic recommendation API** for the storefront PDP (“You may also like”) that needs no ML stack, no Redis, and no new service.
3. Reuse view data so recommendations improve as traffic grows, without blocking v1 on co-view history.

Non-goals:

- Collaborative filtering / embeddings / personalization models
- Real-time trending windows, A/B ranking, or recs email campaigns
- Homepage “for you” feed (can reuse the same scorer later)
- Counting recommendations as views (recs are read-only)

## Why these two together

| Capability | Alone | Together |
|------------|--------|----------|
| View counter | Social proof on PDP (`viewCount`) | Popularity signal for ranking |
| Content-based recs | Works from catalog attrs only | Views break ties + cold-start fallback |
| Co-view (“also viewed”) | Needs `product_views` history | Natural phase 2 once views exist |

Views are the foundation; recommendations are the first product feature that consumes them.

---

## Part A — View counter (summary)

Full design lives in [product-guest-views-plan.md](product-guest-views-plan.md). This plan **does not change** that contract; it treats phase 1 of that doc as a hard prerequisite for recommendation phases that use popularity or co-view.

### Must ship first

| Item | Detail |
|------|--------|
| Cookie | `dupli1_guest` minted on public PDP |
| Table | `product_views (guest_id, product_id)` unique |
| Counter | `products.view_count` incremented only on first unique view |
| JSON | Public `viewCount` on `GET /api/v1/products/{id}` |
| Failure | View write never fails the PDP |

### Recommendation-relevant side effects

Once views land:

- Every PDP has an O(1) popularity score (`view_count`).
- `product_views` becomes a sparse guest×product matrix for optional co-view later.
- Same parent-product grain as search/PDP (not variant SKU).

---

## Part B — Simple recommendation algorithm

### Product surface

```text
GET /api/v1/products/{id}/recommendations?limit=8
```

| Property | Choice |
|----------|--------|
| Auth | Public (same as PDP); optional Bearer ignored for ranking v1 |
| Scope | Parent product ids only (active parents) |
| Default `limit` | `8` (clamp 1–24) |
| Response shape | Same parent card fields as search list (no full `variants[]` unless cheap) |
| Exclude | Seed product itself; drafts/archived |
| Empty | `{"items":[]}` — never 404 when seed exists |

If seed product is missing / not public → `404` (same as PDP).

### Algorithm v1 — weighted content similarity + popularity

No training. Score candidates in SQL (or memory store for tests) and return top-N.

**Candidate pool (narrow first):**

1. Same `category` as seed (required for bags marketplace — avoids shoes↔bags noise).
2. Active parents only.
3. Cap pool (e.g. 200) before scoring if catalog grows; for current size, score all same-category actives.

**Feature scores (0/1 or small integers):**

| Signal | Weight | Rule |
|--------|--------|------|
| Same `brand` / `brand_code` | +5 | Strong style affinity |
| Shared `material` | +3 | Material match |
| Tag overlap | +2 × min(shared tags, 3) | Cap so tag spam cannot dominate |
| Price band | +2 | Seed `price_from` within ±30% of candidate `price_from` (use summary price) |
| Popularity | +1 × log10(`view_count` + 1) | Soft boost; cold products still rank on attrs |

**Final score** = sum of weights. Sort by score DESC, then `view_count` DESC, then `id` ASC (stable).

**Why this shape:**

- Explainable and testable with fixed fixtures.
- Works with **zero** views (attrs alone).
- Improves automatically once `view_count` exists.
- Fits existing product columns (`brand`, `material`, `category`, `tags`, price summaries) — no schema for v1 beyond views.

### Algorithm v1.5 — co-view boost (optional, after views have data)

When `product_views` has enough rows:

```text
For guests who viewed seed product P:
  count other products Q they also viewed
Boost Q by +4 × min(co_view_count, 5)  (or join in a second query)
```

Keep content similarity as the base; co-view is a boost, not a replacement (cold products / new catalog still need attrs).

Skip co-view in v1 if traffic is thin — empty co-view should not change ranking.

### Explicitly rejected for v1

| Approach | Why not yet |
|----------|-------------|
| Random same-category | Weak UX; no learning from catalog structure |
| “Most viewed globally” only | Ignores seed; homepage widget, not PDP related |
| Redis sorted sets / ML service | Extra infra; product has Postgres today |
| User-id personalization | Guests + JWT merge is later; cookie history is enough for co-view |

---

## API / JSON

```http
GET /api/v1/products/BOT-001/recommendations?limit=8
```

```json
{
  "seedId": "BOT-001",
  "items": [
    {
      "id": "BOT-014",
      "name": "…",
      "brand": "…",
      "category": "bags",
      "priceFrom": 189.0,
      "defaultImageUrl": "…",
      "viewCount": 420,
      "availableColors": ["Black", "Green"]
    }
  ]
}
```

| Field | Notes |
|-------|--------|
| `seedId` | Echo of path id |
| `items` | Ordered recommendations |
| `viewCount` | Include once Part A ships (helpful for admin debugging; storefront may hide) |
| Score | **Not** exposed publicly |

List/search may later add `?sort=popular` using `view_count` — out of scope here; same column.

### Gateway

Add nginx route for `/api/v1/products/{id}/recommendations` → `dupli1-product` (same upstream as other product paths; ensure more-specific location or that `{id}` does not swallow `recommendations` if routed as a static segment — prefer registering the path in the Go mux before generic `{id}` handlers, matching existing route style).

---

## Data flow

```mermaid
sequenceDiagram
    participant Browser
    participant Gateway as dupli1-proxy
    participant Product as dupli1-product
    participant DB as postgres-product

    Note over Browser,DB: Part A — PDP view (existing guest-views plan)
    Browser->>Gateway: GET /api/v1/products/{id}
    Gateway->>Product: proxy + cookie
    Product->>DB: unique view upsert + view_count++
    Product-->>Browser: PDP + viewCount

    Note over Browser,DB: Part B — recommendations
    Browser->>Gateway: GET /api/v1/products/{id}/recommendations?limit=8
    Gateway->>Product: proxy
    Product->>DB: load seed; score same-category actives
    Product-->>Browser: 200 + items[]
```

Recommendations do **not** mint cookies or record views.

---

## Service layout (implementation sketch)

Follow existing hexagonal layout in `product/`:

| Layer | Responsibility |
|-------|----------------|
| `handler` | `PublicGetRecommendations`; parse `limit`; map errors |
| `service` | `Recommend(seedID, limit)`; load seed; call store; map to list DTOs |
| `ports` | `ProductRecommender` or method on `ProductStore`: `RecommendSimilar(seed, limit)` |
| `infra/pg` | SQL scoring query (weights in SQL or Go over a small candidate set) |
| `infra/memory` | Same scoring rules for unit tests |
| `domain` | Optional: no new types required; reuse `Product` list shape |

Config (optional kill-switches):

- `PRODUCT_RECOMMENDATIONS_ENABLED` (default true)
- `PRODUCT_RECOMMENDATIONS_DEFAULT_LIMIT` / max clamp

Reuse Part A stores for views; do not couple recommendation reads to the cookie path.

### Suggested SQL sketch (v1)

```sql
-- Pseudocode: weights mirror the table above; price_from via existing variant aggregate or denormalized summary.
SELECT p.id, …,
  (CASE WHEN p.brand_code = $seed_brand THEN 5 ELSE 0 END)
  + (CASE WHEN p.material = $seed_material THEN 3 ELSE 0 END)
  + (2 * LEAST(cardinality(ARRAY(SELECT UNNEST(p.tags) INTERSECT SELECT UNNEST($seed_tags::text[]))), 3))
  + (CASE WHEN p.price_from BETWEEN $lo AND $hi THEN 2 ELSE 0 END)
  + LN(10, p.view_count + 1) AS score
FROM products p
WHERE p.status = 'active'
  AND p.category = $seed_category
  AND p.id <> $seed_id
ORDER BY score DESC, p.view_count DESC, p.id ASC
LIMIT $limit;
```

Exact `price_from` / tag intersection syntax follows whatever the current `products` + variant summary columns already expose in `product_store.go` (compute in Go over candidates if SQL array intersect is awkward).

---

## Phased delivery

### Phase 0 — Views prerequisite

Implement [product-guest-views-plan.md](product-guest-views-plan.md) phase 1:

1. Schema + cookie + unique upsert
2. `viewCount` on PDP
3. Tests for uniqueness and failure isolation

### Phase 1 — Content + popularity recommendations (this plan’s core)

1. Route `GET /api/v1/products/{id}/recommendations`
2. Scorer: category + brand + material + tags + price band + `log(view_count)`
3. Memory + PG tests with fixed catalog fixtures (known ranking order)
4. Docs: `api.md`, `endpoints.md`, `current-state.md`
5. Gateway path coverage if needed

### Phase 2 — Co-view boost (optional)

1. Query co-viewed parents from `product_views`
2. Add capped boost to score
3. Feature flag if co-view volume is low

### Phase 3 — Storefront / polish (separate)

- PDP “You may also like” rail in `dupli1-web`
- Optional homepage “Popular” using `ORDER BY view_count`
- Admin: exclude staff views (guest-views phase 3)

---

## Frontend notes (`dupli1-web`)

- Call recommendations after PDP load (or in parallel with PDP); do not block hero render.
- Same-origin `/api/...` preferred (same cookie story as views, though recs do not need the cookie).
- Cards: reuse catalog card component; show image, name, price; `viewCount` optional.
- If `items` empty, hide the section (no empty rail).

---

## Testing plan

| Case | Expect |
|------|--------|
| Seed missing | 404 |
| Alone in category | `items: []` |
| Same brand beats other brand (equal other attrs) | Higher rank |
| Shared tags increase score | Deterministic order in fixture |
| Higher `view_count` breaks ties | Stable sort |
| `limit=2` | At most 2 items; seed excluded |
| Draft candidate | Never returned |
| Views disabled / zero counts | Still returns content-based order |

---

## Open decisions (defaults chosen)

| Question | Default |
|----------|---------|
| Recs unit | Parent product (not SKU) |
| Primary signal | Content similarity |
| Popularity | Soft `log10(view_count+1)` |
| Co-view | Phase 2 only |
| Public score | Hidden |
| Same category required | Yes |
| Personalization by login | No (guest co-view later is anonymous) |

## Acceptance criteria

### Views (from guest-views plan)

- [ ] Unique `viewCount` on PDP; reload does not double-count

### Recommendations phase 1

- [ ] `GET /api/v1/products/{id}/recommendations` returns ordered active parents
- [ ] Seed excluded; drafts excluded; same category only
- [ ] Ranking matches documented weights on fixtures
- [ ] Works with all-zero `view_count`
- [ ] `limit` clamped; invalid seed → 404
- [ ] No new infra (Postgres + existing product module only)

## Doc / TODO updates when implementing

- Mark phases done in this file and [product-guest-views-plan.md](product-guest-views-plan.md)
- Update [TODO.md](TODO.md), [current-state.md](current-state.md), [api.md](api.md), [endpoints.md](endpoints.md)
