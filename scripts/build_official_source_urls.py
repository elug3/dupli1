#!/usr/bin/env python3
"""Generate scripts/catalog/official_source_urls.json from model keyword map."""

from __future__ import annotations

import json
from pathlib import Path

OUT = Path(__file__).resolve().parent / "catalog" / "official_source_urls.json"
CATALOG = Path(__file__).resolve().parent / "catalog" / "luxury_bags.json"

# Official PDP URLs (en/eu locales scrape reliably).
MODEL_URLS: dict[str, list[tuple[str, str]]] = {
    "miu-miu": [
        ("Wander", "https://www.miumiu.com/eu/en/p/wander-matelasse-nappa-leather-hobo-mini-bag/5BP078_AN88_F0002_V_OOO"),
        ("Arcadie", "https://www.miumiu.com/eu/en/p/arcadie-matelasse-nappa-leather-bag/5BB142_AN88_F0002_V_OON"),
        ("Aventure", "https://www.miumiu.com/eu/en/p/aventure-nappa-leather-tote-bag/5BC088_2HCP_F0637_V_OOO"),
        ("Beau", "https://www.miumiu.com/eu/en/p/beau-nappa-leather-shoulder-bag/5BH039_2AIX_F0002_V_OOO"),
        ("Madras", "https://www.miumiu.com/eu/en/p/madras-woven-nappa-leather-mini-bag/5BA067_2DXY_F0002_V_OOO"),
        ("Crystal", "https://www.miumiu.com/eu/en/p/wander-matelasse-crystal-embellished-nappa-leather-mini-bag/5BP078_2HCP_F0002_V_OOO"),
        ("bucket", "https://www.miumiu.com/eu/en/p/nappa-leather-bucket-bag/5BC088_2HCP_F0002_V_OOO"),
        ("crossbody", "https://www.miumiu.com/eu/en/p/matelasse-nappa-leather-crossbody-bag/5BP078_2HCP_F0002_V_OOO"),
        ("shopper", "https://www.miumiu.com/eu/en/p/soft-nappa-leather-shopper/5BC088_2HCP_F0637_V_OOO"),
        ("Satin", "https://www.miumiu.com/ww/en/p/wander-matelasse-silk-mini-bag/5BP078_2CV8_F0002_V_OLO"),
        ("denim", "https://www.miumiu.com/eu/en/p/wander-denim-and-nappa-leather-mini-bag/5BP078_2DXY_F0002_V_OOO"),
    ],
    "gucci": [
        ("Marmont", "https://www.gucci.com/sg/en_gb/pr/women/handbags/crossbody-bags-for-women/gg-marmont-small-shoulder-bag-p-443497DRW1T1000"),
        ("Jackie", "https://www.gucci.com/sg/en_gb/pr/women/handbags/shoulder-bags-for-women/jackie-1961-small-shoulder-bag-p-636709AAFB01000"),
        ("Horsebit", "https://www.gucci.com/sg/en_gb/pr/women/handbags/shoulder-bags-for-women/horsebit-1955-shoulder-bag-p-602204AAUEN9778"),
        ("Dionysus", "https://www.gucci.com/sg/en_gb/pr/women/handbags/shoulder-bags-for-women/dionysus-small-shoulder-bag-p-400249AUE0T1000"),
        ("Ophidia", "https://www.gucci.com/sg/en_gb/pr/women/handbags/shoulder-bags-for-women/ophidia-mini-bag-p-625772AAE0T1000"),
        ("Bamboo", "https://www.gucci.com/sg/en_gb/pr/women/handbags/top-handle-bags-for-women/bamboo-1947-small-top-handle-bag-p-702732AAU0T1000"),
        ("Blondie", "https://www.gucci.com/sg/en_gb/pr/women/handbags/shoulder-bags-for-women/gg-blondie-small-shoulder-bag-p-735178AAU0T1000"),
    ],
    "bottega-veneta": [
        ("Cassette", "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html"),
        ("Jodie", "https://www.bottegaveneta.com/en-us/mini-jodie-black-651876VCPP58803.html"),
        ("Andiamo", "https://www.bottegaveneta.com/en-us/mini-andiamo-fondant-874957VCPP12272.html"),
        ("Loop", "https://www.bottegaveneta.com/en-us/mini-loop-camera-bag-black-723547V1G118425.html"),
        ("Hop", "https://www.bottegaveneta.com/en-us/hop-small-black-806452V2H211443.html"),
        ("Pouch", "https://www.bottegaveneta.com/en-us/candy-concert-pouch-jungle-855179VCPP13197.html"),
        ("Candy", "https://www.bottegaveneta.com/en-us/mini-jodie-black-651876VCPP58803.html"),
        ("Padded", "https://www.bottegaveneta.com/en-ca/padded-cassette-caramel-591970VCQR19850.html"),
    ],
    "dior": [
        ("Lady Dior", "https://www.dior.com/en_us/fashion/products/S0980ZEDM_M900"),
        ("Book Tote", "https://www.dior.com/en_us/fashion/products/M1324CZBB_M928-medium-dior-book-tote-blue-dior-oblique-embroidery-and-calfskin-36.5-x-28-x-17.5-cm"),
        ("Saddle", "https://www.dior.com/en_us/fashion/products/M0447ZRIW_M928"),
        ("Caro", "https://www.dior.com/en_us/fashion/products/M9310ZEDM_M900"),
        ("Diorama", "https://www.dior.com/en_us/fashion/products/M9000ZEDM_M900"),
        ("Bobby", "https://www.dior.com/en_us/fashion/products/M9200ZEDM_M900"),
        ("Toujours", "https://www.dior.com/en_us/fashion/products/M1324OWHP_M900-medium-dior-book-tote-black-macrocannage-calfskin-36.5-x-28-x-17.5-cm"),
    ],
    "loewe": [
        ("Puzzle", "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html"),
        ("Flamenco", "https://www.loewe.com/int/en/women/bags/flamenco/mini-flamenco-clutch-in-nappa-calfskin/A411FC2XA6-2150.html"),
        ("Hammock", "https://www.loewe.com/int/en/women/bags/hammock/small-hammock-bag-in-classic-calfskin/387.30.S35-2530.html"),
        ("Amazona", "https://www.loewe.com/int/en/women/bags/amazona/small-amazona-180-bag-in-soft-calfskin/A039AS0X01-9579.html"),
        ("Gate", "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html"),
        ("Balloon", "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html"),
    ],
    "celine": [
        ("Triomphe", "https://www.celine.com/en/women/handbags/triomphe/triomphe-shoulder-bag-in-shiny-calfskin-188373BF4.38NO.html"),
        ("Classic Box", "https://www.celine.com/en/women/handbags/classic/classic-box-medium-bag-in-box-calfskin-189613BF4.04LU.html"),
        ("Luggage", "https://www.celine.com/en/women/handbags/luggage/luggage-nano-bag-in-drummed-calfskin-189613BF4.04LU.html"),
        ("Belt", "https://www.celine.com/en/women/handbags/belt/belt-mini-bag-in-grained-calfskin-189613BF4.04LU.html"),
        ("16", "https://www.celine.com/en/women/handbags/16/16-small-bag-in-satinated-calfskin-189613BF4.04LU.html"),
        ("Ava", "https://www.celine.com/en/women/handbags/ava/ava-bag-in-smooth-calfskin-189613BF4.04LU.html"),
        ("Cabas", "https://www.celine.com/en/women/handbags/cabas/cabas-phantom-bag-in-soft-grained-calfskin-189613BF4.04LU.html"),
    ],
    "saint-laurent": [
        ("Lou", "https://www.ysl.com/en-us/pr/lou-camera-bag-in-quilted-leather-761554DV7041000.html"),
        ("Kate", "https://www.ysl.com/en-us/pr/kate-medium-in-grain-de-poudre-embossed-leather-364021BOW0J1000.html"),
        ("Loulou", "https://www.ysl.com/en-us/pr/loulou-small-in-matelasse-lambskin-801437AAEAX1000.html"),
        ("Sac de Jour", "https://www.ysl.com/en-us/pr/sac-de-jour-in-grained-leather-nano-392035B681N1000.html"),
        ("Niki", "https://www.ysl.com/en-us/pr/niki-medium-in-grained-lambskin-633178AACYT3212.html"),
        ("Solferino", "https://www.ysl.com/en-us/pr/solferino-small-in-box-saint-laurent-8323300SX0W1000.html"),
        ("Cassandre", "https://www.ysl.com/en-us/pr/envelope-small-in-quilted-grain-de-poudre-embossed-leather-600195BOW911000.html"),
    ],
}

