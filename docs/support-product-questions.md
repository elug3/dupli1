# Support — product questions (상품 문의)

A shopper asks about a product from its page, staff answer from the manage-web `/support` console, and the answer is published under the question. This is the per-product Q&A Korean shoppers expect, not a consultation: anyone can read it, and an answer to a size question helps the next shopper choose too. A shopper who wants privacy marks the question **비밀글**. Then only they and staff see its words.

It lives in `support` beside the Telegram bot ([support-telegram-bot.md](support-telegram-bot.md)) and web chat ([support-web-chat.md](support-web-chat.md)). Those already hold the staff inbox, the `support.*` permissions, the ops alert path, the answer email and the `user.deleted` handling. Design (screens and flow): the Figma file linked from the "Design Product Inquiry" project thread.

## Rules

- Anyone can read a product's questions. Only a signed-in customer or manager account can ask, and service accounts get `403`.
- A question names the **variant** the shopper had selected (`sku_id`). Support checks it against the catalog through the gateway, as web chat does, and it must belong to the product in the path. The question keeps a snapshot of the variant label (`Black / M`) and the product name.
- The four types are `size`, `stock`, `product` and `other`. There is no order or delivery type on purpose. Those questions need the shopper's order, which belongs in the private 1:1 chat, not on a public page.
- A `size` question may carry `fit`: `height_cm` (100–230), `weight_kg` (30–200) and `usual_size` (up to 10 characters). Any other type with `fit` is `400`. Fit is published with a public question. It holds nothing that identifies anyone.
- Authors appear only as a mask: the first two characters of the email and `****` (`s3****`). The email, the account id, and which manager answered or hid a question are returned to staff only.
- A secret question asked by someone else is still listed, so counts add up. Its `body`, `fit`, `answer` and `variant_label` are empty and `redacted` is `true`, and the storefront shows "비밀글입니다".
- The author may edit or withdraw a question until it is answered. After that it is locked (`409 question_answered`), so an answer never ends up under a question it did not answer.
- Staff may answer again to correct an answer. Only the first answer emails the shopper.
- Staff may **hide** a question (spam, abuse, personal details). It leaves the product page and the waiting queue but keeps its status, and it can be shown again.
- At most 5 questions a minute and 30 a day per account, per replica (`429 rate_limited`).

## Notifications

- **To staff:** a new question publishes `support.inquiry_opened` with `channel: "product_question"`, topic `prd`, the product and variant as `entry_context`, and an excerpt of up to 200 characters. Notification sends it to chats with `alert_support`, titled 상품 문의 and linking to `/support?tab=questions&question=<id>`. A lost alert does not fail the question. It still waits in the console.
- **To the shopper:** the first answer sends the web chat's reply-notice email. The subject is the product name and the link is `DUPLI1_STOREFRONT_URL/product/<id>?qna=<question id>`. Like the chat notice, the email never carries the answer text. Without SMTP or a storefront URL, no email is sent and nothing else changes.

## Account deletion

On `user.deleted`, every question the account asked has its body replaced with `(탈퇴한 회원의 문의입니다)`. Its fit and email are cleared and the author becomes `탈퇴 회원`. The answer stays, so the product page keeps what other shoppers relied on.

## API

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/api/v1/support/products/{productId}/questions` | none (Bearer optional) | `?type=size|stock|product|other`, `?answered=true`, `?mine=true`, `?page=` (20 per page). Answers `{ questions, total, counts, page, has_more }`. `counts` is per type, over every visible question, before filtering |
| POST | `/api/v1/support/products/{productId}/questions` | Bearer (customer/manager) | `{ sku_id, type, body, secret?, fit? }` → `201` with the question |
| GET | `/api/v1/support/me/product-questions` | Bearer | The caller's own questions, newest first |
| PATCH | `/api/v1/support/me/product-questions/{id}` | Bearer | `{ type, body, secret?, fit? }`. The product and variant stay as asked |
| DELETE | `/api/v1/support/me/product-questions/{id}` | Bearer | `204` |
| GET | `/api/v1/support/product-questions` | `support.read` | Console queue: `?queue=waiting` (default, oldest first), `answered`, `hidden`; `?type=`, `?product_id=`, `?limit=` (≤200) |
| GET | `/api/v1/support/product-questions/{id}` | `support.read` | One question with the staff fields |
| POST | `/api/v1/support/product-questions/{id}/answer` | `support.reply` | `{ answer }`, 1–2000 characters |
| POST | `/api/v1/support/product-questions/{id}/hide` | `support.reply` | `{ hidden }`, default `true`. `false` shows the question again |

A question on the wire: `id`, `product_id`, `sku_id`, `variant_label`, `product_name`, `author`, `type`, `body`, `fit`, `secret`, `status` (`waiting` / `answered`), `answer`, `answered_at`, `created_at`, `updated_at`, `redacted`, `mine`, `editable`, and for staff `customer_id`, `customer_email`, `answered_by`, `hidden`, `hidden_by`, `hidden_at`.

Errors carry a `code`: `invalid_question` 400, `invalid_answer` 400, `invalid_reference` 422 (unknown SKU, or a SKU of another product), `reference_unavailable` 503 (no gateway, or the catalog did not answer), `rate_limited` 429, `question_not_found` 404 (also for someone else's question), `question_answered` 409, `customer_required` 403, `questions_unavailable` 503.

Without `AUTH_JWKS_URL` / `JWT_SECRET` the public list is still served, anonymously. Every other route answers `503`.

## Storage

The `support_product_questions` table, created inline like the rest of support's schema. It is indexed by product, by customer, and by status for the queue. A product collects tens of questions, so the product list is filtered and paged in the service, not in SQL.

## Not yet

- **Deployment.** support is not in production yet ([support-telegram-bot.md](support-telegram-bot.md#deploying-phase-7)). Product questions go live with its first deployment. The public route is under `/api/v1/support/`, which the gateway already routes.
- **Retention.** The 180-day purge covers consultation transcripts, not product questions. A public question is catalog content that other shoppers rely on, so it stays until its author withdraws it or deletes their account, or staff hide it.
