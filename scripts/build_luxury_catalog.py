#!/usr/bin/env python3
"""Build luxury_bags.json and luxury_bags.html from curated Korean catalog data.

The HTML file is the image source: each product has an ``<img src="...">`` tag.
Run this before ``seed_luxury_bags.py`` to refresh the JSON from HTML, or use
``--write-all`` to regenerate both files from embedded data.

Usage::

    python scripts/build_luxury_catalog.py --write-all
    python scripts/build_luxury_catalog.py --from-html
    python scripts/build_luxury_catalog.py --dry-run
"""

from __future__ import annotations

import argparse
import html
import json
import re
import sys
from pathlib import Path

CATALOG_DIR = Path(__file__).resolve().parent / "catalog"
JSON_PATH = CATALOG_DIR / "luxury_bags.json"
HTML_PATH = CATALOG_DIR / "luxury_bags.html"

# Official CDN img src URLs are populated by fetch_official_luxury_images.py.
# Do not use Unsplash placeholders here.
IMAGE_SRCS: list[str] = []

KOREAN_NAMES: dict[str, str] = {
    # Miu Miu
    "MM-001": "완더 마테라세 미니 백",
    "MM-002": "완더 소프트 레더 숄더백",
    "MM-003": "아르카디 마테라세 핸드백",
    "MM-004": "아벤투르 캔버스 토트",
    "MM-005": "보 레더 숄더백",
    "MM-006": "마드라스 레더 미니 백",
    "MM-007": "크리스탈 마테라세 클러치",
    "MM-008": "완더 호보 백",
    "MM-009": "아르카디 미니 핸드백",
    "MM-010": "레더 버킷 백",
    "MM-011": "마테라세 크로스바디",
    "MM-012": "아벤투르 미니 토트",
    "MM-013": "완더 크리스탈 백",
    "MM-014": "소프트 레더 쇼퍼",
    "MM-015": "미우 완더 플랩 백",
    "MM-016": "아르카디 미디엄 백",
    "MM-017": "새틴 이브닝 백",
    "MM-018": "완더 데님 백",
    # Gucci
    "GUC-001": "GG 마몬트 스몰 숄더백",
    "GUC-002": "재키 1961 스몰 숄더백",
    "GUC-003": "호스빗 1955 숄더백",
    "GUC-004": "디오니소스 스몰 숄더백",
    "GUC-005": "오피디아 GG 미니 백",
    "GUC-006": "뱀부 1947 스몰 탑 핸들",
    "GUC-007": "GG 마몬트 마테라세 미니 백",
    "GUC-008": "재키 1961 미디엄 호보",
    "GUC-009": "호스빗 1955 미니 백",
    "GUC-010": "디오니소스 슈퍼 미니 백",
    "GUC-011": "오피디아 미디엄 토트",
    "GUC-012": "GG 마몬트 미디엄 백",
    "GUC-013": "뱀부 미니 버킷 백",
    "GUC-014": "재키 소프트 스몰 백",
    "GUC-015": "호스빗 체인 숄더백",
    "GUC-016": "GG 블론디 숄더백",
    "GUC-017": "오피디아 스몰 숄더백",
    "GUC-018": "디오니소스 미디엄 백",
    # Bottega Veneta
    "BV-001": "카세트 크로스바디 백",
    "BV-002": "미니 조디 숄더백",
    "BV-003": "안디아모 스몰 탑 핸들",
    "BV-004": "패디드 카세트 백",
    "BV-005": "미니 루프 카메라 백",
    "BV-006": "조디 스몰 숄더백",
    "BV-007": "홉 스몰 크로스바디",
    "BV-008": "안디아모 미디엄 백",
    "BV-009": "카세트 미니 백",
    "BV-010": "틴 조디 백",
    "BV-011": "캔디 조디 백",
    "BV-012": "파우치 라지 클러치",
    "BV-013": "미니 카세트 크로스바디",
    "BV-014": "홉 미디엄 백",
    "BV-015": "안디아모 미니 백",
    "BV-016": "루프 스몰 카메라 백",
    "BV-017": "조디 미디엄 백",
    "BV-018": "카세트 체인 백",
    # Dior
    "DIO-001": "레이디 디올 마이 ABCDior 백",
    "DIO-002": "북 토트 미디엄",
    "DIO-003": "새들 백",
    "DIO-004": "디올 카로 스몰 백",
    "DIO-005": "레이디 디올 미니 백",
    "DIO-006": "북 토트 스몰",
    "DIO-007": "새들 미니 백",
    "DIO-008": "디오라마 미디엄 백",
    "DIO-009": "레이디 디올 미디엄 백",
    "DIO-010": "카로 마이크로 백",
    "DIO-011": "북 토트 라지",
    "DIO-012": "새들 숄더백",
    "DIO-013": "디올 보비 스몰 백",
    "DIO-014": "레이디 디올 라지 백",
    "DIO-015": "카로 미디엄 백",
    "DIO-016": "북 토트 미니",
    "DIO-017": "새들 벨트 백",
    "DIO-018": "디올 투줴 토트",
    # Loewe
    "LOE-001": "퍼즐 스몰 백",
    "LOE-002": "플라멩코 클러치 미니",
    "LOE-003": "해먹 컴팩트 백",
    "LOE-004": "아마조나 23 백",
    "LOE-005": "게이트 미니 백",
    "LOE-006": "퍼즐 엣지 스몰 백",
    "LOE-007": "플라멩코 퍼스",
    "LOE-008": "해먹 너겟 백",
    "LOE-009": "퍼즐 라지 백",
    "LOE-010": "아마조나 28 백",
    "LOE-011": "게이트 듀얼 미니 백",
    "LOE-012": "퍼즐 폴드 토트",
    "LOE-013": "플라멩코 미니 백",
    "LOE-014": "벌룬 스몰 백",
    "LOE-015": "퍼즐 범백",
    "LOE-016": "해먹 스몰 백",
    "LOE-017": "아마조나 미니 백",
    "LOE-018": "게이트 버킷 백",
    # Celine
    "CEL-001": "트리옴프 숄더백",
    "CEL-002": "클래식 박스 미디엄",
    "CEL-003": "러기지 나노 백",
    "CEL-004": "벨트 백 미니",
    "CEL-005": "16 스몰 백",
    "CEL-006": "트리옴프 캔버스 백",
    "CEL-007": "클래식 박스 스몰",
    "CEL-008": "러기지 미니 백",
    "CEL-009": "벨트 백 미디엄",
    "CEL-010": "16 미디엄 백",
    "CEL-011": "트리옴프 틴 백",
    "CEL-012": "아바 백",
    "CEL-013": "러기지 마이크로 백",
    "CEL-014": "클래식 박스 틴",
    "CEL-015": "트리옴프 퀴르 백",
    "CEL-016": "벨트 백 퀴르",
    "CEL-017": "16 나노 백",
    "CEL-018": "카바스 팬텀 백",
    # Saint Laurent
    "YSL-001": "루 카메라 백",
    "YSL-002": "케이트 미디엄 숄더백",
    "YSL-003": "룰루 스몰 백",
    "YSL-004": "삭 드 주르 나노",
    "YSL-005": "니키 미디엄 백",
    "YSL-006": "루 미니 백",
    "YSL-007": "케이트 스몰 백",
    "YSL-008": "룰루 퍼퍼 미니",
    "YSL-009": "삭 드 주르 스몰",
    "YSL-010": "니키 미니 백",
    "YSL-011": "솔페리노 스몰 백",
    "YSL-012": "케이트 태슬 백",
    "YSL-013": "루 카메라 퀼팅",
    "YSL-014": "룰루 미디엄 백",
    "YSL-015": "삭 드 주르 베이비",
    "YSL-016": "니키 쇼핑 백",
    "YSL-017": "카상드르 엔벨로프 클러치",
    "YSL-018": "케이트 99 백",
}

