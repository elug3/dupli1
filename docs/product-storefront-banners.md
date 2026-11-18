# Storefront image banners

**Status:** built 2026-11-18. Product service, manage-web `/banners`, storefront home hero.

## What it is

Image banners that staff upload in manage-web, switch on, and take down by hand. There is no schedule (serial, 2026-11-18: banners go up and down manually). The login page's welcome-code block is **not** a banner: it is text in dupli1-web (`signupOffer.*`), since the codes rarely change.

## Model

One table, `storefront_banners`, owned by product:

| Field | Notes |
|-------|-------|
| `placement` | Where it shows. Only `home_hero` today (the home page's top image). A new placement needs a constant in `domain.BannerPlacement` and a storefront slot |
| `image_url`, `mobile_image_url` | Desktop (wide, about 5:2) and an optional phone image (about 4:5). Uploaded through `POST /api/v1/products/banners/images` to the product image bucket under `banners/`, unchanged |
| `title`, `link_label` | `{ ko, en, zh }`, optional; an empty language falls back to Korean |
| `link_url` | Optional click target: a storefront path or `https://` |
| `active` | The switch. A new banner starts hidden |
| `sort_order` | Lowest first. The storefront shows the first live banner of a placement |

API: [api.md → Storefront banners](api.md#storefront-banners-apiv1productsbanners). Permission: `banner.manage` (in `catalog_admin`); the live list is public.

## Storefront

dupli1-web's home page reads `GET /api/v1/products/banners?placement=home_hero` on the server. With a live banner it shows that image (the phone image on narrow screens), its title and its link; with none, or when product cannot be reached, it shows the built-in video hero as before. The read is cached for 60 seconds, so a change shows within a minute.

## Not built

- Scheduling (start/end dates).
- More placements (category pages, a strip above the header).
- Cleanup of images no banner uses any more.
