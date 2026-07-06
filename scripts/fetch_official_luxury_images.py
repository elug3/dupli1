#!/usr/bin/env python3
"""Fetch 8-9 official product images from brand sites into luxury_bags catalog.

Reads ``scripts/catalog/official_source_urls.json`` (product id -> PDP URL),
scrapes ``<img src>`` / attachment URLs from each official page, and writes
``imageUrls`` (up to 9) into ``luxury_bags.json`` + ``luxury_bags.html``.

Usage::

    python scripts/fetch_official_luxury_images.py
    python scripts/fetch_official_luxury_images.py --brand gucci --dry-run
    python scripts/fetch_official_luxury_images.py --update-api
"""

from __future__ import annotations

import argparse
import html
import json
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

CATALOG_DIR = Path(__file__).resolve().parent / "catalog"
JSON_PATH = CATALOG_DIR / "luxury_bags.json"
HTML_PATH = CATALOG_DIR / "luxury_bags.html"
URLS_PATH = CATALOG_DIR / "official_source_urls.json"
CACHE_PATH = CATALOG_DIR / "official_image_cache.json"

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

MAX_IMAGES = 9
MIN_IMAGES = 8

AEM_IMAGE_RE = re.compile(
    r"https://www\.(?:prada|miumiu)\.com/content/dam/"
    r"(?:prada|miumiu)bkg_products/[^\s\"'<>\\,]+?\.jpg",
    re.IGNORECASE,
)
GUCCI_IMAGE_RE = re.compile(
    r"https://media\.gucci\.com/style/[^\"'\s<>]+?\.jpg",
    re.IGNORECASE,
)
DIOR_CATALOG_RE = re.compile(
    r"https://www\.dior\.com/couture/ecommerce/media/catalog/product/"
    r"[^\"'\s<>]+?\.jpg(?:\?[^\"'\s<>]*)?",
    re.IGNORECASE,
)
DIOR_ASSETS_RE = re.compile(
    r"https://assets\.christiandior\.com/is/image/diorprod/[^\"'\s<>]+",
    re.IGNORECASE,
)
LOEWE_IMAGE_RE = re.compile(
    r"https://[^\"'\s<>]*loewe\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)(?:\?[^\"'\s<>]*)?",
    re.IGNORECASE,
)
CELINE_IMAGE_RE = re.compile(
    r"https://[^\"'\s<>]*celine\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)(?:\?[^\"'\s<>]*)?",
    re.IGNORECASE,
)
YSL_IMAGE_RE = re.compile(
    r"https://[^\"'\s<>]*ysl\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)(?:\?[^\"'\s<>]*)?",
    re.IGNORECASE,
)
BV_IMAGE_RE = re.compile(
    r"https://[^\"'\s<>]*bottegaveneta\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)(?:\?[^\"'\s<>]*)?",
    re.IGNORECASE,
)

BRANDS_CACHE_FIRST = {
    "gucci",
    "dior",
    "bottega-veneta",
    "loewe",
    "celine",
    "saint-laurent",
}

BRAND_EXTRACTORS: dict[str, list[re.Pattern[str]]] = {
    "miu-miu": [AEM_IMAGE_RE],
    "gucci": [GUCCI_IMAGE_RE],
    "bottega-veneta": [BV_IMAGE_RE],
    "dior": [DIOR_CATALOG_RE, DIOR_ASSETS_RE],
    "loewe": [LOEWE_IMAGE_RE],
    "celine": [CELINE_IMAGE_RE],
    "saint-laurent": [YSL_IMAGE_RE],
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


def fetch_html(url: str, timeout: int = 90, retries: int = 3) -> str:
    last_error: Exception | None = None
    for attempt in range(retries):
        try:
            req = urllib.request.Request(
                url,
                headers={
                    "User-Agent": USER_AGENT,
                    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
                    "Accept-Language": "en-US,en;q=0.9,ko-KR,ko;q=0.8",
                },
            )
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return resp.read().decode("utf-8", errors="replace")
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ConnectionResetError) as exc:
            last_error = exc
            time.sleep(1.5 * (attempt + 1))
    raise last_error  # type: ignore[misc]


def fetch_images_for_product(
    brand_key: str,
    source_url: str,
    fallback_url: str = "",
) -> tuple[list[str], str]:
    for candidate in [source_url, fallback_url]:
        if not candidate:
            continue
        html_text = fetch_html(candidate)
        code_hint = product_code_from_url(candidate)
        images = extract_images(brand_key, html_text, code_hint)
        if len(images) >= MIN_IMAGES:
            return images, candidate
    if source_url:
        html_text = fetch_html(source_url)
        images = extract_images(brand_key, html_text, product_code_from_url(source_url))
        if images:
            return images, source_url
    return [], source_url


