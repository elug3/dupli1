#!/usr/bin/env python3
from curl_cffi import requests
import re

r = requests.get(
    "https://www.loewe.com/int/en/women/bags/gate",
    impersonate="chrome120",
    timeout=45,
)
# Find API endpoints and product references
patterns = [
    r"https://[^\"'\s<>]+api[^\"'\s<>]+",
    r"/on/demandware\.store/[^\"'\s<>]+",
    r"Search-UpdateGrid[^\"'\s<>]*",
    r"productTile[^\"'\s<>]*",
    r"cgid[^\"'\s<>]*gate[^\"'\s<>]*",
    r"A650[A-Z0-9-]+",
    r"gate[^\"'\s<>]{0,40}\.html",
]
for pat in patterns:
    hits = sorted(set(re.findall(pat, r.text, re.I)))
    if hits:
        print(f"\n{pat[:50]} ({len(hits)})")
        for h in hits[:8]:
            print(" ", h[:150])

# Try demandware search API
for url in [
    "https://www.loewe.com/on/demandware.store/Sites-Loewe-Site/en/Search-Show?cgid=women-bags-gate",
    "https://www.loewe.com/on/demandware.store/Sites-Loewe_INT-Site/en/Search-Show?cgid=women-bags-gate",
    "https://www.loewe.com/on/demandware.store/Sites-Loewe_INT-Site/en/Search-UpdateGrid?cgid=women-bags-gate&start=0&sz=24",
]:
    try:
        rr = requests.get(url, impersonate="chrome120", timeout=45)
        print(f"\n{url.split('Sites')[1][:80]} -> {rr.status_code} len={len(rr.text)}")
        paths = re.findall(r"/women/bags/gate/[^\"'<>]+\.html", rr.text)
        print(" gate paths", len(set(paths)))
        for p in sorted(set(paths))[:5]:
            print(" ", p)
    except Exception as exc:
        print(url, exc)
