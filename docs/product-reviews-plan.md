# Product reviews (리뷰)

**Status:** Backend implemented 2026-11-08 in `product` (Phases 1, 2 and the console API of 4). The storefront and manage-web screens are not built. Closes the "Reviews feature" item in [TODO.md](TODO.md#storefront) once they are.
**Design:** [Figma, page "Reviews (PDP)"](https://www.figma.com/design/tqtyEpto8TYyJqqN2HjWth?node-id=12-433): the summary row in the detail links, the full-width section under the gallery, mobile, and the empty state.
**Related:** [order-service.md](order-service.md) (statuses), [support-product-questions.md](support-product-questions.md) (the private Q&A this sits beside), [product-sold-count.md](product-sold-count.md) (counters on the parent), [product-images-browser-access.md](product-images-browser-access.md) (where photos are served from), [permissions.md](permissions.md).

A shopper who bought a product rates it 1–5, writes about it, and may add photos. Reviews are **public** on the product page, unlike 상품 문의, which only the asker and staff see. Every review comes from a finished order line, so every review on the page carries 구매 확인.

## Decisions

Decisions 7 and 8 were confirmed on 2026-11-08.

1. **Owner: the product service.** Reviews are read on every product page view, next to `soldCount` and `viewCount`, and the summary belongs on the parent product. Product already stores and serves catalog photos (SeaweedFS `product-images`, the `imaging` pipeline), and `/api/v1/products/*` is already routed. The alternative, support, has the staff console and `user.deleted` handling but nothing on the public read path. Product will need to subscribe to `user.deleted` (it already subscribes to `user.registered`).
2. **Who can write: the buyer of a `fulfilled` order (구매확정), once per order line.** `delivered` is not enough. A delivered order can still become `disputed`, and the Korean convention is review-after-구매확정. On a delivered order the storefront offers 구매확정하고 리뷰 쓰기, which calls `POST /orders/{id}/receipt/confirm` and then opens the form. The 14-day auto-fulfil sweep makes every undisputed order reviewable.
3. **An order line is `(order_id, sku_id)`.** Order items have no line id (`order/pkg/domain/order.go` `OrderItem`). A line with quantity 2 still gets one review. An order *can* list the same SKU on two lines (`SetItems` and `NewOrder` do not merge them); it still gets one review per SKU, which is what the unique key says.
4. **Eligibility is checked at write time, against order, with the shopper's own token.** On create, product calls `GET /api/v1/orders/{order_id}` through the gateway, forwarding the caller's Bearer. Order's owner check (ABAC) already allows it, so product gets no new service account and no `order.read.all`. Product checks that the order's `customer_id` is the caller (a manager with `order.read.all` can read anyone's order, so order's check alone is not enough), that `status == fulfilled`, and that `skuId` is a line of that order. A line written before orders carried `sku_id` is matched on its SKU string. If order does not answer, the result is `503 reference_unavailable` and nothing is written. Building a ledger from `order.status_updated` was considered and rejected: core NATS does not redeliver, so a missed event would lock a buyer out for good, and every order fulfilled before launch would need a backfill.
5. **Reviews belong to the parent product, across its variants.** Each review keeps a snapshot of the variant label (`블랙 / M`) from the catalog at write time, and the page shows it.
6. **Reviews are visible immediately, and staff can hide them.** There is no approval queue. Staff hide spam, abuse, personal data or off-topic posts, with a reason. A hidden review leaves the public list and the summary. Its author still sees it, marked 숨김 처리됨 with the reason.
7. **The author's name is masked, as 김\*\*, and the full `display_name` is never shown or stored** (decided 2026-11-08). At write time product reads `display_name` from profile through the gateway with the caller's token, keeps the first character and stores only the masked form in `author_display`. With no display name, or when profile does not answer, the author is 구매자. The masked form is fixed at write time; renaming the profile does not change old reviews.
8. **Account deletion deletes the account's reviews, ratings included, and their photos** (decided 2026-11-08). There are no anonymous ratings: nothing of a deleted account stays in the list or the summary. On `user.deleted`, product deletes the rows and the photo objects and recomputes each affected product's counters in the same transaction.
9. **Clothing asks for fit.** Only when the parent's category is `clothing`. `fit` is `small` / `true_to_size` / `large`, with optional `heightCm` (100–230) and `usualSize` (up to 10 characters). There is no weight, unlike 상품 문의, because reviews are public.

## Rules

- Rating is a whole number 1–5. Text is 10–1000 characters after trimming. Up to 10 photos.
- One live review per `(order_id, sku_id)`. A second create returns `409 review_exists` with the existing id.
- The author may edit text, rating, fit and photos at any time. An edited review shows `editedAt`. Product, variant and order cannot change.
- The author may delete a review. They can then write a new one for the same line.
- A hidden review cannot be edited (`409 review_hidden`), so it cannot be quietly republished. It can still be deleted.
- Service accounts and managers without a customer order get `403 customer_required` (managers may still buy and review as customers).
- At most 10 review writes (create, edit, delete) and 30 photo uploads a minute per account (`429`, `reason: rate_limited`), shared across replicas through `REDIS_URL` when it is set. The budget is per account, not per IP: every shopper reaches product through the storefront's server. A full review is up to 10 uploads plus one create, so uploads get their own budget. Writes are rare and tied to orders, so this only stops scripts.

## Photos

- `image/jpeg`, `image/png`, `image/webp`, up to 10 MB and 60 megapixels each. Each is re-encoded to JPEG through `imaging.ReviewJPEGs` (long edge 1600, plus a 400 thumbnail). Re-encoding drops EXIF, including GPS, which matters because these are photos taken in shoppers' homes. The EXIF orientation is applied to the pixels first, so a phone photo stays upright.
- Uploading is a separate step from creating the review, so a slow phone upload can retry one photo without losing the text. `POST …/reviews/photos` returns a `photoId`. The create or edit call lists `photoIds`. An upload not attached to a review within 24 hours is deleted by a sweep.
- Objects are `reviews/<photoId>.jpg` and `reviews/<photoId>.w400.jpg` in the `product-images` bucket. The key names neither the shopper nor the product. Only keys are stored; URLs are built from `S3_PUBLIC_ENDPOINT` on every read, so moving the image host needs no data change. Without S3 configured, uploads answer `503 reviews_unavailable`.
- Hiding a review does not delete its photos (it can be unhidden). Deleting the review, replacing its photos, or deleting the account does: rows first, then the objects. An object whose delete fails is logged and left unreferenced, never a broken review.

## API

All paths are under `/api/v1/products`, which the gateway already routes to product. Product's wire format is camelCase, unlike order and support.

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/{productId}/reviews` | public | `?sort=latest` (default) / `rating_desc` / `rating_asc`, `?photo=true`, `?rating=1..5`, `?limit=` (default 5, max 20), `?cursor=`. Returns `{ summary, reviews, nextCursor }`. `Cache-Control: public, max-age=60` |
| GET | `/{productId}/reviews/photos` | public | Every photo from visible reviews, newest first, for the photo strip and the full-screen viewer: `{ photos: [{ reviewId, id, url, thumbUrl }], nextCursor }` |
| POST | `/reviews/photos` | Bearer (customer) | Multipart, field `photo`. `201 { photoId, url, thumbUrl }` |
| POST | `/{productId}/reviews` | Bearer | `{ orderId, skuId, rating, body, fit?, photoIds? }` → `201` with the review |
| GET | `/reviews/me` | Bearer | The caller's reviews, newest first. `?orderId=` narrows to one order, so the order page can tell which lines are reviewed |
| PATCH | `/reviews/by-id/{reviewId}` | Bearer, author | `{ rating?, body?, fit?, photoIds? }`. `photoIds` replaces the set |
| DELETE | `/reviews/by-id/{reviewId}` | Bearer, author | `204` |
| GET | `/reviews` | `product.review.moderate` | Console list, newest first: `?status=visible` (default) / `hidden`, `?productId=`, `?rating=`, `?limit=` (default 50, ≤200), `?cursor=`. `{ reviews, nextCursor }` |
| POST | `/reviews/by-id/{reviewId}/hide` | `product.review.moderate` | `{ hidden, reason }`. `hidden` defaults to `true`; `reason` (≤200 characters) is required when hiding and shown to the author |

`/reviews…` sits beside `/promotions` and `/reports` as a static segment. A review id sits behind a literal `by-id` segment, like `by-sku` and `by-code`, because `/reviews/{reviewId}` would overlap `/{productId}/wishlist` in Go's mux. Unknown JSON fields are refused. Cursors are opaque strings; pass back `nextCursor` as `?cursor=`.

**Summary:** `{ count, average, byRating: { "5": n, … "1": n }, photoCount, fit?: { small, trueToSize, large } }`. `average` is rounded to one decimal place, and is `null` when `count` is 0. `fit` is present for clothing only.

**Review (public):** `id`, `productId`, `skuId`, `variantLabel`, `rating`, `body`, `fit`, `photos: [{ id, url, thumbUrl }]`, `author` (masked), `verifiedPurchase: true`, `createdAt`, `editedAt`.
**The author also sees:** `orderId`, `hidden`, `hiddenReason`.
**Staff also see:** `customerId`, `hiddenBy`, `hiddenAt`.

**On the product:** `GET /api/v1/products/{id}` and listing rows gain `reviewCount` and `ratingAverage`, so cards and sorting can use them later without a second request.

**Errors** use product's shape, `{ error, code }` with the HTTP status in `code`, plus a machine-readable `reason`:

| Reason | Status | When |
|---|---|---|
| `invalid_review` | 400 | Rating, text, fit or photo count out of range; `fit` on a bag; a photo id that is not the caller's or is already used; a bad sort or cursor |
| `invalid_photo` | 400 | Wrong type, too large, unreadable |
| `customer_required` | 403 | Service account |
| `not_reviewable` | 403 | The order is not the caller's, is not `fulfilled`, or has no such `skuId`. One code for all three, so the API does not reveal other people's orders |
| `review_not_found` | 404 | Also for someone else's review on PATCH and DELETE |
| `product_not_found` | 404 | Unknown product; on the public reads also a draft or archived one, as the PDP |
| `review_exists` | 409 | The line already has a review; carries `reviewId` |
| `review_hidden` | 409 | Editing a hidden review |
| `rate_limited` | 429 | |
| `reference_unavailable` | 503 | Order did not answer. Profile not answering only drops the name to 구매자 |
| `reviews_unavailable` | 503 | No `DUPLI1_GATEWAY_URL` (create), no S3 (upload), or a store failure |

## Storage

Inline schema in product, like `promotions`:

Inline schema in product (`infra/pg/review_store.go`), created at startup like the other stores:

- `product_reviews`: `id` (ULID, so id order is time order), `product_id` (→ `products`, `ON DELETE CASCADE`), `sku_id`, `order_id`, `customer_id`, `variant_label`, `author_display`, `rating`, `body`, `fit` (JSONB), `hidden`, `hidden_reason`, `hidden_by`, `hidden_at`, `created_at`, `edited_at`. Unique `(order_id, sku_id)`. Indexes `(product_id, hidden, id DESC)`, `(product_id, hidden, rating, id DESC)`, `(customer_id, id DESC)`, `(hidden, id DESC)`.
- `product_review_photos`: `id`, `review_id` (null until attached; cascades with the review), `customer_id`, `object_key`, `thumb_key`, `position`, `created_at`.
- On `products`: `review_count`, `review_rating_sum`, `review_r1`…`review_r5`, `review_photo_count`, `review_fit_small`, `review_fit_true`, `review_fit_large`. Every write locks the parent's `products` row and **recomputes** these from the visible rows in the same transaction. The lock serialises writers on one product, so each recount sees the other's committed row and the counters are exact; there is no increment to drift and no repair job.

Deleting a product cascades to its reviews and photo rows, but not to the photo objects, which are left unreferenced in the bucket.

## Notifications

None in Phase 1. Later: `product.review_created` with `{ reviewId, productId, rating }` (ids and the rating, no text, as with `SupportLive`), so notification can alert ops to 1–2 star reviews.

## Storefront and console

- **dupli1-web:** add the PDP summary row and section (design linked above), reading from public `GET …/reviews`. On the order page, a 리뷰 쓰기 button per line once the order is `fulfilled` (구매확정하고 리뷰 쓰기 while `delivered`), and 마이페이지 → 내 리뷰. Writes go through `/auth/session/gateway`.
- **dupli1-manage-web:** a 리뷰 tab: list, filter by rating and product, hide and unhide with a reason.
- **Permissions:** `product.review.moderate` is in the catalog and the `support_agent` bundle, since the people who answer 상품 문의 are the ones reading reviews. `product.*` covers it.
- **Configuration:** product reads `DUPLI1_GATEWAY_URL` (the gateway's internal listener, as order does). It is read lazily, so product does not wait on the proxy at startup. Without it every other review route works and new reviews answer `503`.

## Phases

- [x] **1 — Core:** tables and counters; public list and summary; create, edit and delete; the order check; `reviewCount` / `ratingAverage` on products; `user.deleted` deletes the account's reviews and recomputes counters; the masked author from profile; [api.md](api.md), [endpoints.md](endpoints.md), [permissions.md](permissions.md), [current-state.md](current-state.md).
- [x] **2 — Photos:** upload, re-encode with orientation, attach, hourly orphan sweep, the photo strip endpoint.
- [ ] **3 — Storefront:** PDP block, order-page write flow, 내 리뷰 (dupli1-web).
- [ ] **4 — Console:** API and `product.review.moderate` done; the manage-web tab is not.
- [ ] OpenAPI for the review routes.
- [ ] **5 — Later:** the low-rating ops alert, stars on product cards, sort by rating in listings.
