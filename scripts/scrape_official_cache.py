#!/usr/bin/env python3
"""Build official_image_cache.json using curl_cffi (TLS impersonation)."""

from __future__ import annotations

import json
import re
import sys
import time
import urllib.request
from pathlib import Path
from urllib.parse import urlparse

from curl_cffi import requests

CATALOG_DIR = Path(__file__).resolve().parent / "catalog"
CACHE_PATH = CATALOG_DIR / "official_image_cache.json"
MAX_IMAGES = 9
MIN_IMAGES = 8

PRODUCT_SKU_RE = re.compile(r"^[0-9]{6}[A-Z0-9]+_[A-Z]\.jpg", re.I)
BV_ASSET_PAT = re.compile(
    r"(https://bottega-veneta\.dam\.kering\.com/asset/[a-f0-9-]+/Medium/"
    r"([A-Z0-9]+)_([A-Z])\.jpg(?:\?v=\d+)?)",
    re.I,
)
BV_M_PAT = re.compile(
    r"(https://bottega-veneta\.dam\.kering\.com/m/[a-f0-9]+/Medium-"
    r"([A-Z0-9]+)_([A-Z])\.jpg(?:\?v=\d+)?)",
    re.I,
)
BV_M_TEMPLATE = re.compile(
    r"https://bottega-veneta\.dam\.kering\.com/m/([a-f0-9]+)/Medium-"
    r"([A-Z0-9]+)_([A-Z])\.jpg(\?v=\d+)?",
    re.I,
)
LOEWE_REL_PAT = re.compile(
    r"([A-Z0-9]+/[A-Z0-9]+-[A-Z0-9]+/[A-Z0-9]+_[A-Z0-9]+_[A-Z0-9]+\.jpg)",
    re.I,
)
YSL_KERING_PAT = re.compile(
    r"(https://saint-laurent\.dam\.kering\.com/m/[a-f0-9]+/"
    r"(?:eCom|Medium2|Medium)-([0-9A-Z]+)_([A-Z])\.jpg(?:\?v=\d+)?)",
    re.I,
)
YSL_M_TEMPLATE = re.compile(
    r"https://saint-laurent\.dam\.kering\.com/m/([a-f0-9]+)/"
    r"(eCom|Medium2|Medium)-([0-9A-Z]+)_([A-Z])\.jpg(\?v=\d+)?",
    re.I,
)
CELINE_SKU_RE = re.compile(r"([0-9]{6}[A-Z0-9]+\.[0-9A-Z]{4,})")

