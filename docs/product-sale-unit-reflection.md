# Reflection: Product as sale unit vs SKU

**Status:** Reflection / decision plan (no code change yet).  
**Related:** [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md), [product-variants-plan.md](product-variants-plan.md), [product-sku-system.md](product-sku-system.md), [product-price-on-parent.md](product-price-on-parent.md).

## Premise

Today:

| Layer | Go / storage | Customer meaning | Shared `styleCode`? |
|-------|--------------|------------------|---------------------|
| Style master | `styles` under brand | Design identity (CAS001) | *is* the style code |
| Parent **Product** | `products` | Catalog card / PDP shell | Yes — one per parent |
| **Variant** (SKU) | `product_variants` | Color / size option | Always inherited from parent |

Observation (correct): every variant under a product shares the same `brandCode` + `styleCode`. Price already lives on the parent ([product-price-on-parent.md](product-price-on-parent.md)).

Desire: use **product** as the **unit for sale**, instead of SKU.

## Short answer

**Keep SKU (`Variant` / `skuId`) as the unit for sale** if customers can choose color (or size) and stock differs per option.

Sharing a style code does not make options interchangeable for checkout. Cart, order, inventory, and warehouse need a line identity that includes color/size. That identity is the SKU.

You *can* make “product” the sale unit only if you also change the business rule to one of the options below — not by renaming alone.

## Why style-sharing ≠ one sale unit

```text
Style CAS001 (master)
    └── Product BOT-… / ULID   ← PDP, search card, shared price
            ├── Variant black  / M   ← stock 3, cart line A
            └── Variant green  / M   ← stock 0, cart line B
```

| Concern | Parent Product | Variant SKU |
|---------|----------------|-------------|
| Same style code | Yes | Yes (copied into human `sku`) |
| Same price (today) | Yes | Inherited |
| Same images | Often no (per color) | Per variant |
| Same stock | No | Per `skuId` |
| Cart / order / reserve | Must not key here if multi-option | Keys on `skuId` |

If checkout keyed on `productId` only:

- Cannot reserve “black” vs “green”
- Cannot ship the right color
- Cart cannot hold two colors of the same style as separate lines
- Sold-out green would incorrectly block (or incorrectly allow) black

So: **style-level sameness is catalog identity; sale unit is stock-keeping identity.**

## What “product as sale unit” could mean

### Option A — Flatten: each color/size *is* a Product (sale unit = Product)

```text
Product (black Cassette)  ← sold, stocked
Product (green Cassette)  ← sold, stocked
Style master CAS001       ← groups them for search/PDP (optional)
```

| Pros | Cons |
|------|------|
| “Product” matches commerce language | Search shows duplicates again (the problem variants solved) unless Style regroups results |
| Cart keys on `productId` | Large migration: cart/order/inventory leave `skuId` |
| | Parent+variant APIs, docs, frontends rewritten |
| | Human SKU format still needs color/size segments somewhere |

**Verdict:** Only if you deliberately undo parent+variant and reintroduce a *Style* grouping layer for browse/PDP. High cost; same physics under new names.

### Option B — Style-only commerce: one stock pool per Product (no sellable color split)

```text
Product = sale unit = stock unit
Colors are display-only (or single default variant forever)
```

| Pros | Cons |
|------|------|
| Matches “buy the bag, not the SKU” | Unfit for multi-color bag inventory |
| Simplifies cart to `productId` | Warehouse cannot pick by color |
| | Conflicts with current variant images + inventory merge |

**Verdict:** Reject for a fashion bag marketplace with per-color stock.

### Option C — Keep current model; clarify vocabulary (recommended)

```text
Style master     → design identity (shared styleCode)
Product (parent) → browse / PDP / shared price / wishlist / views
Variant (SKU)    → unit for sale (cart, order, stock, ship)
```

| Pros | Cons |
|------|------|
| Matches implemented system | “Product” is not the cart line id |
| Price already style-level | Naming feels odd if ops say “product” for sellable |
| Search stays one card per style | — |

Optional clarity later (docs / API comments only):

- Call parent **style product** or **PDP product**
- Call variant **sellable SKU** (already true)
- Do **not** rename `Product` → sale unit without Option A/B

**Verdict: Accept.**

### Option D — Rename Variant → Product (sale unit named Product)

Parent becomes `Style` / `StyleProduct`; sellable row becomes `Product`.

| Pros | Cons |
|------|------|
| Sale unit named “product” | Breaks `/api/v1/products`, `product.*` permissions, every client |
| | Conflicts with “product = bag style” mental model already in docs |
| | Still need a parent grouping type |

**Verdict:** Reject as a rename project; same as Option A with worse churn.

## Relation to “Product is kind of a bag”

Bag-ness belongs on `category` + bag taxonomy, not on the sale-unit question. Whether the sale unit is parent or SKU is independent of wallets/clothing later — every category with options still needs an SKU-level stock key.

See [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md).

## Decision

| Proposal | Decision |
|----------|----------|
| Use parent `Product` as cart/order/inventory key because variants share `styleCode` | **Reject** — style sharing ≠ interchangeable stock |
| Drop color/size SKUs and sell one pool per style (Option B) | **Reject** unless business drops per-color inventory |
| Flatten so each option is a Product (Option A) | **Defer / avoid** — reopens duplicate-search problem; huge migration |
| Keep Variant/`skuId` as unit for sale; Product as PDP/style shell (Option C) | **Accept (current)** |

## If product-level selling is still desired later

Require an explicit product decision first:

1. Do black and green of the same style have **separate stock**? (If yes → SKU remains sale unit.)
2. Does search show **one card or many** per style?
3. Is price always identical across colors? (Already yes on parent.)

Only if (1) is “no separate stock” does Option B become viable. Only if (2) is “many cards” does Option A become viable.

## Checklist (no work until business picks A/B)

- [x] Document why shared `styleCode` does not move the sale unit to parent Product
- [ ] Business confirms per-color (and per-size) stock is required
- [ ] If confirmed: leave cart/order on `skuId`; treat Product as catalog/PDP only
- [ ] If not: write a dedicated migration plan (Option A or B) before any rename