TARGET_BRANDS = [
    "miu-miu",
    "gucci",
    "bottega-veneta",
    "dior",
    "loewe",
    "celine",
    "saint-laurent",
]


def _configure_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass


_configure_stdio()


def image_src_for(product_id: str) -> str:
    seed = sum(ord(c) for c in product_id)
    return IMAGE_SRCS[seed % len(IMAGE_SRCS)]


def load_json_catalog(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        data = json.load(handle)
    brands = data.get("brands")
    if not isinstance(brands, dict):
        raise ValueError("catalog missing brands object")
    return brands


def enrich_product(item: dict) -> dict:
    product_id = item["id"]
    enriched = dict(item)
    enriched["name"] = KOREAN_NAMES.get(product_id, item.get("name", product_id))
    src = image_src_for(product_id)
    enriched["imageUrls"] = [src]
    return enriched


def build_json(brands: dict) -> dict:
    out: dict = {"brands": {}}
    for key in TARGET_BRANDS:
        entry = brands.get(key)
        if not entry:
            continue
        products = [enrich_product(p) for p in entry.get("products") or []]
        out["brands"][key] = {"name": entry["name"], "products": products}
    return out


def render_html(catalog: dict) -> str:
    lines = [
        "<!DOCTYPE html>",
        '<html lang="ko">',
        "<head><meta charset=\"utf-8\"><title>Luxury Bags Catalog</title></head>",
        "<body>",
    ]
    for key in TARGET_BRANDS:
        entry = catalog["brands"].get(key)
        if not entry:
            continue
        brand_name = entry["name"]
        lines.append(f'<section data-brand="{html.escape(key)}">')
        lines.append(f"  <h1>{html.escape(brand_name)}</h1>")
        for item in entry["products"]:
            product_id = item["id"]
            name = item["name"]
            imgs = item.get("imageUrls") or [""]
            desc = html.escape(item.get("description") or "")
            lines.append(
                f'  <article class="product" data-id="{html.escape(product_id)}" '
                f'data-brand="{html.escape(key)}">'
            )
            lines.append(f"    <h2>{html.escape(name)}</h2>")
            for src in imgs:
                if not src:
                    continue
                lines.append(
                    f'    <img src="{html.escape(src)}" alt="{html.escape(name)}">'
                )
            lines.append(f"    <p>{desc}</p>")
            lines.append("  </article>")
        lines.append("</section>")
    lines.extend(["</body>", "</html>"])
    return "\n".join(lines) + "\n"


def parse_html(path: Path) -> dict[str, dict]:
    text = path.read_text(encoding="utf-8")
    articles = re.findall(
        r'<article[^>]*class="product"[^>]*data-id="([^"]+)"[^>]*data-brand="([^"]+)"[^>]*>'
        r"(.*?)</article>",
        text,
        flags=re.S,
    )
    parsed: dict[str, dict] = {}
    for product_id, brand_key, body in articles:
        name_match = re.search(r"<h2[^>]*>([^<]+)</h2>", body)
        img_match = re.search(r'<img[^>]+src="([^"]+)"', body)
        desc_match = re.search(r"<p[^>]*>([^<]*)</p>", body)
        if not name_match or not img_match:
            continue
        parsed[product_id] = {
            "brand_key": brand_key,
            "name": html.unescape(name_match.group(1).strip()),
            "imageUrls": [html.unescape(img_match.group(1).strip())],
            "description": html.unescape(desc_match.group(1).strip()) if desc_match else "",
        }
    return parsed


def merge_html_into_json(brands: dict, html_products: dict[str, dict]) -> dict:
    out: dict = {"brands": {}}
    for key in TARGET_BRANDS:
        entry = brands.get(key)
        if not entry:
            continue
        merged_products = []
        for item in entry.get("products") or []:
            merged = dict(item)
            html_item = html_products.get(item["id"])
            if html_item:
                merged["name"] = html_item["name"]
                merged["imageUrls"] = html_item["imageUrls"]
                if html_item.get("description"):
                    merged["description"] = html_item["description"]
            else:
                merged = enrich_product(merged)
            merged_products.append(merged)
        out["brands"][key] = {"name": entry["name"], "products": merged_products}
    return out


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write-all", action="store_true", help="Regenerate JSON and HTML")
    parser.add_argument("--from-html", action="store_true", help="Parse HTML img src into JSON")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    if not JSON_PATH.is_file():
        print(f"catalog not found: {JSON_PATH}", file=sys.stderr)
        return 2

    brands = load_json_catalog(JSON_PATH)

    if args.from_html:
        if not HTML_PATH.is_file():
            print(f"html not found: {HTML_PATH}", file=sys.stderr)
            return 2
        html_products = parse_html(HTML_PATH)
        catalog = merge_html_into_json(brands, html_products)
        print(f"parsed {len(html_products)} products from HTML img src")
    else:
        catalog = build_json(brands)

    product_count = sum(
        len(entry.get("products") or []) for entry in catalog["brands"].values()
    )
    print(f"brands={len(catalog['brands'])} products={product_count}")

    if args.dry_run:
        for key, entry in catalog["brands"].items():
            sample = (entry.get("products") or [None])[0]
            if sample:
                print(f"  {key}: {sample['name']} -> {(sample.get('imageUrls') or [''])[0][:60]}")
        return 0

    if args.write_all or not args.from_html:
        HTML_PATH.write_text(render_html(catalog), encoding="utf-8")
        print(f"wrote {HTML_PATH}")

    JSON_PATH.write_text(
        json.dumps(catalog, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {JSON_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
