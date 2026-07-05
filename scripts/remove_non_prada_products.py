#!/usr/bin/env python3
"""Delete all products whose brand is not Prada.

Usage::

    python scripts/remove_non_prada_products.py
    python scripts/remove_non_prada_products.py --dry-run
    python scripts/remove_non_prada_products.py --keep-brand Prada
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from import_gallery_products import login, request_json  # noqa: E402


def _configure_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass


_configure_stdio()


def list_products(base_url: str, token: str) -> list[dict]:
    status, payload = request_json("GET", f"{base_url}/api/v1/products", token=token)
    products = payload.get("results") if isinstance(payload, dict) else payload
    if status != 200 or not isinstance(products, list):
        raise RuntimeError(f"list products failed ({status}): {payload}")
    return [p for p in products if isinstance(p, dict)]


def delete_product(base_url: str, token: str, product_id: str) -> None:
    status, payload = request_json(
        "DELETE",
        f"{base_url}/api/v1/products/{product_id}",
        token=token,
    )
    if status not in (200, 204):
        raise RuntimeError(f"delete failed ({status}): {payload}")


def is_kept_brand(brand: str, keep_brand: str) -> bool:
    return (brand or "").strip().lower() == keep_brand.strip().lower()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--email", default="admin@dupli1.com")
    parser.add_argument("--password", default="password")
    parser.add_argument(
        "--keep-brand",
        default="Prada",
        help="Brand to keep (case-insensitive). All others are deleted.",
    )
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    token = login(args.base_url, args.email, args.password)
    products = list_products(args.base_url, token)

    keep = [p for p in products if is_kept_brand(p.get("brand") or "", args.keep_brand)]
    remove = [p for p in products if not is_kept_brand(p.get("brand") or "", args.keep_brand)]

    print(f"total={len(products)} keep={len(keep)} remove={len(remove)}")
    for p in keep:
        print(f"keep  {p.get('id', '?')} ({p.get('brand', '')}): {p.get('name', '')}")

    deleted = failed = 0
    for p in remove:
        product_id = p.get("id", "?")
        brand = p.get("brand", "")
        name = p.get("name", "")
        if args.dry_run:
            print(f"dry   {product_id} ({brand}): {name}")
            deleted += 1
            continue
        try:
            delete_product(args.base_url, token, product_id)
            print(f"del   {product_id} ({brand}): {name}")
            deleted += 1
        except Exception as exc:
            print(f"fail  {product_id} ({brand}): {exc}", file=sys.stderr)
            failed += 1

    print(f"\ndone: deleted={deleted} kept={len(keep)} failed={failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