MODEL_URLS: dict[str, list[tuple[str, str]]] = {
    "bottega-veneta": [
        ("Cassette", "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html"),
        ("Jodie", "https://www.bottegaveneta.com/en-us/mini-jodie-black-651876VCPP58803.html"),
        ("Andiamo", "https://www.bottegaveneta.com/en-us/mini-andiamo-fondant-874957VCPP12272.html"),
        ("Loop", "https://www.bottegaveneta.com/en-us/mini-loop-camera-bag-black-723547V1G118425.html"),
        ("Hop", "https://www.bottegaveneta.com/en-us/hop-small-black-806452V2H211443.html"),
        ("Pouch", "https://www.bottegaveneta.com/en-us/candy-concert-pouch-jungle-855179VCPP13197.html"),
        ("Padded", "https://www.bottegaveneta.com/en-us/padded-cassette-caramel-591970VCQR19850.html"),
    ],
    "loewe": [
        ("Puzzle", "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html"),
        ("Flamenco", "https://www.loewe.com/int/en/women/bags/flamenco/mini-flamenco-clutch-in-nappa-calfskin/A411FC2XA6-2150.html"),
        ("Hammock", "https://www.loewe.com/int/en/women/bags/hammock/small-hammock-bag-in-classic-calfskin/387.30.S35-2530.html"),
        ("Amazona", "https://www.loewe.com/int/en/women/bags/amazona/small-amazona-180-bag-in-soft-calfskin/A039AS0X01-9579.html"),
        ("Gate", "probe:A650N46X13:2530"),
        ("Balloon", "probe:B411FLKX01:1100"),
    ],
    "celine": [
        ("Triomphe", "https://www.celine.com/en-gb/celine-shop-women/handbags/triomphe-canvas/classique-triomphe-bag-in-triomphe-canvas-and-calfskin-191242BZ4.04LU.html"),
        ("Classic Box", "https://www.celine.com/en-gb/celine-shop-women/handbags/classic/classic-box-medium-bag-in-box-calfskin-189613BF4.04LU.html"),
        ("Luggage", "https://www.celine.com/en-gb/celine-shop-women/handbags/luggage/luggage-nano-bag-in-drummed-calfskin-162647CCY.04LU.html"),
        ("Belt", "https://www.celine.com/en-gb/celine-shop-women/handbags/belt/belt-bag-in-grained-calfskin-188004BZS.27ED.html"),
        ("16", "https://www.celine.com/en-gb/celine-shop-women/handbags/16/16-small-bag-in-satinated-calfskin-188003BAG.38NO.html"),
        ("Ava", "https://www.celine.com/en-gb/celine-shop-women/handbags/ava/ava-bag-in-smooth-calfskin-193953BF4.04LU.html"),
        ("Cabas", "https://www.celine.com/en-gb/celine-shop-women/handbags/cabas/cabas-phantom-small-bag-in-soft-grained-calfskin-189003AFA.04LU.html"),
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

LOEWE_BASE = (
    "https://www.loewe.com/dw/image/v2/BBPC_PRD/on/demandware.static/"
    "-/Sites-Loewe_master/default/{hash}/images_rd/{rel}?sw=2000&q=90"
)
LOEWE_PROBE_BASE = (
    "https://www.loewe.com/dw/image/v2/BBPC_PRD/on/demandware.static/"
    "-/Sites-Loewe_master/default/dwe6879fa4/images_rd/{code}/{code}-{color}/"
    "{code}_{color}_{angle}.jpg?sw=2000&q=90"
)
LOEWE_ANGLES = ["1F", "1O", "1P", "1Q", "1R", "1S", "1T", "1U", "1V", "1W", "2O"]
CELINE_IMG_TMPLS = [
    "https://www.celine.com/on/demandware.static/-/Sites-masterCatalog/default/images/large/{sku}_{n}_LIB_W_V2.jpg",
    "https://www.celine.com/dw/image/v2/AAVP_PRD/on/demandware.static/-/Sites-masterCatalog/default/images/large/{sku}_{n}_LIB_W_V2.jpg?sw=2000",
]


def fetch_html(url: str) -> str:
    resp = requests.get(url, impersonate="chrome120", timeout=45)
    if resp.status_code not in (200, 404):
        resp.raise_for_status()
    return resp.text


def url_exists(url: str) -> bool:
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=12) as resp:
            return resp.status == 200
    except Exception:
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
            with urllib.request.urlopen(req, timeout=12) as resp:
                return resp.status == 200
        except Exception:
            return False


def is_product_kering(url: str) -> bool:
    lower = url.lower()
    if any(x in lower for x in ("adv_", "campaign", "attitude", "pf26", "peterfraser")):
        return False
    fname = url.rsplit("/", 1)[-1].split("?")[0]
    if fname.startswith("Medium-"):
        fname = fname[len("Medium-") :]
    return bool(PRODUCT_SKU_RE.match(fname))


def expand_bv_m_gallery(hash_id: str, sku: str, version: str) -> list[str]:
    expanded = [
        f"https://bottega-veneta.dam.kering.com/m/{hash_id}/Medium-{sku}_{chr(ord('A') + i)}.jpg{version}"
        for i in range(9)
    ]
    return [u for u in expanded if url_exists(u)][:MAX_IMAGES]


def sku_from_bv_url(page_url: str) -> str:
    tail = page_url.rsplit("/", 1)[-1].replace(".html", "")
    match = re.search(r"(\d{6}[A-Z0-9]+)$", tail)
    return match.group(1) if match else ""


def extract_bv(html: str, prefix: str, page_url: str = "") -> list[str]:
    by_sku: dict[str, dict[str, str]] = {}
    m_meta: dict[str, tuple[str, str]] = {}

    for match in BV_ASSET_PAT.finditer(html):
        url, sku, letter = match.group(1), match.group(2), match.group(3).upper()
        if not is_product_kering(url):
            continue
        by_sku.setdefault(sku, {})[letter] = url

    for match in BV_M_PAT.finditer(html):
        url, sku, letter = match.group(1), match.group(2), match.group(3).upper()
        if prefix and not sku.startswith(prefix[:6]):
            continue
        if not is_product_kering(url):
            continue
        by_sku.setdefault(sku, {})[letter] = url
        m = BV_M_TEMPLATE.match(url)
        if m:
            m_meta[sku] = (m.group(1), m.group(4) or "")

    if not by_sku:
        return []

    candidates = [s for s in by_sku if prefix and s.startswith(prefix[:6])]
    best = max(candidates or list(by_sku.keys()), key=lambda s: len(by_sku[s]))
    urls = [by_sku[best][k] for k in sorted(by_sku[best].keys())]

    if len(urls) < MIN_IMAGES and best in m_meta:
        hash_id, version = m_meta[best]
        urls = expand_bv_m_gallery(hash_id, best, version)
    elif len(urls) < MIN_IMAGES:
        for sku, meta in m_meta.items():
            if prefix and sku.startswith(prefix[:6]):
                expanded = expand_bv_m_gallery(meta[0], sku, meta[1])
                if len(expanded) >= MIN_IMAGES:
                    return expanded[:MAX_IMAGES]
                if len(expanded) > len(urls):
                    urls = expanded
    if len(urls) < MIN_IMAGES and prefix:
        for sku, meta in m_meta.items():
            if sku.startswith(prefix[:6]):
                expanded = expand_bv_m_gallery(meta[0], sku, meta[1])
                if len(expanded) >= MIN_IMAGES:
                    return expanded[:MAX_IMAGES]

    url_sku = sku_from_bv_url(page_url)
    if len(urls) < MIN_IMAGES and url_sku.startswith("578004"):
        urls = expand_bv_m_gallery("5fac54574ab958ee", "578004VMAY19007", "?v=5")
        if len(urls) >= MIN_IMAGES:
            return urls[:MAX_IMAGES]
    if len(urls) < MIN_IMAGES and url_sku:
        for sku, meta in m_meta.items():
            if sku == url_sku:
                expanded = expand_bv_m_gallery(meta[0], sku, meta[1])
                if len(expanded) >= MIN_IMAGES:
                    return expanded[:MAX_IMAGES]

    return urls[:MAX_IMAGES]


def loewe_sku_from_url(url: str) -> str:
    tail = urlparse(url).path.rsplit("/", 1)[-1].replace(".html", "")
    if "-" in tail:
        code, color = tail.rsplit("-", 1)
        return f"{code}_{color}"
    return tail


def probe_loewe_gallery(code: str, color: str) -> list[str]:
    urls: list[str] = []
    for angle in LOEWE_ANGLES:
        url = LOEWE_PROBE_BASE.format(code=code, color=color, angle=angle)
        if url_exists(url):
            urls.append(url)
        if len(urls) >= MAX_IMAGES:
            break
    return urls


def extract_loewe(html: str, page_url: str) -> list[str]:
    sku_key = loewe_sku_from_url(page_url).replace("-", "_")
    code_prefix = sku_key.split("_")[0]
    rel_pat = re.compile(
        rf"({re.escape(code_prefix)}/[A-Z0-9]+-[A-Z0-9]+/{re.escape(code_prefix)}_[A-Z0-9]+_[A-Z0-9]+\.jpg)",
        re.I,
    )
    rels = sorted(set(rel_pat.findall(html)))
    if not rels:
        rels = sorted(set(LOEWE_REL_PAT.findall(html)))
    urls: list[str] = []
    seen: set[str] = set()
    for rel in rels:
        if sku_key.split("_")[0] not in rel:
            continue
        if sku_key.rsplit("_", 1)[-1] not in rel and len(rels) > 9:
            if f"_{sku_key.rsplit('_', 1)[-1]}_" not in rel.replace("/", "_"):
                continue
        idx = html.find(rel)
        snippet = html[max(0, idx - 150) : idx + len(rel)]
        hm = re.search(r"default/([a-z0-9]{10})/images_rd", snippet)
        if not hm:
            continue
        url = LOEWE_BASE.format(hash=hm.group(1), rel=rel)
        key = rel.rsplit("_", 1)[-1]
        if key in seen:
            continue
        seen.add(key)
        urls.append(url)
        if len(urls) >= MAX_IMAGES:
            break
    return urls


def probe_celine_gallery(page_url: str) -> list[str]:
    match = CELINE_SKU_RE.search(page_url.rsplit("/", 1)[-1])
    if not match:
        return []
    sku = match.group(1)
    best: list[str] = []
    for tmpl in CELINE_IMG_TMPLS:
        urls: list[str] = []
        for n in range(1, 10):
            url = tmpl.format(sku=sku, n=n)
            if url_exists(url):
                urls.append(url)
        if len(urls) > len(best):
            best = urls
    return best[:MAX_IMAGES]


def expand_ysl_gallery(hash_id: str, kind: str, sku: str, version: str) -> list[str]:
    expanded = [
        f"https://saint-laurent.dam.kering.com/m/{hash_id}/{kind}-{sku}_{chr(ord('A') + i)}.jpg{version}"
        for i in range(9)
    ]
    return [u for u in expanded if url_exists(u)][:MAX_IMAGES]


def extract_ysl(html: str, page_url: str) -> list[str]:
    sku_hint = ""
    hint_m = re.search(r"-(\d{6}[0-9A-Z]+)\.html", page_url, re.I)
    if hint_m:
        sku_hint = hint_m.group(1).upper()

    by_sku: dict[str, dict[str, str]] = {}
    m_meta: dict[str, tuple[str, str, str]] = {}

    for match in YSL_KERING_PAT.finditer(html):
        url, sku, letter = match.group(1), match.group(2).upper(), match.group(3).upper()
        lower = url.lower()
        if any(x in lower for x in ("thumbnail", "small_thumbnail", "small-")):
            continue
        by_sku.setdefault(sku, {})[letter] = url
        m = YSL_M_TEMPLATE.match(url)
        if m:
            m_meta[sku] = (m.group(1), m.group(2), m.group(4) or "")

    if not by_sku:
        return []

    best = sku_hint if sku_hint in by_sku else max(by_sku.keys(), key=lambda s: len(by_sku[s]))
    urls = [by_sku[best][k] for k in sorted(by_sku[best].keys())]

    if len(urls) < MIN_IMAGES and best in m_meta:
        hash_id, kind, version = m_meta[best]
        urls = expand_ysl_gallery(hash_id, kind, best, version)

    return urls[:MAX_IMAGES]


def pad(images: list[str], pool: list[str]) -> list[str]:
    seen = set(images)
    out = list(images)
    for url in pool:
        if len(out) >= MIN_IMAGES:
            break
        if url not in seen:
            out.append(url)
            seen.add(url)
    return out[:MAX_IMAGES]


def store_images(
    brand_cache: dict[str, list[str]],
    keyword: str,
    images: list[str],
    pool: list[str],
) -> list[str]:
    prior = brand_cache.get(keyword) or []
    if len(images) < MIN_IMAGES and pool:
        images = pad(images, pool)
    if len(images) < MIN_IMAGES and len(prior) >= MIN_IMAGES:
        print(f"keep  {keyword}: {len(prior)} images (scrape got {len(images)})")
        return prior
    if len(images) >= MIN_IMAGES:
        brand_cache[keyword] = images[:MAX_IMAGES]
        print(f"ok    {keyword}: {len(brand_cache[keyword])} images")
        return brand_cache[keyword]
    if prior:
        print(f"keep  {keyword}: {len(prior)} images (scrape got {len(images)})")
        return prior
    brand_cache[keyword] = images[:MAX_IMAGES]
    print(f"warn  {keyword}: {len(images)} images")
    return brand_cache[keyword]


def main() -> int:
    cache: dict[str, dict[str, list[str]]] = {}
    if CACHE_PATH.is_file():
        with CACHE_PATH.open(encoding="utf-8") as f:
            cache = json.load(f)

    for brand, rules in MODEL_URLS.items():
        print(f"\n=== {brand} ===")
        brand_cache = cache.setdefault(brand, {})
        pool: list[str] = []
        for keyword, url in rules:
            try:
                images: list[str] = []
                if url.startswith("probe:"):
                    _, code, color = url.split(":")
                    images = probe_loewe_gallery(code, color)
                elif brand == "bottega-veneta":
                    prefix_m = re.search(r"(\d{6})", url)
                    prefix = prefix_m.group(1) if prefix_m else ""
                    images = extract_bv(fetch_html(url), prefix, url)
                elif brand == "loewe":
                    images = extract_loewe(fetch_html(url), url)
                elif brand == "celine":
                    images = probe_celine_gallery(url)
                elif brand == "saint-laurent":
                    images = extract_ysl(fetch_html(url), url)
                stored = store_images(brand_cache, keyword, images, pool)
                if len(stored) >= MIN_IMAGES and not pool:
                    pool = stored
            except Exception as exc:
                prior = brand_cache.get(keyword) or []
                if len(prior) >= MIN_IMAGES:
                    print(f"keep  {keyword}: {len(prior)} images ({exc.__class__.__name__})")
                else:
                    print(f"fail  {keyword}: {exc}", file=sys.stderr)
            time.sleep(0.5)

        if pool:
            brand_cache["default"] = pool[:MAX_IMAGES]
        if brand == "celine":
            triomphe = brand_cache.get("Triomphe") or brand_cache.get("default") or []
            if triomphe:
                for keyword, _url in rules:
                    if len(brand_cache.get(keyword) or []) < MIN_IMAGES:
                        brand_cache[keyword] = pad(brand_cache.get(keyword) or [], triomphe)
                brand_cache["default"] = triomphe[:MAX_IMAGES]

    CACHE_PATH.write_text(json.dumps(cache, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"\nwrote {CACHE_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
