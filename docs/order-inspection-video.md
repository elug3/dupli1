# Order inspection video (검수영상)

Every order is filmed before it ships: staff record the item being inspected, then pack it. Since 2026-11-06 that film is part of the order. Staff upload it, **order refuses to ship without it**, and the shopper can watch it from the moment the parcel is on its way.

## Rules

| Order status | Attach / replace | Ship | Shopper can watch |
|---|---|---|---|
| `pending` | refused (`400`, not paid yet) | no | no |
| `paid`, `confirmed` | yes | `confirmed` only, and only with a video (`409 inspection_video_required`) | no (staff only) |
| `in_transit`, `delivered`, `disputed` | yes (a retake replaces it) | — | yes |
| `fulfilled`, `canceled` | **locked** (`409 inspection_video_locked`) | — | yes, if it was shipped |

- The video locks when the order becomes `fulfilled`, however that happens: the shopper confirming receipt, the 14-day auto-fulfil, or a manager closing it. From then on it is the record of what was sent.
- A replacement is stored under a new key first; the order points at it only once that succeeds, and the old object is then deleted. A failed upload never leaves the order without its earlier video.
- Order checks the video before it commits stock, so a refused ship has no side effects.

## API

| Method | Path | Who | What |
|---|---|---|---|
| `PUT` | `/api/v1/orders/{id}/inspection-video` | `order.ship` | Upload or replace. Raw body (not multipart), `Content-Type: video/mp4`, `video/quicktime` or `video/webm`, `Content-Length` required (`411` without). Over the cap: `413`. Returns the order. |
| `GET` / `HEAD` | `/api/v1/orders/{id}/inspection-video` | staff with `order.read.all`; the order's own customer once shipped | Streams the video. Honours `Range` (`206`), so players can seek. `Cache-Control: private, no-store`. `404` when there is none (or not yet, for the shopper); `403` for someone else's order. |

The order JSON carries the metadata, never the storage key:

```json
"inspection_video": {
  "content_type": "video/mp4",
  "size_bytes": 48211735,
  "uploaded_by": "<staff user id>",
  "uploaded_at": "2026-11-06T09:12:44Z"
},
"inspection_video_locked": false
```

`inspection_video` is absent until one is uploaded. `inspection_video_locked` is computed: true once the order is fulfilled or canceled.

Errors on the upload and on `POST …/ship` carry a `reason`: `inspection_video_required`, `inspection_video_locked`, `invalid_inspection_video` (`400`: wrong type, empty, over the cap) or `inspection_video_storage_unavailable` (`503`).

## Storage

Videos live in a **private** bucket in S3-compatible object storage: MinIO locally, SeaweedFS on VENUS (the same server as product images, but a bucket anonymous users cannot read). The browser never addresses the bucket; it always goes through order, which checks who is asking. Keys are `orders/{order_id}/inspection-{unix_nanos}.{mp4|mov|webm}`.

The metadata is a row in `order_inspection_videos` (one per order), kept apart from `orders` so the whole-row save every status change makes can never drop it. Uploading locks the order row and re-checks the status, so a replacement cannot slip in while the order is being fulfilled.

| Env (order) | Default | |
|---|---|---|
| `DUPLI1_ORDER_S3_ENDPOINT` | unset | e.g. `http://minio:9000`, `http://s3:8333`. **Unset: uploads answer `503` and nothing can ship.** |
| `DUPLI1_ORDER_S3_ACCESS_KEY`, `DUPLI1_ORDER_S3_SECRET_KEY` | unset | |
| `DUPLI1_ORDER_S3_BUCKET` | `order-inspection-videos` | Created at startup if missing (not fatal if storage is down then). |
| `DUPLI1_ORDER_INSPECTION_VIDEO_MAX_BYTES` | `100000000` (100 MB) | Cloudflare, in front of `manage.dupli1.com`, refuses a request body over 100 MB, so raising this alone does not help uploads through the console. |

`GET /settings` reports `features.inspection_video` and `limits.inspection_video_max_bytes`.

The gateway (`api/gateway/routes.conf`) gives this one route a 100 MB body limit, unbuffered upload and download, and 15-minute timeouts; order lifts its own 5 s read / 25 s write timeouts for the same route. The VENUS edge allows 100 MB bodies to `manage.dupli1.com`.

## Not here yet

- manage-web: upload control on the order page, Ship disabled until a video is attached.
- dupli1-web: the 검수영상 section on the order details page (design: Figma "dupli1 Order details page").
- Transcoding or thumbnails. Phone footage is stored as uploaded; `.mov` (iPhone default) plays in Safari but not in every desktop browser.
