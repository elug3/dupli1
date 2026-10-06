# Support — product questions (상품 문의)

A signed-in shopper asks about a product from its page, and staff answer from the manage-web `/support` console. Questions are **private**. Only the shopper who asked and staff ever see a question or its answer, so there is no public Q&A list on the product page (decided 2026-10-06). Compared with the 1:1 web chat ([support-web-chat.md](support-web-chat.md)), a product question is one question and one answer, filed under the product and variant it is about. The shopper finds it on that product's page and in 마이페이지 → 상품 문의.

It lives in `support` beside the Telegram bot ([support-telegram-bot.md](support-telegram-bot.md)) and web chat. Those already hold the staff inbox, the `support.*` permissions, the ops alert path, the answer email, retention and the `user.deleted` handling. Design (screens and flow): the Figma file linked from the "Design Product Inquiry" project thread.

## Rules

- Only a signed-in customer or manager account can ask. Service accounts get `403`.
- A question names the **variant** the shopper had selected (`sku_id`). Support checks it against the catalog through the gateway, as web chat does, and it must belong to the product in the path. The question keeps a snapshot of the variant label (`Black / M`) and the product name.
- The four types are `size`, `stock`, `product` and `other`. There is no order or delivery type on purpose. Those questions need the shopper's order, which belongs in the 1:1 chat.
- A `size` question may carry `fit`: `height_cm` (100–230), `weight_kg` (30–200) and `usual_size` (up to 10 characters). Any other type with `fit` is `400`.
- The author may edit or withdraw a question until it is answered. After that it is locked (`409 question_answered`), so an answer never ends up under a question it did not answer.
- Staff may answer again to correct an answer. Only the first answer emails the shopper.
- Staff may **hide** a question (spam, abuse, a duplicate). It moves from the waiting queue to the hidden one, and unhiding moves it back. Its author still sees it, unanswered, and never sees the hidden flag.
- The shopper never sees staff fields: their own account id and email, which manager answered, and the hidden state.
- At most 5 questions a minute and 30 a day per account, per replica (`429 rate_limited`).

## Notifications

- **To staff:** a new question publishes `support.inquiry_opened` with `channel: "product_question"`, topic `prd`, the product and variant as `entry_context`, and an excerpt of up to 200 characters. Notification sends it to chats with `alert_support`, titled 상품 문의 and linking to `/support?tab=questions&question=<id>`. A lost alert does not fail the question. It still waits in the console.
- **To the shopper:** the first answer sends the web chat's reply-notice email. The subject is the product name and the link is `DUPLI1_STOREFRONT_URL/product/<id>?qna=<question id>`. Like the chat notice, the email never carries the answer text. Without SMTP or a storefront URL, no email is sent and nothing else changes.

## Retention and account deletion

Questions follow the transcript rules. The daily retention sweep (`DUPLI1_SUPPORT_MESSAGE_RETENTION_DAYS`, default 180) replaces the text of older questions with the purge placeholder and drops their fit. On `user.deleted`, every question the account asked has its text replaced with `(탈퇴한 회원의 문의입니다)`, and its fit and email are cleared. In both cases the row and staff's answer stay.

## API

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/api/v1/support/products/{productId}/questions` | Bearer (customer/manager) | The caller's own questions about this product, newest first: `{ questions }` |
| POST | `/api/v1/support/products/{productId}/questions` | Bearer | `{ sku_id, type, body, fit? }` → `201` with the question |
| GET | `/api/v1/support/me/product-questions` | Bearer | All of the caller's questions, newest first |
| PATCH | `/api/v1/support/me/product-questions/{id}` | Bearer | `{ type, body, fit? }`. The product and variant stay as asked |
| DELETE | `/api/v1/support/me/product-questions/{id}` | Bearer | `204` |
| GET | `/api/v1/support/product-questions` | `support.read` | Console queue: `?queue=waiting` (default, oldest first), `answered`, `hidden`; `?type=`, `?product_id=`, `?limit=` (≤200) |
| GET | `/api/v1/support/product-questions/{id}` | `support.read` | One question with the staff fields |
| POST | `/api/v1/support/product-questions/{id}/answer` | `support.reply` | `{ answer }`, 1–2000 characters |
| POST | `/api/v1/support/product-questions/{id}/hide` | `support.reply` | `{ hidden }`, default `true`. `false` puts it back in the queue |

A question on the wire: `id`, `product_id`, `sku_id`, `variant_label`, `product_name`, `type`, `body`, `fit`, `status` (`waiting` / `answered`), `answer`, `answered_at`, `created_at`, `updated_at`, `editable` (the shopper may still change it), and for staff `customer_id`, `customer_email`, `answered_by`, `hidden`, `hidden_by`, `hidden_at`.

Errors carry a `code`: `invalid_question` 400, `invalid_answer` 400, `invalid_reference` 422 (unknown SKU, or a SKU of another product), `reference_unavailable` 503 (no gateway, or the catalog did not answer), `rate_limited` 429, `question_not_found` 404 (also for someone else's question), `question_answered` 409, `customer_required` 403, `questions_unavailable` 503.

## Storage

The `support_product_questions` table, created inline like the rest of support's schema. It is indexed by customer and product (the shopper's views), and by status for the queue.

## Not yet

- **Deployment.** support is not in production yet ([support-telegram-bot.md](support-telegram-bot.md#deploying-phase-7)). Product questions go live with its first deployment, under `/api/v1/support/`, which the gateway already routes.
