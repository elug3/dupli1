# Moncler padded jackets — scrape fit check and Dupli1 import

**Status:** Imported to the production (VENUS) catalogue on 2026-10-20. 180 Moncler padded-jacket products are live.
**Related:** [product-sku-system.md](product-sku-system.md), [product-master-catalog.md](product-master-catalog.md), [product-sku-dimensions.md](product-sku-dimensions.md), [TODO.md](TODO.md) (October 2026 padded jackets).

This document records where the Moncler data came from, how it was checked
against the Dupli1 product model, the adaptations that made it fit, and how
the import was run and verified. The import scripts, state and downloaded
images live on the ops host (not in this repo), next to the earlier
Prada/Canada Goose jacket imports; they are referred to below by file name.

## Result

| | Count |
|---|---:|
| Scraped colourway SKUs (KR, `ko`) | 357 |
| Scraped styles | 238 |
| **Imported products (all `active`)** | **180** |
| Imported colourways | 283 |
| Imported variants (SKUs) | 1,783 |
| Kids colourways excluded (`아동`) | 74 (58 styles) |
| Size options skipped (Moncler label `8`) | 29 |

Padded jackets in the catalogue after the import
(`category=clothing & subCategory=padded`, counted with
`count-padded-jackets.py`):

| Brand | Products | SKUs |
|---|---:|---:|
| Moncler | 180 | 1,783 |
| Canada Goose | 55 | 581 |
| Prada | 32 | 411 |
| **Total** | **267** | **2,775** |

## Source data (luxscrape)

- Spider: luxscrape `moncler/spider.py`, with the
  padded-jacket classifier in `moncler/padded.py` — a page counts only when
  the page itself names both a fill word (다운/패딩/down/padded/puffer/…) and
  the garment (재킷/jacket); vests, print tees and unfilled shells are
  dropped by the spider, so every record below is already a padded jacket.
- Raw tree: luxscrape `data/moncler/{style}/{sku}/{sku}.json`
  (one luxscrape 22-field record per colourway; scraped
  2026-10-21T02:19–02:32Z), mirrored in
  `data/moncler-padded-jackets.jsonl` (357 rows, same SKU set).
- Records are KR/ko, KRW (official ₩770,000–₩7,840,000, median ₩3,220,000),
  all `InStock`, no missing fields. `dimensions` is `null` on every record —
  harmless for clothing (variant dimensions are optional; see
  [product-sku-dimensions.md](product-sku-dimensions.md)).

## Fit check

Checker: `check-moncler-fit.py` → `moncler-fit-report.json` (read-only).

**Fits as-is:** the luxscrape record shape; `clothing`/`padded` taxonomy;
product grouping (one product per style, no price split inside a style);
colour codes (all valid 3-digit segments); explicit variant SKUs of the form
`<moncler sku>-<size>` (the Canada Goose import pattern); the padded
price band (see below).

**Did not fit — and the adaptation applied:**

| Problem | Detail | Adaptation |
|---|---|---|
| Size codes | 2,215 of 2,218 size options are Moncler labels (`00, 0–8`, kids `4Y–14Y`, `3/6M…`); Dupli1 clothing accepts only `XXS–4XL` + Italian `34–60` (`domain.CheckVariantSize`, hard-coded `ApparelSizes` — size masters do not override it). Only 3 options passed. | Map adult labels to letters (below); kids excluded entirely. |
| Style code length | Every Moncler `style_code` is 17 chars; Dupli1 SKU segments cap at 12 (`^[A-Z0-9]{1,12}$`), so product create would fail. | `styleCode = style_code[:12]`; the 3 truncation-collision groups (6 styles) use `style_code[:7] + style_code[12:]` — all 180 codes unique. Full code kept in `attributes.monclerStyle`. |
| No downloaded images | 2,248 remote Thron CDN URLs, 0 files in the tree; the jacket importer maps URLs to local files and skips records without them. | Importer downloads images to `moncler/{style}/{sku}/` on the host first (1,894 JPEGs, 368 MB). |
| Brand master | `MON` not among the 17 live catalogue brands. | Created via `product.master.write` (agent key), plus 180 style masters and the numeric colour masters. |
| Target for Grenoble | 12 Grenoble colourways carry no gender in `category[0]`. | Gender taken from the URL path (`/men/`, `/women/`); `여성→women`, `남성→men` as in the other jacket imports. |

