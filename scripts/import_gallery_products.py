#!/usr/bin/env python3
"""Import gallery scraper output (info.json per product) into the Dupli1 product API."""

from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.error
import urllib.request
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


def request_json(
    method: str,
    url: str,
    body: dict | None = None,
    token: str | None = None,
) -> tuple[int, dict | list | str]:
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"

    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read().decode("utf-8")
            if not raw:
                return resp.status, ""
            return resp.status, json.loads(raw)
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(raw) if raw else {"error": exc.reason}
        except json.JSONDecodeError:
            payload = {"error": raw or exc.reason}
        return exc.code, payload


def login(base_url: str, email: str, password: str) -> str:
    status, payload = request_json(
        "POST",
        f"{base_url}/api/v1/auth/login",
        {"email": email, "password": password},
    )
    if status != 200 or not isinstance(payload, dict) or "refresh_token" not in payload:
        raise RuntimeError(f"login failed ({status}): {payload}")

    status, payload = request_json(
        "POST",
        f"{base_url}/api/v1/auth/refresh",
        {"refresh_token": payload["refresh_token"]},
    )
    if status != 200 or not isinstance(payload, dict) or "token" not in payload:
        raise RuntimeError(f"refresh failed ({status}): {payload}")
    return payload["token"]


def parse_price(value: str | None) -> float | None:
    if not value:
        return None
    digits = re.sub(r"[^\d]", "", value)
    if not digits:
        return None
    return float(digits)


def parse_stock(info: dict) -> int:
    for entry in info.get("size_codes") or []:
        for key in ("totalAvailable", "availableQuantity"):
            qty = entry.get(key)
            if isinstance(qty, int) and qty > 0:
                return min(qty, 999)
    return 1


def parse_capacity(attrs: dict) -> str:
    height = attrs.get("Height")
    width = attrs.get("Width")
    length = attrs.get("Length")
    parts = [p for p in (height, width, length) if p]
    if not parts:
        return ""
    return " x ".join(parts) + " cm"


def parse_category(product_type: str) -> str:
    if not product_type:
        return "bags"
    if "bags" in product_type:
        return "bags"
    return product_type.rsplit("/", 1)[-1] or "bags"


def build_description(info: dict) -> str:
    parts: list[str] = []
    if info.get("description"):
        parts.append(str(info["description"]).strip())
    if info.get("short_description"):
        parts.append(str(info["short_description"]).strip())
    return "\n\n".join(parts)


def is_product_entry(info: dict) -> bool:
    if not info.get("name"):
        return False
    if parse_price(info.get("price") or info.get("price_formatted")) is None:
        return False
    if not info.get("skus"):
        return False
    return True


def gallery_product(info: dict, brand: str) -> dict:
    attrs = info.get("attributes") or {}
    return {
        "id": info["id"],
        "name": info["name"],
        "description": build_description(info),
        "price": parse_price(info.get("price") or info.get("price_formatted")),
        "brand": brand,
        "color": info.get("color") or "",
        "material": info.get("material") or "",
        "stock": parse_stock(info),
        "category": parse_category(info.get("product_type") or ""),
        "status": "active",
        "imageUrls": info.get("image_urls") or [],
        "capacity": parse_capacity(attrs),
    }


def load_existing_ids(base_url: str, token: str) -> set[str]:
    status, payload = request_json("GET", f"{base_url}/api/v1/products", token=token)
    products = payload.get("results") if isinstance(payload, dict) else payload
    if status != 200 or not isinstance(products, list):
        raise RuntimeError(f"list products failed ({status}): {payload}")
    return {item["id"] for item in products if isinstance(item, dict) and item.get("id")}


def import_dir(
    base_url: str,
    token: str,
    source_dir: Path,
    brand: str,
    dry_run: bool,
) -> tuple[int, int, int]:
    existing = load_existing_ids(base_url, token)
    created = skipped = failed = 0

    entries = sorted(source_dir.iterdir())
    for entry in entries:
        if not entry.is_dir():
            continue
        info_path = entry / "info.json"
        if not info_path.is_file():
            continue

        with info_path.open(encoding="utf-8") as handle:
            info = json.load(handle)

        if not is_product_entry(info):
            print(f"skip  {entry.name}: not a product PDP")
            skipped += 1
            continue

        product = gallery_product(info, brand)
        if product["id"] in existing:
            print(f"skip  {product['id']}: already exists")
            skipped += 1
            continue

        if dry_run:
            print(f"dry   {product['id']}: {product['name']} ({product['price']:.0f})")
            created += 1
            continue

        status, payload = request_json(
            "POST",
            f"{base_url}/api/v1/products",
            product,
            token=token,
        )
        if status == 201 and isinstance(payload, dict):
            print(f"added {payload.get('id', product['id'])}: {payload.get('name', product['name'])}")
            existing.add(payload.get("id", product["id"]))
            created += 1
            continue

        print(f"fail  {product['id']}: {status} {payload}", file=sys.stderr)
        failed += 1

    return created, skipped, failed


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source_dir", type=Path, help="Gallery output directory")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--email", default="admin@dupli1.com")
    parser.add_argument("--password", default="password")
    parser.add_argument("--brand", default="Prada")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    if not args.source_dir.is_dir():
        print(f"source directory not found: {args.source_dir}", file=sys.stderr)
        return 2

    token = login(args.base_url, args.email, args.password)
    created, skipped, failed = import_dir(
        args.base_url,
        token,
        args.source_dir,
        args.brand,
        args.dry_run,
    )
    print(f"done: created={created} skipped={skipped} failed={failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