def extract_attachment_paths(html_text: str) -> list[str]:
    paths: list[str] = []
    for match in re.finditer(r'"attachmentAssetPath"\s*:\s*"([^"]+)"', html_text):
        url = match.group(1).replace("\\/", "/")
        if url.endswith(".jpg") and "bkg_products" in url:
            paths.append(url)
    return paths


def normalize_url(url: str) -> str:
    url = html.unescape(url.strip())
    if "?" in url and "dior.com/couture" in url:
        base, query = url.split("?", 1)
        if "imwidth=" not in query:
            url = f"{base}?imwidth=3000"
    return url


def is_noise(url: str) -> bool:
    lower = url.lower()
    noise = (
        "logo",
        "icon",
        "menu",
        "homepage",
        "navigation",
        "footer",
        "personalization",
        "look_f_",
        "promocomponent",
        "thematic_",
    )
    return any(token in lower for token in noise)


def unique_urls(urls: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for url in urls:
        url = normalize_url(url)
        if not url or url in seen or is_noise(url):
            continue
        seen.add(url)
        out.append(url)
    return out


def extract_images(brand_key: str, html_text: str, product_code_hint: str = "") -> list[str]:
    urls: list[str] = []
    urls.extend(extract_attachment_paths(html_text))

    for pattern in BRAND_EXTRACTORS.get(brand_key, []):
        urls.extend(pattern.findall(html_text))

    urls = unique_urls(urls)

    if product_code_hint and brand_key == "dior":
        code = product_code_hint.split("-")[0].replace("_", "").upper()
        filtered = [u for u in urls if code[:10] in u.replace("_", "").replace("-", "").upper()]
        if len(filtered) >= MIN_IMAGES:
            urls = filtered

    if brand_key == "gucci":
        # Prefer full gallery shots over tiny thumbs.
        urls.sort(key=lambda u: ("235x235" in u, "490x490" in u))

    return urls[:MAX_IMAGES]


def product_code_from_url(url: str) -> str:
    path = urllib.parse.urlparse(url).path.rstrip("/")
    tail = path.rsplit("/", 1)[-1]
    if "-p-" in tail:
        return tail.split("-p-", 1)[-1]
    return tail


def load_image_cache() -> dict[str, dict[str, list[str]]]:
    if not CACHE_PATH.is_file():
        return {}
    with CACHE_PATH.open(encoding="utf-8") as handle:
        return json.load(handle)


def cache_images_for_product(
    brand_key: str,
    description: str,
    cache: dict[str, dict[str, list[str]]],
) -> list[str]:
    brand_cache = cache.get(brand_key) or {}
    desc = description.lower()
    for keyword, urls in brand_cache.items():
        if keyword != "default" and keyword.lower() in desc:
            return urls[:MAX_IMAGES]
    default = brand_cache.get("default") or []
    return default[:MAX_IMAGES]


def load_catalog() -> dict:
    with JSON_PATH.open(encoding="utf-8") as handle:
        return json.load(handle)


def load_source_urls() -> dict[str, str]:
    if not URLS_PATH.is_file():
        raise FileNotFoundError(f"missing source URL map: {URLS_PATH}")
    with URLS_PATH.open(encoding="utf-8") as handle:
        data = json.load(handle)
    urls = data.get("products") or data
    if not isinstance(urls, dict):
        raise ValueError("official_source_urls.json must contain a products object")
    return urls


def render_html(catalog: dict) -> str:
    lines = [
        "<!DOCTYPE html>",
        '<html lang="ko">',
        "<head><meta charset=\"utf-8\"><title>Luxury Bags Catalog</title></head>",
        "<body>",
    ]
    for key, entry in catalog.get("brands", {}).items():
        lines.append(f'<section data-brand="{html.escape(key)}">')
        lines.append(f"  <h1>{html.escape(entry['name'])}</h1>")
        for item in entry.get("products") or []:
            imgs = item.get("imageUrls") or []
            lines.append(
                f'  <article class="product" data-id="{html.escape(item["id"])}" '
                f'data-brand="{html.escape(key)}">'
            )
            lines.append(f"    <h2>{html.escape(item['name'])}</h2>")
            for src in imgs:
                lines.append(
                    f'    <img src="{html.escape(src)}" alt="{html.escape(item["name"])}">'
                )
            lines.append(f"    <p>{html.escape(item.get('description') or '')}</p>")
            lines.append("  </article>")
        lines.append("</section>")
    lines.extend(["</body>", "</html>"])
    return "\n".join(lines) + "\n"


def update_api(products: list[dict], base_url: str, email: str, password: str) -> None:
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from import_gallery_products import login, request_json  # noqa: E402
    from seed_luxury_bags import to_api_product, update_product  # noqa: E402

    token = login(base_url, email, password)
    ok = fail = 0
    for item in products:
        body = to_api_product(item, item["_brand_name"])
        status, payload = update_product(base_url, token, body)
        if status == 200:
            print(f"upd   {item['id']}: {len(item.get('imageUrls') or [])} images")
            ok += 1
        else:
            print(f"fail  {item['id']}: {status} {payload}", file=sys.stderr)
            fail += 1
    print(f"api: updated={ok} failed={fail}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--brand", default="", help="Only process one brand key")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument(
        "--use-cache",
        action="store_true",
        help="Use curated official CDN URLs when live scrape fails",
    )
    parser.add_argument("--update-api", action="store_true")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--email", default="admin@dupli1.com")
    parser.add_argument("--password", default="password")
    args = parser.parse_args(argv)

    from build_official_source_urls import BRAND_FALLBACK_URL  # noqa: E402
    catalog = load_catalog()
    source_urls = load_source_urls()
    fallbacks = BRAND_FALLBACK_URL
    image_cache = load_image_cache() if args.use_cache or CACHE_PATH.is_file() else {}
    updated_items: list[dict] = []
    fetched = skipped = failed = 0

    for brand_key, entry in catalog.get("brands", {}).items():
        if args.brand and brand_key != args.brand:
            continue
        brand_name = entry["name"]
        print(f"\n=== {brand_name} ===")
        for item in entry.get("products") or []:
            product_id = item["id"]
            source_url = source_urls.get(product_id)
            if not source_url:
                print(f"skip  {product_id}: no sourceUrl")
                skipped += 1
                continue
            try:
                images: list[str] = []
                used_url = source_url
                if (
                    args.use_cache
                    and brand_key in BRANDS_CACHE_FIRST
                    and brand_key in image_cache
                ):
                    images = cache_images_for_product(
                        brand_key,
                        item.get("description") or "",
                        image_cache,
                    )
                    used_url = source_url + " (cache)"
                else:
                    images, used_url = fetch_images_for_product(
                        brand_key,
                        source_url,
                        fallbacks.get(brand_key, ""),
                    )
                if len(images) < MIN_IMAGES and args.use_cache:
                    cached = cache_images_for_product(
                        brand_key,
                        item.get("description") or "",
                        image_cache,
                    )
                    if len(cached) >= len(images):
                        images = cached
                        used_url = source_url + " (cache)"
                if len(images) < MIN_IMAGES:
                    print(
                        f"warn  {product_id}: only {len(images)} official images",
                        file=sys.stderr,
                    )
                if not images:
                    print(f"fail  {product_id}: no images extracted", file=sys.stderr)
                    failed += 1
                    continue
                item = dict(item)
                item["imageUrls"] = images
                item["sourceUrl"] = used_url
                item["_brand_name"] = brand_name
                updated_items.append(item)
                print(f"ok    {product_id}: {len(images)} images")
                fetched += 1
                time.sleep(0.5)
            except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, ConnectionResetError) as exc:
                if args.use_cache:
                    cached = cache_images_for_product(
                        brand_key,
                        item.get("description") or "",
                        image_cache,
                    )
                    if cached:
                        item = dict(item)
                        item["imageUrls"] = cached
                        item["sourceUrl"] = source_url + " (cache)"
                        item["_brand_name"] = brand_name
                        updated_items.append(item)
                        print(f"ok    {product_id}: {len(cached)} images (cache after {exc.__class__.__name__})")
                        fetched += 1
                        continue
                print(f"fail  {product_id}: {exc}", file=sys.stderr)
                failed += 1

    print(f"\ndone: fetched={fetched} skipped={skipped} failed={failed}")

    if args.dry_run or not updated_items:
        return 1 if failed else 0

    # Merge back into catalog.
    by_id = {item["id"]: item for item in updated_items}
    for brand_key, entry in catalog.get("brands", {}).items():
        products = []
        for item in entry.get("products") or []:
            merged = dict(by_id.get(item["id"], item))
            merged.pop("_brand_name", None)
            products.append(merged)
        entry["products"] = products

    JSON_PATH.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    HTML_PATH.write_text(render_html(catalog), encoding="utf-8")
    print(f"wrote {JSON_PATH}")
    print(f"wrote {HTML_PATH}")

    if args.update_api:
        update_api(updated_items, args.base_url, args.email, args.password)

    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
