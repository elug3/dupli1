#!/usr/bin/env python3
"""Upload product images into MinIO via POST /api/v1/products/{id}/images.

For each product:
  - If it has remote image URLs (e.g. Prada CDN), download and upload them.
  - If it has no images, upload a simple brand-colored placeholder PNG.
  - Replace stored URLs with MinIO URLs reachable from the host (localhost:9000).

Usage::

    python scripts/upload_product_images.py
    python scripts/upload_product_images.py --brand Prada
    python scripts/upload_product_images.py --max-images 3
    python scripts/upload_product_images.py --dry-run
"""

from __future__ import annotations

import argparse
import io
import json
import struct
import sys
import urllib.error
import urllib.request
import uuid
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from import_gallery_products import login, request_json  # noqa: E402

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

# Product service stores MinIO URLs with the Docker hostname; rewrite for browsers.
MINIO_INTERNAL_PREFIX = "http://minio:9000/"
MINIO_PUBLIC_PREFIX = "http://localhost:9000/"

BRAND_COLORS: dict[str, tuple[int, int, int]] = {
    "Prada": (0x1A, 0x1A, 0x1A),
    "Miu Miu": (0xC4, 0x5C, 0x6A),
    "Gucci": (0x0C, 0x4A, 0x1E),
    "Bottega Veneta": (0x5C, 0x40, 0x33),
    "Dior": (0x1C, 0x1C, 0x2E),
    "Loewe": (0x8B, 0x5A, 0x2B),
    "Celine": (0x2F, 0x2F, 0x2F),
    "Saint Laurent": (0x11, 0x11, 0x11),
    "Fendi": (0xB8, 0x86, 0x0B),
}


def _configure_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass


_configure_stdio()


def is_minio_url(url: str) -> bool:
    return url.startswith(MINIO_INTERNAL_PREFIX) or url.startswith(MINIO_PUBLIC_PREFIX)


def public_minio_url(url: str) -> str:
    if url.startswith(MINIO_INTERNAL_PREFIX):
        return MINIO_PUBLIC_PREFIX + url[len(MINIO_INTERNAL_PREFIX) :]
    return url


def download_bytes(url: str) -> tuple[bytes, str]:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=60) as resp:
        data = resp.read()
        content_type = resp.headers.get("Content-Type", "application/octet-stream")
        if ";" in content_type:
            content_type = content_type.split(";", 1)[0].strip()
        return data, content_type


def _png_chunk(tag: bytes, data: bytes) -> bytes:
    return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)


def make_placeholder_png(brand: str, product_id: str, size: int = 800) -> bytes:
    """Solid-color PNG placeholder (no third-party deps)."""
    r, g, b = BRAND_COLORS.get(brand, (0x44, 0x44, 0x55))
    # Subtle vertical gradient using product_id hash so images differ.
    seed = sum(ord(c) for c in product_id) % 40
    rows = []
    for y in range(size):
        shade = seed + (y * 20) // size
        rr = min(255, r + shade)
        gg = min(255, g + shade // 2)
        bb = min(255, b + shade // 3)
        # Filter byte 0 (None) + RGB pixels.
        rows.append(b"\x00" + bytes([rr, gg, bb]) * size)
    raw = b"".join(rows)
    ihdr = struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0)
    return (
        b"\x89PNG\r\n\x1a\n"
        + _png_chunk(b"IHDR", ihdr)
        + _png_chunk(b"IDAT", zlib.compress(raw, 9))
        + _png_chunk(b"IEND", b"")
    )


def upload_image(
    base_url: str,
    token: str,
    product_id: str,
    data: bytes,
    filename: str,
    content_type: str,
) -> dict:
    boundary = f"----dupli1{uuid.uuid4().hex}"
    body = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="image"; filename="{filename}"\r\n'
        f"Content-Type: {content_type}\r\n\r\n"
    ).encode("utf-8") + data + f"\r\n--{boundary}--\r\n".encode("utf-8")

    req = urllib.request.Request(
        f"{base_url}/api/v1/products/{product_id}/images",
        data=body,
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"upload failed ({exc.code}): {raw}") from exc


def update_product(base_url: str, token: str, product: dict) -> dict:
    body = {
        "id": product["id"],
        "name": product.get("name") or "",
        "description": product.get("description") or "",
        "price": product.get("price") or 0,
        "cost": product.get("cost") or 0,
        "brand": product.get("brand") or "",
        "color": product.get("color") or "",
        "material": product.get("material") or "",
        "stock": product.get("stock") or 0,
        "category": product.get("category") or "bags",
        "status": product.get("status") or "active",
        "imageUrls": product.get("imageUrls") or [],
        "capacity": product.get("capacity") or "",
        "tags": product.get("tags") or [],
    }
    status, payload = request_json(
        "PUT",
        f"{base_url}/api/v1/products/{product['id']}",
        body,
        token=token,
    )
    if status != 200 or not isinstance(payload, dict):
        raise RuntimeError(f"update failed ({status}): {payload}")
    return payload


