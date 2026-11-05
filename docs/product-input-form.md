# Standard product input form

**Status:** As built (2026-11-05). This is the form operators use in `dupli1-manage-web` today. It records that form. It does not add a required-field policy, a swipe-to-add-SKU layout, or a seller photo flow — those stay open under **Meeting follow-ups (2026-10-31)** in [TODO.md](TODO.md).

**Where:** manage console.

| Screen | Route | What it does |
|--------|--------|----------------|
| New product | `/products/new` | Two steps: parent, then one SKU at a time |
| Product | `/products/:id` | Edit the parent, add another SKU, galleries, clothing size chart |
| SKU | `/products/:id/SKU/:skuId` | That SKU's price, measurements, gallery |
| Masters | `/catalog` | Brand, style, color, size, edition dictionaries the form selects from |

A product cannot be created until the brand and style exist. The new-product page links to `/catalog` and can create a style under the selected brand (`product.master.write`). Colors, sizes, and editions are only chosen, not typed.

**Who:** `catalog_editor` covers create, variant create, image upload, and master read/write. Setting on-hand stock also needs `inventory.stock.write`, which that bundle does not include. See [permissions.md](permissions.md).

Money on this form is **whole Korean won** in `price` and `officialPrice`. Do not divide by 100. The parent holds the default; a SKU may override it later. Variant reads return the effective price. See [product-price-on-parent.md](product-price-on-parent.md).

The 2026-10-31 meeting asked h-global's registration to work in a similar way. That site is a separate repository. This page is only the Dupli1 form.

## Step 1 — parent (`POST /api/v1/products`)

`product.create`. The console will not leave this step without the rows marked required. The API's hard checks are the master codes and taxonomy, called out in the last column. An empty name or material can still be stored.

| Field | JSON | Console | Notes |
|-------|------|---------|--------|
| Name | `name` | Required | Marketing name. Not part of the human `sku`. |
| Category | `category` | Required, default `bags` | `bags` or `clothing`. Unknown category is `400`. |
| Subcategory | `subCategory` | Optional | Must belong to the category. |
| Brand | `brandCode` | Required | Existing master. Display `brand` is filled from the master's name. |
| Style | `styleCode` | Required | Existing style under that brand, or one created on this page. |
| Material | `material` | Required | Free text. |
| Description | `description` | Optional | Long prose. |
| Official price | `officialPrice` | Optional | Whole won, `≥ 0`. Reference price, not what is charged. |
| Price | `price` | Required | Whole won, `≥ 0`. Default sale price for every SKU that does not override it. |
| Status | `status` | Optional, default `active` | `active`, `draft`, or `archived`. Blank on the API becomes `active`. |

Not on this step (edited later on the product page): look `style`, `target`, parent `attributes`, and the clothing size chart. `style` here is the merchandising look, not `styleCode`. See [product-master-catalog.md](product-master-catalog.md) and [product-attributes.md](product-attributes.md).

`brandCode` is 2–3 letters. `styleCode`, `colorCode`, `sizeCode`, and `editionCode` are 1–12 letters or digits. The service uppercases them and strips anything else before that check. A missing or illegal code is `400`. A code that is not in the catalog is `404`. The parent `id` is a new ULID. Human identity is `brandCode` + `styleCode`, not the id. See [product-sku-system.md](product-sku-system.md).

## Step 2 — one SKU (`POST /api/v1/products/{id}/variants`)

`product.variant.create`. The same fields are on **Add variant** on the product page. Each submit creates one SKU. Finish is allowed with none. Color and size are two dropdowns side by side on a wide screen.

| Field | JSON | Console | Notes |
|-------|------|---------|--------|
| Color | `colorCode` | Required | Existing color master. Display `color` is that master's name. |
| Size | `sizeCode` | Required | Existing size master. Default `OS` for bags, `M` for clothing. |
| Edition | `editionCode` | Optional | Existing edition master. Omitted from the human `sku` when blank. |
| Status | `status` | Default `active` | `active`, `draft`, or `archived`. |
| Initial stock | — | Optional | Whole units, `≥ 0`. See below. |
| Width, height, depth | `dimensions.widthMm` / `heightMm` / `depthMm` | Optional | Bags only, millimeters. Hidden for clothing. An axis may be omitted. `0`–`10000`. See [product-sku-dimensions.md](product-sku-dimensions.md). |
| Photos | — | Optional | Uploaded after the SKU exists. See below. |

