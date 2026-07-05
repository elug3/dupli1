#!/usr/bin/env python3
from curl_cffi import requests
import re

URLS = [
    ("cassette", "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html", "578004"),
    ("jodie", "https://www.bottegaveneta.com/en-us/mini-jodie-black-651876VCPP58803.html", "651876"),
    ("padded", "https://www.bottegaveneta.com/en-ca/padded-cassette-caramel-591970VCQR19850.html", "591970"),
    ("small", "https://www.bottegaveneta.com/en-us/small-cassette-black-730848VMAY18425.html", "730848"),
    ("andiamo", "https://www.bottegaveneta.com/en-us/small-andiamo-fondant-760409V2H211443.html", "760409"),
    ("loop", "https://www.bottegaveneta.com/en-us/mini-loop-camera-bag-black-666787VCPKB8817.html", "666787"),
]

PAT = re.compile(
    r"https://bottega-veneta\.dam\.kering\.com/(?:m/[a-f0-9]+|asset/[a-f0-9-]+)/"
    r"(?:Medium|Original-Ecom)/([A-Z0-9]+)_([A-Z])\.jpg(?:\?v=\d+)?",
    re.I,
)

for name, url, prefix in URLS:
    try:
        h = requests.get(url, impersonate="chrome120", timeout=45).text
        by_sku: dict[str, dict[str, str]] = {}
        for m in PAT.finditer(h):
            u, sku, letter = m.group(0).split(" ")[0], m.group(1), m.group(2).upper()
            if any(x in u for x in ("Adv_", "Campaign", "Attitude")):
                continue
            by_sku.setdefault(sku, {})[letter] = u
        candidates = [s for s in by_sku if s.startswith(prefix)] or list(by_sku.keys())
        best = max(candidates, key=lambda s: len(by_sku[s])) if candidates else ""
        imgs = [by_sku[best][k] for k in sorted(by_sku.get(best, {}))]
        print(f"{name}: status=200 sku={best} imgs={len(imgs)} url={url}")
        for u in imgs:
            print(f"  {u}")
    except Exception as exc:
        print(f"{name}: FAIL {exc}")
