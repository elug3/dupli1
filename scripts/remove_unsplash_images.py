#!/usr/bin/env python3
"""Remove images.unsplash.com URLs from products and MinIO bucket."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from import_gallery_products import login, request_json  # noqa: E402
from seed_luxury_bags import load_catalog, to_api_product  # noqa: E402

CATALOG_PATH = Path(__file__).resolve().parent / "catalog" / "luxury_bags.json"
UNSPLASH_PREFIX = "https://images.unsplash.com"

LUXURY_ID_PREFIXES = (
    "MM-",
    "GUC-",
    "BV-",
    "DIO-",
    "LOE-",
    "CEL-",
    "YSL-",
    "FEN-",
)


def is_unsplash(url: str) -> bool:
    return (url or "").startswith(UNSPLASH_PREFIX)


def catalog_by_id(catalog: dict) -> dict[str, tuple[dict, str]]:
    out: dict[str, tuple[dict, str]] = {}
    for brand_key, entry in catalog.items():
        for item in entry.get("products") or []:
            out[item["id"]] = (item, entry["name"])
    return out


def strip_unsplash(urls: list[str]) -> list[str]:
    return [u for u in urls if u and not is_unsplash(u)]


def update_variant(
    base_url: str,
    token: str,
    product_id: str,
    variant: dict,
    image_urls: list[str],
    dry_run: bool,
) -> bool:
    body = dict(variant)
    body["imageUrls"] = image_urls
    sku = variant.get("sku") or product_id
    if dry_run:
        print(f"dry   {product_id}/{sku}: -> {len(image_urls)} images")
        return True
    status, resp = request_json(
        "PUT",
        f"{base_url}/api/v1/products/{product_id}/variants/{sku}",
        body,
        token=token,
    )
    if status == 200:
        print(f"upd   {product_id}/{sku}: {len(image_urls)} images")
        return True
    print(f"fail  {product_id}/{sku}: {status} {resp}", file=sys.stderr)
    return False


def clean_api_products(
    base_url: str,
    token: str,
    catalog_map: dict[str, tuple[dict, str]],
    dry_run: bool,
) -> tuple[int, int]:
    status, payload = request_json("GET", f"{base_url}/api/v1/products", token=token)
    products = payload.get("results") if isinstance(payload, dict) else payload
    if status != 200 or not isinstance(products, list):
        raise RuntimeError(f"list products failed ({status}): {payload}")

    updated = unchanged = 0
    for summary in products:
        product_id = summary.get("id", "")
        status, detail = request_json(
            "GET",
            f"{base_url}/api/v1/products/{product_id}",
            token=token,
        )
        if status != 200 or not isinstance(detail, dict):
            print(f"fail  {product_id}: get detail {status}", file=sys.stderr)
            continue

        variants = detail.get("variants") or []
        if not variants and (detail.get("imageUrls") or []):
            variants = [{"sku": product_id, **detail}]

        has_unsplash = any(
            is_unsplash(u)
            for v in variants
            for u in (v.get("imageUrls") or [])
        ) or any(is_unsplash(u) for u in (detail.get("imageUrls") or []))
        if not has_unsplash:
            unchanged += 1
            continue

        if product_id in catalog_map:
            item, brand_name = catalog_map[product_id]
            new_urls = to_api_product(item, brand_name)["imageUrls"]
        else:
            all_urls = []
            for v in variants:
                all_urls.extend(v.get("imageUrls") or [])
            if not all_urls:
                all_urls = detail.get("imageUrls") or []
            new_urls = strip_unsplash(all_urls)

        changed = False
        for variant in variants:
            old = variant.get("imageUrls") or []
            if any(is_unsplash(u) for u in old) or (product_id in catalog_map and old != new_urls):
                if update_variant(base_url, token, product_id, variant, new_urls, dry_run):
                    changed = True

        if changed:
            updated += 1
        else:
            unchanged += 1

    return updated, unchanged


def luxury_minio_prefixes(catalog_map: dict[str, tuple[dict, str]]) -> list[str]:
    prefixes = {pid for pid in catalog_map if pid.startswith(LUXURY_ID_PREFIXES)}
    # Placeholder uploads from unsplash seeding (Fendi catalog uses FEN-* ids).
    prefixes.update(f"FEN-{i:03d}" for i in range(1, 19))
    return sorted(prefixes)


def remove_minio_prefixes(prefixes: list[str], dry_run: bool) -> int:
    if not prefixes:
        return 0
    alias_cmd = (
        "mc alias set local http://dupli1-minio-1:9000 dupli1 dupli1_dev && "
    )
    removed = 0
    for prefix in prefixes:
        target = f"local/product-images/{prefix}/"
        cmd = alias_cmd + (f"mc rm --recursive --force {target}" if not dry_run else f"mc ls {target}")
        docker_cmd = [
            "docker",
            "run",
            "--rm",
            "--network",
            "dupli1_default",
            "--entrypoint",
            "/bin/sh",
            "minio/mc:latest",
            "-c",
            cmd,
        ]
        result = subprocess.run(docker_cmd, capture_output=True, text=True)
        if result.returncode != 0 and "does not exist" not in (result.stderr + result.stdout):
            print(f"warn  minio {prefix}: {result.stderr.strip()}", file=sys.stderr)
            continue
        if dry_run:
            lines = [ln for ln in result.stdout.splitlines() if ln.strip()]
            print(f"dry   minio/{prefix}/: {len(lines)} objects")
        else:
            print(f"rm    minio/{prefix}/")
        removed += 1
    return removed


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--email", default="admin@dupli1.com")
    parser.add_argument("--password", default="password")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--skip-minio", action="store_true")
    parser.add_argument("--skip-api", action="store_true")
    args = parser.parse_args(argv)

    catalog = load_catalog(CATALOG_PATH)
    catalog_map = catalog_by_id(catalog)

    if not args.skip_api:
        token = login(args.base_url, args.email, args.password)
        updated, unchanged = clean_api_products(
            args.base_url, token, catalog_map, args.dry_run
        )
        print(f"\napi: updated={updated} unchanged={unchanged}")

    if not args.skip_minio:
        prefixes = luxury_minio_prefixes(catalog_map)
        count = remove_minio_prefixes(prefixes, args.dry_run)
        print(f"minio: processed {count} product prefixes")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
