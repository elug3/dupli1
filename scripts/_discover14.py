#!/usr/bin/env python3
from curl_cffi import requests
import re

r = requests.get(
    "https://www.loewe.com/int/en/women/bags/gate",
    impersonate="chrome120",
    timeout=45,
)
# dump script tags with product-ish content
for m in re.finditer(r"<script[^>]*>(.*?)</script>", r.text, re.S):
    body = m.group(1)
    if any(k in body for k in ("A650", "gate", "product", "images_rd", "w_gate")):
        if len(body) < 5000:
            print("--- script", len(body), "---")
            print(body[:800])
            print()

# search json blobs
for pat in [r'"pid"\s*:\s*"([^"]+)"', r'"productID"\s*:\s*"([^"]+)"', r'"id"\s*:\s*"(A[0-9][^"]+)"']:
    hits = re.findall(pat, r.text)
    if hits:
        print(pat, sorted(set(hits))[:15])
