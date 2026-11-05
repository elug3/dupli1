# Color swatches and two-tone colors

The storefront draws each color of a product as a round swatch: 28px in the
product page's color picker, 12px on product cards. Since 2026-11-02 the fill
comes from the **color master**, not from a name-to-hex guess in the
storefront. Design: [Figma](https://www.figma.com/design/tqtyEpto8TYyJqqN2HjWth)
(Linear SER-99, SER-100).

## Data

Two tables:

- **`colors`** (the SKU color master, [product-sku-system.md](product-sku-system.md))
  holds **colorways**: the code the sku carries, usually the brand's own
  (`1139` "블랙", `F0002` "블랙"). Two colorways may share a
  name and stay distinct; a style can sell both.
- **`base_colors`** holds the **plain colors** shoppers recognize (`BLK`
  Black, `BRN` Brown), seeded from `domain.SeedColors`. Each colorway points
  at one (`base_code`) or, for a two-tone colorway (black body, brown trim),
  two (`base2_code`). `NULL` = not assigned yet. Foreign keys are
  `RESTRICT`, and `base2_code` needs `base_code` and must differ from it.

| Column / JSON | Meaning |
|---|---|
| `base_colors.hex` | The base color's swatch fill, lowercase `#rrggbb`. |
| `colors.base_code` / `baseCode` | The colorway's (main) base color. |
| `colors.base2_code` / `base2Code` | Second base color of a two-tone colorway. |
| `colors.hex` | Optional colorway-specific fill that overrides its base color's. |
| `colors.hex2` | **Retired** two-tone fill: read if present, never set (`400`); `""` clears it. |

Swatch fills on a variant read: `colorHex` = the colorway's own `hex`, else
its base color's; `colorHex2` = the second base color's, else a legacy
`hex2`.

| Colorway | Swatch |
|---|---|
| one base (or own hex) | Solid |
| two bases | Two-tone: diagonal split, main top-left, second bottom-right |
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
- `GET /api/v1/products/catalog/colors` → `[{ code, name, hex?, baseCode?, base2Code?, hex2? }]`
- `POST …/catalog/colors` `{ code, name, hex?, baseCode?, base2Code? }`
- `PATCH …/catalog/colors/{code}` `{ name?, hex?, baseCode?, base2Code?, hex2? }`:
  an omitted field is kept, `""` clears it; a non-empty `hex2`, a `base2Code`
  without `baseCode` or equal to it is `400`, an unknown base color `404`.

Every variant read (PDP `variants[]`, `GET /products/{id}/variants`,
by-SKU and by-SKU-id lookups) carries `baseColorCode` / `baseColor2Code` and
the swatch fills `colorHex` / `colorHex2`, omitted when empty. They are
response-only, read from `colors` and `base_colors` at query time, so editing
either changes every product at once.

Products carry `availableBaseColors` (base color codes of their active
variants, both colors of a two-tone one), and search takes
`?baseColor=BLK` (alias `base_color`), matching a product with an active
variant whose colorway has that base color as either base.

## Clients

- **dupli1-web:** `colorHex` / `colorHex2` decide the swatch. A variant without
  `colorHex` still falls back to the old name list, then to the unknown
  swatch. A shopper-facing color filter should list base colors
  (`availableBaseColors`, `?baseColor=`), not colorway names.
- **dupli1-manage-web:** `/catalog` → Colors should pick `baseCode` /
  `base2Code` from the base colors instead of editing `hex2`, and a Base
  colors tab edits the palette.

## Assigning base colors

Colorways created before base colors existed have none. An operator script
outside this repo guesses them from colorway names ("다크 캐러멜" → `TAN`,
"블랙/브라운" → `BLK` + `BRN`), writes a plan for review, then sets them through
`PATCH …/catalog/colors/{code}`; names with no plain color word ("오팔린") are
left for staff.
