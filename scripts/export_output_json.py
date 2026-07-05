#!/usr/bin/env python3
"""Export scripts/catalog/luxury_bags.json to scripts/catalog/output.json."""

from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path

CATALOG_DIR = Path(__file__).resolve().parent / "catalog"
CATALOG_PATH = CATALOG_DIR / "luxury_bags.json"
URLS_PATH = CATALOG_DIR / "official_source_urls.json"
OUTPUT_PATH = CATALOG_DIR / "output.json"


def main() -> None:
    with CATALOG_PATH.open(encoding="utf-8") as handle:
        catalog = json.load(handle)
    source_urls: dict[str, str] = {}
    if URLS_PATH.is_file():
        with URLS_PATH.open(encoding="utf-8") as handle:
            data = json.load(handle)
        source_urls = data.get("products") or {}

    products: list[dict] = []
    for brand_key, entry in catalog.get("brands", {}).items():
        brand_name = entry["name"]
        for item in entry.get("products") or []:
            image_urls = item.get("imageUrls") or []
            products.append(
                {
                    "id": item["id"],
                    "brandKey": brand_key,
                    "brand": brand_name,
                    "name": item.get("name"),
                    "description": item.get("description"),
                    "price": item.get("price"),
                    "color": item.get("color"),
                    "material": item.get("material"),
                    "capacity": item.get("capacity"),
                    "sourceUrl": item.get("sourceUrl") or source_urls.get(item["id"], ""),
                    "imageUrls": image_urls,
                    "imageCount": len(image_urls),
                }
            )

    output = {
        "generatedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "productCount": len(products),
        "brands": len(catalog.get("brands", {})),
        "products": products,
    }
    OUTPUT_PATH.write_text(
        json.dumps(output, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {OUTPUT_PATH} ({len(products)} products)")


if __name__ == "__main__":
    main()
