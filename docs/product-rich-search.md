# Product rich search + wishlist

**Status:** Implemented.  
**Related:** [api.md](api.md), [product-sold-count.md](product-sold-count.md), [product-guest-views-plan.md](product-guest-views-plan.md).

## `GET /api/v1/products` query params

| Param | Values | Default | Notes |
|-------|--------|---------|-------|
| `q` | string | — | Case-insensitive match on name, brand, or description |
| `sort` | `newest`, `views`, `sold`, `wishlist`, `price`, `name` | `newest` | Aliases: `popular`→`views` |
| `order` | `asc`, `desc` | `desc` (`asc` for `name`) | Invalid → `400` |
| `period` | `day`, `week`, `month` | — | Created within that window (`7d`/`past_week`→`week`) |
| Existing filters | `category`, `subcategory` (`subCategory`), `style`, `target`, `brand`, `color`, `size`, `material`, `tags`, `status` | — | Bag taxonomy: [product-master-catalog.md](product-master-catalog.md) |
| Pagination | `limit`, `offset` | `50` / `0` | Max limit `100` |

Response echoes effective `sort`, `order`, and `period` (when set) alongside `total` / `limit` / `offset` / `results`.

Sort keys use denormalized parent counters (`view_count`, `sold_count`, `wishlist_count`) or `created_at` / `name` / min active variant price.

`period=week` keeps only parents with `created_at` in the last 7 days (from request time).

## Wishlist

| Route | Purpose |
|-------|---------|
| `PUT` / `POST /api/v1/products/{id}/wishlist` | Add (idempotent); bumps `wishlistCount` once per owner × product |
| `DELETE /api/v1/products/{id}/wishlist` | Remove; decrements count |
| `GET /api/v1/products/wishlist` | List current owner's wishlisted public parents |

**Owner key:** JWT `sub` as `u:<id>` when authenticated; otherwise guest cookie `g:<dupli1_guest>`.

Schema: `product_wishlists (owner_key, product_id)` + `products.wishlist_count`.

## Checklist

- [x] `sort` / `order` / `q` / `period` on product search
- [x] Sort by views, newest, sold, wishlist, price, name
- [x] Past-week (and day/month) created_at window filter
- [x] Wishlist add/remove/list + public `wishlistCount`
- [x] OpenAPI + docs