BRAND_FALLBACK_URL: dict[str, str] = {
    "miu-miu": "https://www.miumiu.com/eu/en/p/wander-matelasse-nappa-leather-hobo-mini-bag/5BP078_AN88_F0002_V_OOO",
    "gucci": "https://www.gucci.com/sg/en_gb/pr/women/handbags/crossbody-bags-for-women/gg-marmont-small-shoulder-bag-p-443497DRW1T1000",
    "bottega-veneta": "https://www.bottegaveneta.com/en-us/cassette-bag-calfskin-578004V04A08117.html",
    "dior": "https://www.dior.com/en_us/fashion/products/M1324CZBB_M928-medium-dior-book-tote-blue-dior-oblique-embroidery-and-calfskin-36.5-x-28-x-17.5-cm",
    "loewe": "https://www.loewe.com/int/en/women/bags/small-puzzle-bag-in-classic-calfskin/A510P47X01.html",
    "celine": "https://www.celine.com/en/women/handbags/triomphe/triomphe-shoulder-bag-in-shiny-calfskin-188373BF4.38NO.html",
    "saint-laurent": "https://www.ysl.com/en-us/pr/lou-camera-bag-in-quilted-leather-761554DV7041000.html",
}


def pick_url(brand_key: str, description: str) -> str:
    desc = description.lower()
    rules = MODEL_URLS.get(brand_key) or []
    for keyword, url in rules:
        if keyword.lower() in desc:
            return url
    return BRAND_FALLBACK_URL.get(brand_key) or (rules[0][1] if rules else "")


def main() -> None:
    with CATALOG.open(encoding="utf-8") as handle:
        catalog = json.load(handle)
    products: dict[str, str] = {}
    for brand_key, entry in catalog.get("brands", {}).items():
        for item in entry.get("products") or []:
            url = pick_url(brand_key, item.get("description") or item.get("name") or "")
            if url:
                products[item["id"]] = url
    OUT.write_text(
        json.dumps({"products": products}, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {len(products)} source URLs -> {OUT}")


if __name__ == "__main__":
    main()