### Size mapping (adults)

```
00 → XXS    0 → XS    1 → S    2 → M    3 → L
 4 → XL     5 → XXL   6 → XXXL 7 → 4XL
```

- This is a Dupli1 merchandising mapping, applied sequentially; the original
  Moncler label for every size is preserved in the parent attribute
  `attributes.monclerSizes` (e.g. `00=XXS,0=XS,1=S,…`), and the variant SKU
  carries the mapped size (`L20911A5360068950999-M`).
- Label `8` is **skipped**: Dupli1 has no size beyond `4XL`, and mapping it
  to `4XL` would collide with label `7` under the
  `UNIQUE(product_id, color, size)` variant constraint. 29 size options on
  29 colourways are therefore not sold. Labels already valid in Dupli1
  (`38`, `40`, `42` — 3 options) pass through unchanged.
- Sold-out Moncler sizes are still listed and stocked like the earlier
  jacket imports (stock 10 per variant regardless of the page's
  availability flag) — inherited behaviour, noted for merchandising.

## Import

Importer: `import-moncler-padded.py` — reuses
`import-jackets.py`'s API client and import steps (draft → variant images →
stock 10/size → activate), resumable through
`moncler-import-state.json` (keyed by full Moncler style,
holds `productId`/`skuIds`/source).

```bash
# on the ops host, from the importer's directory
python3 import-moncler-padded.py --dry-run   # plan: 180 products, 1,783 variants
python3 import-moncler-padded.py --limit 3   # test batch (verified live first)
python3 import-moncler-padded.py             # remaining 177
```

- Products are named `몽클레르 <name>`, brand `Moncler` / `MON`,
  `category=clothing`, `subCategory=padded`, tags
  `luxscrape, moncler, padded`; attributes carry `source`,
  `sourceMarket`, `monclerStyle`, `monclerSizes`, `monclerCategory`.
- Selling price uses the shared padded band (`sale_price` in
  `import-jackets.py`: official KRW mapped into ₩200,000–₩300,000, the same
  band `adjust-padded-prices.py` maintains for all padded brands).
- Auth is the catalogue agent's API key, exchanged at the internal token
  endpoint; the key is never printed.
- Result: 3 + 177 products, **0 failures**; re-running the importer is a
  no-op (all 180 styles are `active` in the state file).

## Verification

```bash
curl -s "http://127.0.0.1/api/v1/products?category=clothing&subCategory=padded&brand=Moncler&limit=1"
# {"total": 180, ...}
curl -s "http://127.0.0.1/api/v1/products/variants/by-sku/L20911A5360068950999-M"
# Maya, size/sizeCode "M", images attached
python3 count-padded-jackets.py   # Moncler 180 / 1,783 on top of CG 55, Prada 32
```

## Exclusions and known limitations

- **Kids (74 colourways / 58 styles) are not imported.** Their sizes
  (`4Y–14Y`, `3/6M–18/24M`) have no Dupli1 apparel size either; importing
  them needs a kids size decision (and `아동 → kids` target mapping).
- **Moncler size `8` is not sold** (see size mapping).
- No `sizeChart` and no variant `dimensions` — Moncler pages publish
  neither in the scrape; both are optional for clothing.
- To undo: archive (preferred) or delete the 180 `productId`s listed in
  `moncler-import-state.json`; deletion requires the
  variants' stock to be emptied first, per the variant delete rules.

## Artifacts

| File (ops host) | Role |
|---|---|
| `check-moncler-fit.py` | Read-only fit checker |
| `moncler-fit-report.json` | Fit-check output |
| `import-moncler-padded.py` | The importer |
| `moncler-import-state.json` | Resume/audit state (style → productId, skuIds) |
| `moncler/` | Downloaded images (1,894 JPEGs) |
| `padded-jackets-by-brand.json` | Post-import padded counts |
