#!/usr/bin/env python3
from curl_cffi import requests
import re

idx = requests.get(
    "https://www.loewe.com/sitemap_index.xml",
    impersonate="chrome120",
    timeout=30,
).text
print(idx)
maps = re.findall(r"<loc>([^<]+)</loc>", idx)
print("maps", maps)

for sm in maps[:5]:
    body = requests.get(sm, impersonate="chrome120", timeout=60).text
    gate = [u for u in re.findall(r"<loc>([^<]+)</loc>", body) if "/bags/gate/" in u]
    balloon = [u for u in re.findall(r"<loc>([^<]+)</loc>", body) if "/bags/balloon/" in u]
    if gate or balloon:
        print(f"\n{sm}: gate={len(gate)} balloon={len(balloon)}")
        for u in (gate + balloon)[:8]:
            print(" ", u)
