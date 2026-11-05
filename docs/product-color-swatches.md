# Color swatches

The storefront draws each color of a product as a round swatch: 28px in the
product page's color picker, 12px on product cards. Since 2026-11-02 the fill
comes from the **color master**, not from a name-to-hex guess in the
storefront. Design: [Figma](https://www.figma.com/design/tqtyEpto8TYyJqqN2HjWth)
(Linear SER-99, SER-100).

## One color per color master row

A color master row is **one color**. A combination of two colors (black
body, brown trim) is not a color master row: it never matches another
product's variants, so it cannot be reused for filtering or swatches.
Combining colors belongs to the sku, not to the master.

Until 2026-11-05 a row could carry a second color (`hex2`, then
`base2_code`). Both columns are dropped on startup, and the API refuses them
(see below).

## Data

Two tables:

- **`colors`** (the SKU color master, [product-sku-system.md](product-sku-system.md))
  holds **colorways**: the code the sku carries, usually the brand's own
  (`1139` "블랙", `F0002` "블랙"). Two colorways may share a
  name and stay distinct; a style can sell both.
- **`base_colors`** holds the **plain colors** shoppers recognize (`BLK`
  Black, `BRN` Brown), seeded from `domain.SeedColors`. Each colorway points
  at one (`base_code`); `NULL` = not assigned yet. The foreign key is
  `RESTRICT`.

| Column / JSON | Meaning |
|---|---|
| `base_colors.hex` | The base color's swatch fill, lowercase `#rrggbb`. |
| `colors.base_code` / `baseCode` | The colorway's base color. |
| `colors.hex` | Optional colorway-specific fill that overrides its base color's. |

Swatch fill on a variant read: `colorHex` = the colorway's own `hex`, else
its base color's.

| Colorway | Swatch |
|---|---|
| a base color or its own hex | Solid |
| no base, no hex | Unknown: dashed neutral circle ("no color data") |

Hex input accepts `#RRGGBB` or `RRGGBB` in any case and stores `#rrggbb`.
Each seed colorway is its own base color (`BLK` → `BLK`). On startup a seed
color or base color whose `hex` is `""` gets the seeded one; a hex staff set
is never overwritten.

## API

Permissions are unchanged (`product.master.read|write`).

- `GET /api/v1/products/catalog/base-colors` → `[{ code, name, hex? }]`
- `POST …/catalog/base-colors` `{ code, name, hex? }`;
  `PATCH …/catalog/base-colors/{code}` `{ name?, hex? }`;
  `DELETE …` is `409` while a colorway points at it.
- `GET /api/v1/products/catalog/colors` → `[{ code, name, hex?, baseCode? }]`
- `POST …/catalog/colors` `{ code, name, hex?, baseCode? }`
- `PATCH …/catalog/colors/{code}` `{ name?, hex?, baseCode? }`: an omitted
  field is kept, `""` clears it; an unknown base color is `404`.
- A non-empty `hex2` or `base2Code` on either write is `400`. An empty one
  (`""`) is accepted and ignored, because older clients always send
  `hex2: ""`.

Every variant read (PDP `variants[]`, `GET /products/{id}/variants`,
by-SKU and by-SKU-id lookups) carries `baseColorCode` and the swatch fill
`colorHex`, omitted when empty. They are response-only, read from `colors`
and `base_colors` at query time, so editing either changes every product at
once. `baseColor2Code` and `colorHex2` are no longer returned.

Products carry `availableBaseColors` (base color codes of their active
variants), and search takes `?baseColor=BLK` (alias `base_color`), matching
a product with an active variant whose colorway has that base color.

## Clients

- **dupli1-web:** `colorHex` decides the swatch. A variant without
  `colorHex` still falls back to the old name list, then to the unknown
  swatch. Without `colorHex2` the two-tone branch is never taken. A
  shopper-facing color filter should list base colors
  (`availableBaseColors`, `?baseColor=`), not colorway names.
- **dupli1-manage-web:** `/catalog` → Colors should pick one `baseCode`
  from the base colors and stop offering a second fill (`hex2`); a Base
  colors tab edits the palette.

## Assigning base colors

Colorways created before base colors existed have none. An operator script
outside this repo guesses them from colorway names ("다크 캐러멜" → `TAN`)
and writes a plan for review, then sets them through
`PATCH …/catalog/colors/{code}`; names with no plain color word ("오팔린") are
left for staff.
