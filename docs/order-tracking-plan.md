# Order tracking plan

**Status:** Implementing (Phase A + B).  
**Repos:** `dupli1` (order), `dupli1-web`, `dupli1-manage-web`.

## Goal

Customers can list orders and see shipping status + tracking. Ops must enter carrier + tracking number when shipping.

## Decisions

| Decision | Choice |
|----------|--------|
| Tracking on ship | **Required** (`carrier` + `tracking_number`) |
| Carrier list | Fixed KR set: `cj`, `hanjin`, `lotte`, `logen`, `epost`, `other` + `carrier_note` when `other` |
| Storefront route | Dedicated `/orders` + profile tab link |

## Phase A — Storefront order history

- `GET /api/v1/orders?customer_id=` (existing ABAC) → list UI
- `/orders` page with status timeline
- Profile Orders tab + footer/confirmation links → `/orders`

## Phase B — Tracking fields

- Domain + PG: `carrier`, `tracking_number`, `carrier_note`
- `POST /api/v1/orders/{id}/ship` body required
- manage-web ship form
- Storefront display + carrier track URL templates (client-side)

## Out of scope

Live courier APIs, guest order lookup, customer cancel, email ship notifications.
