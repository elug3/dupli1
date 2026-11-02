# Color swatches and two-tone colors

The storefront draws each color of a product as a round swatch: 28px in the
product page's color picker, 12px on product cards. Since 2026-11-02 the fill
comes from the **color master**, not from a name-to-hex guess in the
storefront. Design: [Figma](https://www.figma.com/design/tqtyEpto8TYyJqqN2HjWth)
(Linear SER-99, SER-100).

## Data

`colors` (the SKU color master, [product-sku-system.md](product-sku-system.md))
has two columns, both `TEXT NOT NULL DEFAULT ''`:

| Column / JSON | Meaning |
|---|---|
| `hex` | The swatch fill, lowercase `#rrggbb`. `""` = no swatch color yet. |
| `hex2` | The second color of a **two-tone** color (black body, brown trim). Needs `hex`. |

| `hex` | `hex2` | Swatch |
|---|---|---|
| set | `""` | Solid |
| set | set | Two-tone: diagonal split, `hex` top-left, `hex2` bottom-right |
| `""` | `""` | Unknown: dashed neutral circle ("no color data", add a hex) |

A two-tone color is its own color master row (for example `BKB` "Black /
Brown"), so a SKU still has exactly one `colorCode`. Name it with both colors
joined by ` / `; the storefront shows that name in the picker legend.

Input accepts `#RRGGBB` or `RRGGBB` in any case and stores `#rrggbb`. Anything
else, or `hex2` without `hex`, is `400`.

Seed colors carry the fills the storefront used before (`domain.SeedColors`).
On startup a seed color whose `hex` is `""` gets the seeded one; a hex staff set
or changed is never overwritten, so a seed color cannot be left without a hex.

## API

- `GET /api/v1/products/catalog/colors` → `[{ code, name, hex?, hex2? }]`
- `POST …/catalog/colors` `{ code, name, hex?, hex2? }`
- `PATCH …/catalog/colors/{code}` `{ name?, hex?, hex2? }`: an omitted field is
  kept, `""` clears a hex. Clearing `hex` while `hex2` is set is `400`.

Permissions are unchanged (`product.master.read|write`).

Every variant read (PDP `variants[]`, `GET /products/{id}/variants`,
by-SKU and by-SKU-id lookups) carries the color master's fills as
`colorHex` / `colorHex2`, omitted when empty. They are response-only, read from
`colors` at query time, so editing a color changes every product at once.

## Clients

- **dupli1-web:** `colorHex` / `colorHex2` decide the swatch. A variant without
  `colorHex` (a color created on the fly from a variant's name) still falls
  back to the old name list, then to the unknown swatch.
- **dupli1-manage-web:** `/catalog` → Colors edits `hex` and `hex2` with a live
  swatch preview.
