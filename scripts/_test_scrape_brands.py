#!/usr/bin/env python3
import json
import re
import urllib.request
from pathlib import Path

UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)
URLS_PATH = Path(__file__).parent / "catalog" / "official_source_urls.json"

PATTERNS = {
    "bottega-veneta": re.compile(
        r"https://[^\"'\s<>]*bottegaveneta\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)",
        re.I,
    ),
    "loewe": re.compile(
        r"https://[^\"'\s<>]*loewe\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)",
        re.I,
    ),
    "celine": re.compile(
        r"https://[^\"'\s<>]*celine\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)",
        re.I,
    ),
    "saint-laurent": re.compile(
        r"https://[^\"'\s<>]*ysl\.com/[^\"'\s<>]+?\.(?:jpg|jpeg|webp)",
        re.I,
    ),
}

with URLS_PATH.open(encoding="utf-8") as f:
    urls = json.load(f)["products"]

samples = {
    "bottega-veneta": urls["BV-001"],
    "loewe": urls["LOE-001"],
    "celine": urls["CEL-001"],
    "saint-laurent": urls["YSL-001"],
}

for brand, url in samples.items():
    try:
        req = urllib.request.Request(
            url,
            headers={
                "User-Agent": UA,
                "Accept-Language": "en-US,en;q=0.9",
                "Accept": "text/html,application/xhtml+xml",
            },
        )
        with urllib.request.urlopen(req, timeout=45) as resp:
            html = resp.read().decode("utf-8", errors="replace")
        found = PATTERNS[brand].findall(html)
        unique = list(dict.fromkeys(found))
        print(f"\n{brand}: OK len={len(html)} imgs={len(unique)}")
        for u in unique[:12]:
            print(f"  {u[:140]}")
    except Exception as exc:
        print(f"\n{brand}: FAIL {exc}")