def clear_image_urls(base_url: str, token: str, product: dict) -> dict:
    product = dict(product)
    product["imageUrls"] = []
    return update_product(base_url, token, product)


def product_sources(product: dict, max_images: int) -> list[tuple[str, bytes, str]]:
    """Return (filename, bytes, content_type) sources to upload."""
    sources: list[tuple[str, bytes, str]] = []
    remote = [u for u in (product.get("imageUrls") or []) if u and not is_minio_url(u)]
    for index, url in enumerate(remote[:max_images], start=1):
        try:
            data, content_type = download_bytes(url)
        except Exception as exc:
            print(f"  warn download failed {url}: {exc}", file=sys.stderr)
            continue
        ext = "jpg"
        if "png" in content_type:
            ext = "png"
        elif "webp" in content_type:
            ext = "webp"
        sources.append((f"{product['id']}_{index}.{ext}", data, content_type or "image/jpeg"))

    if sources:
        return sources

    # No remote images (or all downloads failed): one placeholder.
    png = make_placeholder_png(product.get("brand") or "", product["id"])
    return [(f"{product['id']}_1.png", png, "image/png")]


def needs_upload(product: dict) -> bool:
    urls = product.get("imageUrls") or []
    if not urls:
        return True
    # Re-upload if any non-MinIO URL remains, or only internal hostnames.
    if any(not is_minio_url(u) for u in urls):
        return True
    if any(u.startswith(MINIO_INTERNAL_PREFIX) for u in urls):
        return True
    return False


def process_product(
    base_url: str,
    token: str,
    product: dict,
    max_images: int,
    dry_run: bool,
) -> str:
    product_id = product["id"]
    urls = product.get("imageUrls") or []

    # Already fully on public MinIO — nothing to do.
    if urls and all(u.startswith(MINIO_PUBLIC_PREFIX) for u in urls):
        return "skip"

    # Only internal MinIO hostnames — rewrite to localhost.
    if urls and all(is_minio_url(u) for u in urls):
        if dry_run:
            return "dry-rewrite"
        rewritten = [public_minio_url(u) for u in urls]
        product = dict(product)
        product["imageUrls"] = rewritten
        update_product(base_url, token, product)
        return "rewrote"

    # Download/generate all sources before mutating the product.
    sources = product_sources(product, max_images)
    if dry_run:
        return f"dry-{len(sources)}"
    if not sources:
        raise RuntimeError("no image sources")

    # Clear remote/CDN URLs so uploads become the only imageUrls.
    clear_image_urls(base_url, token, product)

    uploaded: list[str] = []
    try:
        for filename, data, content_type in sources:
            updated = upload_image(base_url, token, product_id, data, filename, content_type)
            urls = updated.get("imageUrls") or []
            if urls:
                uploaded.append(public_minio_url(urls[-1]))
    except Exception:
        # Restore original remote URLs if upload fails mid-way.
        if not uploaded:
            restore = dict(product)
            update_product(base_url, token, restore)
        raise

    if not uploaded:
        restore = dict(product)
        update_product(base_url, token, restore)
        raise RuntimeError("no images uploaded")

    # Persist public localhost URLs (service returns minio:9000).
    final = dict(product)
    final["imageUrls"] = uploaded
    update_product(base_url, token, final)
    return f"ok-{len(uploaded)}"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--email", default="admin@dupli1.com")
    parser.add_argument("--password", default="password")
    parser.add_argument("--brand", default="", help="Only process this brand")
    parser.add_argument("--max-images", type=int, default=6, help="Max images per product")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    token = login(args.base_url, args.email, args.password)
    status, payload = request_json("GET", f"{args.base_url}/api/v1/products", token=token)
    products = payload.get("results") if isinstance(payload, dict) else payload
    if status != 200 or not isinstance(products, list):
        print(f"list products failed ({status}): {payload}", file=sys.stderr)
        return 1

    if args.brand:
        products = [p for p in products if (p.get("brand") or "").lower() == args.brand.lower()]

    ok = skipped = failed = 0
    for product in products:
        product_id = product.get("id", "?")
        brand = product.get("brand", "")
        try:
            if not needs_upload(product) and all(
                (u or "").startswith(MINIO_PUBLIC_PREFIX) for u in (product.get("imageUrls") or [])
            ):
                print(f"skip  {product_id} ({brand}): already on MinIO")
                skipped += 1
                continue
            result = process_product(
                args.base_url, token, product, args.max_images, args.dry_run
            )
            if result == "skip":
                print(f"skip  {product_id} ({brand}): already on MinIO")
                skipped += 1
            elif result.startswith("dry"):
                print(f"dry   {product_id} ({brand}): {result}")
                ok += 1
            else:
                print(f"upld  {product_id} ({brand}): {result}")
                ok += 1
        except Exception as exc:
            print(f"fail  {product_id} ({brand}): {exc}", file=sys.stderr)
            failed += 1

    print(f"\ndone: uploaded={ok} skipped={skipped} failed={failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