The service composes the human `sku` as `Brand_Style_Color[_Edition]_Size` and assigns a ULID `skuId`. Color, size, and edition codes cannot change later (`400` on `PUT`); a different option is a new SKU. Every new SKU gets a stock row at quantity 0.

A clothing `sizeCode` must be one of that category's sizes (`XXS`–`4XL` and Italian `34`–`60` in S/R/L fits) or the create is `400`. Bag sizes are any size master.

**Initial stock.** A blank field leaves the zero row. A number calls the legacy `PUT /api/v1/inventory/by-sku-id/{skuId}` (`inventory.stock.write`). The canonical path is `PUT /api/v1/products/inventory/items/by-sku-id/{skuId}`. The console keeps the SKU if that call fails, so a `catalog_editor` without the stock permission still gets the SKU at 0. Stock is also editable later on the product page.

**SKU price** is not on this step. On the SKU page, `priceOverride` / `officialPriceOverride` are whole won; blank inherits the parent, and saving `0` clears the override.

## Color

Color on a SKU is the catalog `colorCode`, not a free-text attribute and not a parent `attributes` entry. "Black" is the color master's **name**. Renaming that name does not change the `sku`.

Swatch fills live on the color master: `hex`, and `hex2` for a two-tone color. They are edited on `/catalog` → Colors, not on the product form. A SKU still has exactly one `colorCode`. See [product-color-swatches.md](product-color-swatches.md).

Parent `attributes` is a display memo (condition, care, and, for clothing, shortcuts such as fill weight and outer fabric). It is not searched, not priced, and not the color. At most 32 keys. See [product-attributes.md](product-attributes.md).

## Photos

Photos belong to the SKU (`imageUrls`). The new-product step and the galleries on the product page and the SKU page share one editor.

- Several files can be picked at once. The queue sorts by file name (`2.jpg` before `10.jpg`), then the operator drags or uses the arrows. The first image is the main one.
- Uploads run **one at a time**, in that order: `POST /api/v1/products/{id}/variants/{sku}/images` with multipart field `image` (`product.image.upload`). The service appends each URL. Parallel uploads can drop a file, so the console does not send them in parallel.
- The console refuses a file that is not an image or is over **50MB**. The gateway accepts request bodies up to **20MB** (`client_max_body_size` in `api/gateway/routes.conf` and on the VENUS edge), so a file between those two sizes passes the console and fails at the gateway.
- The service stores the original and writes a listing JPEG sibling (`{key}.w600.jpg`). It must be able to decode the bytes (JPEG, PNG, GIF, or WebP). An empty file is refused. See [product-images-browser-access.md](product-images-browser-access.md).
- A failed upload stops the queue and leaves that file and the ones after it unsent. The SKU is already saved.
- Reordering existing images is one `PUT` of `imageUrls`. There is no review step: a photo is on the SKU as soon as the upload succeeds.
- `POST /api/v1/products/{id}/images` is the same upload onto the product's default variant. The new-product page does not use it; it uploads onto the SKU it just added.

A ZIP catalog import sits next to this form. It is a separate path, not a field on the screen.

## After the wizard

On `/products/:id`, while the product has no SKUs, category can still change. Once a SKU exists, category is locked (a clothing size has to stay legal for that category). Subcategory, look `style`, target, material, both prices, status, description, and attributes stay editable (`product.update`).

Clothing shows a size-chart grid (centimetres, saved on its own as `sizeChart`) and hides the W×H×D editor. Bags show the measurements and no size chart.

Deleting a product or a SKU asks the operator to type the id or the SKU code. A SKU with stock on hand or reserved is `409`. The only SKU of a product cannot be deleted from the console.

## Not this form

- Adding many color × size pairs in one gesture. Today that is one submit per pair.
- A non-manager account uploading photos. There is no seller account type; upload needs `product.image.upload`.
- A second color text field beside `colorCode`.
- Order inspection video. That is filmed outside this form.
