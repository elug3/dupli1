#!/usr/bin/env python3
"""Seed Dupli1 with top luxury bags from a curated multi-brand catalog.

Catalog uses Korean product names and ``<img src>`` URLs from
``scripts/catalog/luxury_bags.html`` (run ``build_luxury_catalog.py`` first).

Brands (Korean aliases accepted):
  미우미우 / miu-miu / miu miu
  구찌 / gucci
  보테가 / bottega / bottega-veneta
  디올 / dior
  loewe / 로에베
  셀린느 / celine
  생로랑 / saint-laurent / ysl

Usage::

    python scripts/build_luxury_catalog.py --write-all
    python scripts/seed_luxury_bags.py
    python scripts/seed_luxury_bags.py --limit 15
    python scripts/seed_luxury_bags.py --brands gucci,dior,loewe
    python scripts/seed_luxury_bags.py --upload-images
    python scripts/seed_luxury_bags.py --dry-run
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def _configure_stdio() -> None:
    """Avoid UnicodeEncodeError on Windows cp949 consoles."""
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass


_configure_stdio()

# Reuse auth/import helpers from the gallery importer.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from import_gallery_products import (  # noqa: E402
    load_existing_ids,
    login,
    request_json,
)

CATALOG_PATH = Path(__file__).resolve().parent / "catalog" / "luxury_bags.json"

BRAND_ALIASES: dict[str, str] = {
    "miu-miu": "miu-miu",
    "miumiu": "miu-miu",
    "miu miu": "miu-miu",
    "미우미우": "miu-miu",
    "gucci": "gucci",
    "구찌": "gucci",
    "bottega-veneta": "bottega-veneta",
    "bottega": "bottega-veneta",
    "bottegaveneta": "bottega-veneta",
    "보테가": "bottega-veneta",
    "보테가베네타": "bottega-veneta",
    "dior": "dior",
    "디올": "dior",
    "loewe": "loewe",
    "로에베": "loewe",
    "celine": "celine",
    "셀린느": "celine",
    "셀린": "celine",
    "saint-laurent": "saint-laurent",
    "saintlaurent": "saint-laurent",
    "ysl": "saint-laurent",
    "생로랑": "saint-laurent",
    "생 로랑": "saint-laurent",
    "fendi": "fendi",
    "펜디": "fendi",
    "텐디": "fendi",
}

DEFAULT_BRANDS = [
    "miu-miu",
    "gucci",
    "bottega-veneta",
    "dior",
    "loewe",
    "celine",
    "saint-laurent",
]


def resolve_brand_key(name: str) -> str:
    key = BRAND_ALIASES.get(name.strip().lower())
    if not key:
        key = BRAND_ALIASES.get(name.strip())
    if not key:
        raise ValueError(f"unknown brand: {name!r}")
    return key


def load_catalog(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        data = json.load(handle)
    brands = data.get("brands")
    if not isinstance(brands, dict):
        raise ValueError("catalog missing brands object")
    return brands


def to_api_product(item: dict, brand_name: str) -> dict:
    return {
        "id": item["id"],
        "name": item["name"],
        "description": item.get("description") or "",
        "price": float(item["price"]),
        "brand": brand_name,
        "color": item.get("color") or "",
        "material": item.get("material") or "",
        "stock": int(item.get("stock") or 5),
        "category": "bags",
        "status": "active",
        "imageUrls": item.get("imageUrls") or item.get("image_urls") or [],
        "capacity": item.get("capacity") or "",
    }


def update_product(base_url: str, token: str, product: dict) -> tuple[int, dict | list | str]:
    return request_json(
        "PUT",
        f"{base_url}/api/v1/products/{product['id']}",
        product,
        token=token,
    )


def seed_brands(
    base_url: str,
    token: str,
    catalog: dict,
    brand_keys: list[str],
    limit: int,
    dry_run: bool,
    update_existing: bool,
) -> tuple[int, int, int, int]:
    existing = load_existing_ids(base_url, token)
    created = updated = skipped = failed = 0

    for key in brand_keys:
        entry = catalog.get(key)
        if not entry:
            print(f"warn  catalog has no brand key {key!r}", file=sys.stderr)
            continue

        brand_name = entry["name"]
        products = list(entry.get("products") or [])[:limit]
        print(f"\n=== {brand_name} ({len(products)} products) ===")

        for item in products:
            product = to_api_product(item, brand_name)
            product_id = product["id"]

            if product_id in existing:
                if not update_existing:
                    print(f"skip  {product_id}: already exists")
                    skipped += 1
                    continue
                if dry_run:
                    print(f"dry   {product_id}: update {product['name']}")
                    updated += 1
                    continue
                status, payload = update_product(base_url, token, product)
                if status == 200 and isinstance(payload, dict):
                    print(f"upd   {product_id}: {payload.get('name', product['name'])}")
                    updated += 1
                    continue
                print(f"fail  {product_id}: {status} {payload}", file=sys.stderr)
                failed += 1
                continue

            if dry_run:
                print(f"dry   {product_id}: {product['name']} ({product['price']:.0f})")
                created += 1
                continue

            status, payload = request_json(
                "POST",
                f"{base_url}/api/v1/products",
                product,
                token=token,
            )
            if status == 201 and isinstance(payload, dict):
                print(f"added {payload.get('id', product_id)}: {payload.get('name', product['name'])}")
                existing.add(payload.get("id", product_id))
                created += 1
                continue

            print(f"fail  {product_id}: {status} {payload}", file=sys.stderr)
            failed += 1

    return created, updated, skipped, failed


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--email", default="admin@dupli1.com")
    parser.add_argument("--password", default="password")
    parser.add_argument(
        "--catalog",
        type=Path,
        default=CATALOG_PATH,
        help="Path to luxury_bags.json",
    )
    parser.add_argument(
        "--brands",
        default=",".join(DEFAULT_BRANDS),
        help="Comma-separated brand keys or Korean names",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=18,
        help="Max products per brand (default: 18, range 15-20)",
    )
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument(
        "--build-catalog",
        action="store_true",
        help="Run build_luxury_catalog.py --write-all before seeding",
    )
    parser.add_argument(
        "--update-existing",
        action="store_true",
        help="PUT Korean names and imageUrls when product id already exists",
    )
    parser.add_argument(
        "--upload-images",
        action="store_true",
        help="Upload remote imageUrls to MinIO after seeding (requires image route)",
    )
    args = parser.parse_args(argv)

    if args.build_catalog:
        from build_luxury_catalog import main as build_main  # noqa: E402

        if build_main(["--write-all"]) != 0:
            return 2

    if args.limit < 1:
        print("--limit must be >= 1", file=sys.stderr)
        return 2

    try:
        brand_keys = [resolve_brand_key(part) for part in args.brands.split(",") if part.strip()]
    except ValueError as exc:
        print(exc, file=sys.stderr)
        return 2

    if not args.catalog.is_file():
        print(f"catalog not found: {args.catalog}", file=sys.stderr)
        return 2

    catalog = load_catalog(args.catalog)
    token = login(args.base_url, args.email, args.password)
    created, updated, skipped, failed = seed_brands(
        args.base_url,
        token,
        catalog,
        brand_keys,
        args.limit,
        args.dry_run,
        args.update_existing,
    )
    print(f"\ndone: created={created} updated={updated} skipped={skipped} failed={failed}")

    if args.upload_images and not args.dry_run and failed == 0:
        from upload_product_images import main as upload_main  # noqa: E402

        print("\n=== uploading product images ===")
        upload_rc = upload_main([])
        if upload_rc != 0:
            return upload_rc

    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
